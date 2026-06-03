// Package notification menyediakan async notification delivery untuk BLIPS IFRS9.
//
// Channels yang didukung: EMAIL (SMTP) dan INAPP (in-app notification queue).
//
// Template-driven: setiap pesan dirender dari sys.notification_template dengan variabel
// yang di-substitute via text/template. Pesan WAJIB SPESIFIK per ux-patterns.md §2.2 —
// DILARANG generic "Berhasil"/"Gagal".
//
// Dev-safe: jika SMTP_HOST env kosong, mailer masuk dry-run mode (log saja, tidak kirim).
// Pola ini sama seperti JWT dev-safe di foundation core.
//
// Async delivery: semua pengiriman di-enqueue ke Asynq (DEC-007) agar HTTP handler tidak
// terblok. Worker mengambil task dari queue dan memanggil Deliver.
package notification

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"text/template"
	"time"
)

// Channel adalah saluran notifikasi yang didukung.
type Channel string

const (
	// ChannelEmail pengiriman via SMTP.
	ChannelEmail Channel = "EMAIL"
	// ChannelInApp pengiriman ke antrian in-app (UI notification bell).
	ChannelInApp Channel = "INAPP"
)

// TemplateCode adalah kode template yang ada di sys.notification_template.
// Nilai wajib cocok persis dengan kolom template_code di DB.
type TemplateCode string

const (
	// Workflow transitions

	// TmplWorkflowSubmitted dikirim ke reviewer ketika maker submit entitas.
	// Variables: EntityLabel, EntityKode, EntityType, MakerName, ActionURL
	TmplWorkflowSubmitted TemplateCode = "WORKFLOW.SUBMITTED"

	// TmplWorkflowReviewed dikirim ke approver ketika reviewer sign-off.
	// Variables: EntityLabel, EntityKode, EntityType, ReviewerName, ActionURL
	TmplWorkflowReviewed TemplateCode = "WORKFLOW.REVIEWED"

	// TmplWorkflowApproved dikirim ke maker ketika entitas di-approve.
	// Variables: EntityLabel, EntityKode, EntityType, ApproverName, ActionURL
	TmplWorkflowApproved TemplateCode = "WORKFLOW.APPROVED"

	// TmplWorkflowRejected dikirim ke maker ketika entitas di-reject.
	// Variables: EntityLabel, EntityKode, EntityType, RejectorName, RejectComment, ActionURL
	TmplWorkflowRejected TemplateCode = "WORKFLOW.REJECTED"

	// TmplWorkflowApproved2 dikirim ke maker ketika 6-eyes second approver sign-off.
	// Variables: EntityLabel, EntityKode, EntityType, Approver2Name, ActionURL
	TmplWorkflowApproved2 TemplateCode = "WORKFLOW.APPROVED2"

	// Job completion

	// TmplJobCompleted dikirim ke user pemicu job ketika job selesai.
	// Variables: JobType, JobLabel, CompletedAt, ResultSummary, ActionURL
	TmplJobCompleted TemplateCode = "JOB.COMPLETED"

	// TmplJobFailed dikirim ke user pemicu job + ROLE-IT-ADMIN ketika job gagal.
	// Variables: JobType, JobLabel, FailedAt, ErrorCode, ErrorMessage, TraceID, ActionURL
	TmplJobFailed TemplateCode = "JOB.FAILED"
)

// TemplateVars adalah map variabel yang disubstitusikan ke template.
// Key harus PascalCase agar cocok dengan {{.Key}} di text/template.
type TemplateVars map[string]string

// NotificationTemplate adalah record dari sys.notification_template.
type NotificationTemplate struct {
	ID             string
	TemplateCode   TemplateCode
	Channel        Channel
	SubjectTemplate string
	BodyTemplate   string
	Language       string
	AktifFlag      bool
	UpdatedAt      *time.Time
}

// TemplateStore memuat template dari sys.notification_template atau fallback in-memory.
type TemplateStore interface {
	// Load memuat template untuk code + channel + language.
	// Mengembalikan ErrTemplateNotFound jika tidak ditemukan.
	Load(ctx context.Context, code TemplateCode, channel Channel, lang string) (*NotificationTemplate, error)
}

// ErrTemplateNotFound adalah error ketika template tidak ditemukan di store.
var ErrTemplateNotFound = fmt.Errorf("notification template tidak ditemukan")

// DBTemplateStore mengambil template dari database.
type DBTemplateStore struct {
	db *sql.DB
}

// NewDBTemplateStore membuat DBTemplateStore.
func NewDBTemplateStore(db *sql.DB) *DBTemplateStore {
	return &DBTemplateStore{db: db}
}

// Load mengambil template dari sys.notification_template.
func (s *DBTemplateStore) Load(ctx context.Context, code TemplateCode, channel Channel, lang string) (*NotificationTemplate, error) {
	if s.db == nil {
		return nil, ErrTemplateNotFound
	}

	var t NotificationTemplate
	var subject sql.NullString
	var updatedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, template_code, channel, subject_template, body_template, language, aktif_flag, updated_at
		FROM sys.notification_template
		WHERE template_code = $1 AND channel = $2 AND language = $3 AND aktif_flag = TRUE
		LIMIT 1
	`, string(code), string(channel), lang).Scan(
		&t.ID, &t.TemplateCode, &t.Channel,
		&subject, &t.BodyTemplate, &t.Language, &t.AktifFlag, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: code=%s channel=%s lang=%s", ErrTemplateNotFound, code, channel, lang)
	}
	if err != nil {
		return nil, fmt.Errorf("notification: load template: %w", err)
	}

	if subject.Valid {
		t.SubjectTemplate = subject.String
	}
	if updatedAt.Valid {
		t.UpdatedAt = &updatedAt.Time
	}

	return &t, nil
}

// InMemoryTemplateStore adalah fallback template store untuk dev/test.
// Berisi template default yang mencerminkan ux-patterns.md §2.2 (pesan spesifik).
type InMemoryTemplateStore struct {
	templates map[templateKey]*NotificationTemplate
}

type templateKey struct {
	code    TemplateCode
	channel Channel
	lang    string
}

// NewInMemoryTemplateStore membuat store dengan template bawaan (dev/test safe).
func NewInMemoryTemplateStore() *InMemoryTemplateStore {
	store := &InMemoryTemplateStore{
		templates: make(map[templateKey]*NotificationTemplate),
	}
	store.seed()
	return store
}

// seed mengisi template bawaan. Pesan SPESIFIK sesuai ux-patterns.md §2.2.
func (s *InMemoryTemplateStore) seed() {
	add := func(code TemplateCode, channel Channel, subject, body string) {
		s.templates[templateKey{code, channel, "id-ID"}] = &NotificationTemplate{
			TemplateCode:    code,
			Channel:         channel,
			SubjectTemplate: subject,
			BodyTemplate:    body,
			Language:        "id-ID",
			AktifFlag:       true,
		}
	}

	// WORKFLOW.SUBMITTED — ke reviewer
	add(TmplWorkflowSubmitted, ChannelEmail,
		`[BLIPS] {{.EntityLabel}} {{.EntityKode}} menunggu review Anda`,
		`Yth. Reviewer,

{{.EntityLabel}} {{.EntityKode}} (tipe: {{.EntityType}}) telah disubmit oleh {{.MakerName}} dan menunggu review Anda.

Silakan buka link berikut untuk meninjau:
{{.ActionURL}}

Salam,
Sistem BLIPS IFRS9`,
	)
	add(TmplWorkflowSubmitted, ChannelInApp,
		"",
		`{{.EntityLabel}} {{.EntityKode}} menunggu review Anda. Disubmit oleh {{.MakerName}}.`,
	)

	// WORKFLOW.REVIEWED — ke approver
	add(TmplWorkflowReviewed, ChannelEmail,
		`[BLIPS] {{.EntityLabel}} {{.EntityKode}} menunggu persetujuan Anda`,
		`Yth. Approver,

{{.EntityLabel}} {{.EntityKode}} (tipe: {{.EntityType}}) telah direview oleh {{.ReviewerName}} dan menunggu persetujuan Anda.

Silakan buka link berikut untuk memberikan persetujuan:
{{.ActionURL}}

Salam,
Sistem BLIPS IFRS9`,
	)
	add(TmplWorkflowReviewed, ChannelInApp,
		"",
		`{{.EntityLabel}} {{.EntityKode}} menunggu persetujuan Anda. Telah direview oleh {{.ReviewerName}}.`,
	)

	// WORKFLOW.APPROVED — ke maker
	add(TmplWorkflowApproved, ChannelEmail,
		`[BLIPS] {{.EntityLabel}} {{.EntityKode}} telah disetujui`,
		`Yth. {{.MakerName}},

{{.EntityLabel}} {{.EntityKode}} (tipe: {{.EntityType}}) telah disetujui oleh {{.ApproverName}}.

Lihat detail:
{{.ActionURL}}

Salam,
Sistem BLIPS IFRS9`,
	)
	add(TmplWorkflowApproved, ChannelInApp,
		"",
		`{{.EntityLabel}} {{.EntityKode}} telah disetujui oleh {{.ApproverName}}.`,
	)

	// WORKFLOW.APPROVED2 — ke maker (6-eyes selesai)
	add(TmplWorkflowApproved2, ChannelEmail,
		`[BLIPS] {{.EntityLabel}} {{.EntityKode}} mendapat persetujuan final`,
		`Yth. {{.MakerName}},

{{.EntityLabel}} {{.EntityKode}} (tipe: {{.EntityType}}) telah mendapat persetujuan final dari {{.Approver2Name}} (6-eyes).

Lihat detail:
{{.ActionURL}}

Salam,
Sistem BLIPS IFRS9`,
	)
	add(TmplWorkflowApproved2, ChannelInApp,
		"",
		`{{.EntityLabel}} {{.EntityKode}} mendapat persetujuan final dari {{.Approver2Name}}.`,
	)

	// WORKFLOW.REJECTED — ke maker
	add(TmplWorkflowRejected, ChannelEmail,
		`[BLIPS] {{.EntityLabel}} {{.EntityKode}} ditolak`,
		`Yth. {{.MakerName}},

{{.EntityLabel}} {{.EntityKode}} (tipe: {{.EntityType}}) ditolak oleh {{.RejectorName}}.

Alasan penolakan:
{{.RejectComment}}

Silakan perbaiki dan submit ulang:
{{.ActionURL}}

Salam,
Sistem BLIPS IFRS9`,
	)
	add(TmplWorkflowRejected, ChannelInApp,
		"",
		`{{.EntityLabel}} {{.EntityKode}} ditolak oleh {{.RejectorName}}. Alasan: {{.RejectComment}}`,
	)

	// JOB.COMPLETED — ke user pemicu
	add(TmplJobCompleted, ChannelEmail,
		`[BLIPS] {{.JobLabel}} selesai`,
		`Yth. Pengguna,

{{.JobLabel}} (tipe: {{.JobType}}) telah selesai pada {{.CompletedAt}}.

Ringkasan hasil:
{{.ResultSummary}}

Lihat detail:
{{.ActionURL}}

Salam,
Sistem BLIPS IFRS9`,
	)
	add(TmplJobCompleted, ChannelInApp,
		"",
		`{{.JobLabel}} selesai pada {{.CompletedAt}}. {{.ResultSummary}}`,
	)

	// JOB.FAILED — ke user + ROLE-IT-ADMIN
	add(TmplJobFailed, ChannelEmail,
		`[BLIPS] {{.JobLabel}} GAGAL — tindakan diperlukan`,
		`Yth. Tim,

{{.JobLabel}} (tipe: {{.JobType}}) GAGAL pada {{.FailedAt}}.

Error: {{.ErrorCode}} — {{.ErrorMessage}}
Trace ID: {{.TraceID}}

Silakan periksa DLQ dan runbook:
{{.ActionURL}}

Salam,
Sistem BLIPS IFRS9`,
	)
	add(TmplJobFailed, ChannelInApp,
		"",
		`{{.JobLabel}} GAGAL ({{.ErrorCode}}). Trace: {{.TraceID}}. Hubungi IT Admin.`,
	)
}

// Load mengambil template dari map in-memory.
func (s *InMemoryTemplateStore) Load(_ context.Context, code TemplateCode, channel Channel, lang string) (*NotificationTemplate, error) {
	t, ok := s.templates[templateKey{code, channel, lang}]
	if !ok {
		return nil, fmt.Errorf("%w: code=%s channel=%s lang=%s", ErrTemplateNotFound, code, channel, lang)
	}
	return t, nil
}

// RenderTemplate merender template dengan variabel yang diberikan.
// Mengembalikan subject dan body yang sudah dirender.
// Error bila template parsing gagal atau variabel tidak lengkap.
func RenderTemplate(tmpl *NotificationTemplate, vars TemplateVars) (subject, body string, err error) {
	body, err = renderText(tmpl.BodyTemplate, vars)
	if err != nil {
		return "", "", fmt.Errorf("notification: render body template %s: %w", tmpl.TemplateCode, err)
	}

	if tmpl.SubjectTemplate != "" {
		subject, err = renderText(tmpl.SubjectTemplate, vars)
		if err != nil {
			return "", "", fmt.Errorf("notification: render subject template %s: %w", tmpl.TemplateCode, err)
		}
	}

	return subject, body, nil
}

// renderText mem-parse dan mengeksekusi text/template dengan vars map.
func renderText(tmplStr string, vars TemplateVars) (string, error) {
	t, err := template.New("").Option("missingkey=zero").Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}
