package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// TaskTypeNotify adalah nama Asynq task untuk pengiriman notifikasi.
const TaskTypeNotify = "notification:deliver"

// DeliveryRequest adalah payload satu pengiriman notifikasi.
// Di-serialize ke JSON dan di-enqueue sebagai Asynq task payload.
type DeliveryRequest struct {
	// TemplateCode adalah kode template di sys.notification_template.
	TemplateCode TemplateCode `json:"templateCode"`
	// Channel adalah saluran pengiriman.
	Channel Channel `json:"channel"`
	// RecipientEmails untuk channel EMAIL.
	RecipientEmails []string `json:"recipientEmails,omitempty"`
	// RecipientUserIDs untuk channel INAPP (UUID pengguna).
	RecipientUserIDs []string `json:"recipientUserIds,omitempty"`
	// Vars adalah variabel template.
	Vars TemplateVars `json:"vars"`
	// Language adalah kode bahasa, default "id-ID".
	Language string `json:"language,omitempty"`
	// TraceID untuk korelasi log.
	TraceID string `json:"traceId,omitempty"`
}

// Service mengorkestrasi pengiriman notifikasi.
// Enqueue ke Asynq; worker kemudian memanggil Deliver.
type Service struct {
	client    *asynq.Client
	store     TemplateStore
	mailer    *Mailer
	inAppSink InAppSink
	logger    *slog.Logger
}

// InAppSink adalah interface untuk menyimpan in-app notification.
// Implementasi nyata menulis ke tabel sys.notification atau Redis pub/sub.
// Fase 1: stub yang hanya log.
type InAppSink interface {
	Store(ctx context.Context, userIDs []string, message string) error
}

// LogInAppSink adalah InAppSink stub yang hanya log (dev/test safe).
type LogInAppSink struct {
	logger *slog.Logger
}

// NewLogInAppSink membuat LogInAppSink.
func NewLogInAppSink(logger *slog.Logger) *LogInAppSink {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogInAppSink{logger: logger}
}

// Store mencatat in-app notification ke log (stub Phase 1).
func (s *LogInAppSink) Store(_ context.Context, userIDs []string, message string) error {
	s.logger.Info("[INAPP stub] notification stored",
		"userIds", userIDs,
		"message_preview", truncate(message, 100),
	)
	return nil
}

// NewService membuat Service.
// asynqClient boleh nil (akan skip enqueue, deliver langsung sinkron — testing mode).
func NewService(
	asynqClient *asynq.Client,
	store TemplateStore,
	mailer *Mailer,
	inAppSink InAppSink,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if store == nil {
		store = NewInMemoryTemplateStore()
	}
	return &Service{
		client:    asynqClient,
		store:     store,
		mailer:    mailer,
		inAppSink: inAppSink,
		logger:    logger,
	}
}

// Enqueue meng-enqueue satu delivery request ke Asynq dengan retry (max 5).
// Jika asynqClient nil, deliver langsung sinkron (testing/dev mode tanpa Redis).
func (s *Service) Enqueue(ctx context.Context, req DeliveryRequest) error {
	if req.Language == "" {
		req.Language = "id-ID"
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("notification: marshal delivery request: %w", err)
	}

	if s.client == nil {
		// Dev mode: deliver sinkron tanpa Redis.
		s.logger.Debug("notification: asynq client nil, delivering synchronously")
		return s.Deliver(ctx, req)
	}

	task := asynq.NewTask(TaskTypeNotify, payload)
	_, err = s.client.EnqueueContext(ctx, task,
		asynq.MaxRetry(5),
		asynq.Queue("notification"),
	)
	if err != nil {
		return fmt.Errorf("notification: enqueue task: %w", err)
	}

	s.logger.DebugContext(ctx, "notification enqueued",
		"templateCode", req.TemplateCode,
		"channel", req.Channel,
		"traceId", req.TraceID,
	)
	return nil
}

// Deliver mengeksekusi pengiriman secara langsung (dipanggil oleh Asynq worker).
// Juga dapat dipanggil langsung untuk testing atau dev sync mode.
func (s *Service) Deliver(ctx context.Context, req DeliveryRequest) error {
	lang := req.Language
	if lang == "" {
		lang = "id-ID"
	}

	tmpl, err := s.store.Load(ctx, req.TemplateCode, req.Channel, lang)
	if err != nil {
		return fmt.Errorf("notification: load template %s/%s: %w", req.TemplateCode, req.Channel, err)
	}

	subject, body, err := RenderTemplate(tmpl, req.Vars)
	if err != nil {
		return err
	}

	switch req.Channel {
	case ChannelEmail:
		if s.mailer == nil {
			return fmt.Errorf("notification: mailer tidak dikonfigurasi untuk channel EMAIL")
		}
		if len(req.RecipientEmails) == 0 {
			return fmt.Errorf("notification: recipientEmails kosong untuk channel EMAIL")
		}
		if err := s.mailer.SendEmail(req.RecipientEmails, subject, body); err != nil {
			return fmt.Errorf("notification: send email template %s: %w", req.TemplateCode, err)
		}
		s.logger.InfoContext(ctx, "notification email sent",
			"templateCode", req.TemplateCode,
			"to", req.RecipientEmails,
			"subject", subject,
		)

	case ChannelInApp:
		if s.inAppSink == nil {
			return fmt.Errorf("notification: inAppSink tidak dikonfigurasi untuk channel INAPP")
		}
		if len(req.RecipientUserIDs) == 0 {
			return fmt.Errorf("notification: recipientUserIds kosong untuk channel INAPP")
		}
		if err := s.inAppSink.Store(ctx, req.RecipientUserIDs, body); err != nil {
			return fmt.Errorf("notification: store in-app notification template %s: %w", req.TemplateCode, err)
		}
		s.logger.InfoContext(ctx, "notification in-app stored",
			"templateCode", req.TemplateCode,
			"userIds", req.RecipientUserIDs,
		)

	default:
		return fmt.Errorf("notification: channel tidak dikenal: %s", req.Channel)
	}

	return nil
}

// NotifyWorkflowTransition mengirim notifikasi workflow untuk sebuah transition.
// Mengirim ke KEDUA channel (EMAIL + INAPP) agar feedback user lengkap.
// Failure pada satu channel di-log tapi tidak membatalkan channel lain.
func (s *Service) NotifyWorkflowTransition(ctx context.Context, p WorkflowTransitionParams) {
	reqs := buildWorkflowRequests(p)
	for _, req := range reqs {
		if err := s.Enqueue(ctx, req); err != nil {
			s.logger.WarnContext(ctx, "notification: failed to enqueue workflow transition",
				"templateCode", req.TemplateCode,
				"channel", req.Channel,
				"error", err,
				"traceId", req.TraceID,
			)
			// Non-fatal: notifikasi gagal tidak boleh membatalkan workflow transition.
		}
	}
}

// NotifyJobCompleted mengirim notifikasi ketika async job selesai.
func (s *Service) NotifyJobCompleted(ctx context.Context, p JobNotificationParams) {
	vars := TemplateVars{
		"JobType":       p.JobType,
		"JobLabel":      p.JobLabel,
		"CompletedAt":   p.CompletedAt,
		"ResultSummary": p.ResultSummary,
		"ActionURL":     p.ActionURL,
	}

	channels := []Channel{ChannelInApp}
	if len(p.RecipientEmails) > 0 {
		channels = append(channels, ChannelEmail)
	}

	for _, ch := range channels {
		req := DeliveryRequest{
			TemplateCode:     TmplJobCompleted,
			Channel:          ch,
			RecipientEmails:  p.RecipientEmails,
			RecipientUserIDs: p.RecipientUserIDs,
			Vars:             vars,
			TraceID:          p.TraceID,
		}
		if err := s.Enqueue(ctx, req); err != nil {
			s.logger.WarnContext(ctx, "notification: failed to enqueue job completed",
				"jobType", p.JobType,
				"channel", ch,
				"error", err,
			)
		}
	}
}

// NotifyJobFailed mengirim notifikasi ketika async job gagal.
// Dikirim ke user pemicu + ROLE-IT-ADMIN (per system owner table FSD-MASTER §5).
func (s *Service) NotifyJobFailed(ctx context.Context, p JobNotificationParams) {
	vars := TemplateVars{
		"JobType":      p.JobType,
		"JobLabel":     p.JobLabel,
		"FailedAt":     p.CompletedAt,
		"ErrorCode":    p.ErrorCode,
		"ErrorMessage": p.ErrorMessage,
		"TraceID":      p.TraceID,
		"ActionURL":    p.ActionURL,
	}

	channels := []Channel{ChannelInApp}
	if len(p.RecipientEmails) > 0 {
		channels = append(channels, ChannelEmail)
	}

	for _, ch := range channels {
		req := DeliveryRequest{
			TemplateCode:     TmplJobFailed,
			Channel:          ch,
			RecipientEmails:  p.RecipientEmails,
			RecipientUserIDs: p.RecipientUserIDs,
			Vars:             vars,
			TraceID:          p.TraceID,
		}
		if err := s.Enqueue(ctx, req); err != nil {
			s.logger.WarnContext(ctx, "notification: failed to enqueue job failed",
				"jobType", p.JobType,
				"channel", ch,
				"error", err,
			)
		}
	}
}

// WorkflowTransitionParams adalah parameter untuk NotifyWorkflowTransition.
type WorkflowTransitionParams struct {
	// TemplateCode: TmplWorkflow{Submitted,Reviewed,Approved,Rejected,Approved2}
	TemplateCode TemplateCode
	// RecipientEmails adalah list email tujuan.
	RecipientEmails []string
	// RecipientUserIDs adalah list user UUID untuk in-app notif.
	RecipientUserIDs []string
	// Vars berisi variabel spesifik template.
	Vars TemplateVars
	// TraceID untuk korelasi.
	TraceID string
}

// JobNotificationParams adalah parameter untuk NotifyJob{Completed,Failed}.
type JobNotificationParams struct {
	JobType          string
	JobLabel         string
	CompletedAt      string // formatted time string
	ResultSummary    string
	ErrorCode        string
	ErrorMessage     string
	ActionURL        string
	RecipientEmails  []string
	RecipientUserIDs []string
	TraceID          string
}

// buildWorkflowRequests membangun daftar DeliveryRequest untuk kedua channel.
func buildWorkflowRequests(p WorkflowTransitionParams) []DeliveryRequest {
	reqs := make([]DeliveryRequest, 0, 2)

	if len(p.RecipientEmails) > 0 {
		reqs = append(reqs, DeliveryRequest{
			TemplateCode:    p.TemplateCode,
			Channel:         ChannelEmail,
			RecipientEmails: p.RecipientEmails,
			Vars:            p.Vars,
			TraceID:         p.TraceID,
		})
	}

	if len(p.RecipientUserIDs) > 0 {
		reqs = append(reqs, DeliveryRequest{
			TemplateCode:     p.TemplateCode,
			Channel:          ChannelInApp,
			RecipientUserIDs: p.RecipientUserIDs,
			Vars:             p.Vars,
			TraceID:          p.TraceID,
		})
	}

	return reqs
}
