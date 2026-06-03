package notification

import (
	"context"
	"testing"
)

// TestInMemoryTemplateStore_Load memverifikasi semua template yang dibutuhkan tersedia
// dan channel-nya benar.
func TestInMemoryTemplateStore_Load(t *testing.T) {
	store := NewInMemoryTemplateStore()
	ctx := context.Background()

	cases := []struct {
		code    TemplateCode
		channel Channel
	}{
		{TmplWorkflowSubmitted, ChannelEmail},
		{TmplWorkflowSubmitted, ChannelInApp},
		{TmplWorkflowReviewed, ChannelEmail},
		{TmplWorkflowReviewed, ChannelInApp},
		{TmplWorkflowApproved, ChannelEmail},
		{TmplWorkflowApproved, ChannelInApp},
		{TmplWorkflowApproved2, ChannelEmail},
		{TmplWorkflowApproved2, ChannelInApp},
		{TmplWorkflowRejected, ChannelEmail},
		{TmplWorkflowRejected, ChannelInApp},
		{TmplJobCompleted, ChannelEmail},
		{TmplJobCompleted, ChannelInApp},
		{TmplJobFailed, ChannelEmail},
		{TmplJobFailed, ChannelInApp},
	}

	for _, tc := range cases {
		t.Run(string(tc.code)+"/"+string(tc.channel), func(t *testing.T) {
			tmpl, err := store.Load(ctx, tc.code, tc.channel, "id-ID")
			if err != nil {
				t.Fatalf("Load(%s, %s) error = %v", tc.code, tc.channel, err)
			}
			if tmpl.BodyTemplate == "" {
				t.Errorf("BodyTemplate kosong untuk %s/%s", tc.code, tc.channel)
			}
			if tmpl.Channel != tc.channel {
				t.Errorf("Channel = %s, want %s", tmpl.Channel, tc.channel)
			}
		})
	}
}

// TestInMemoryTemplateStore_Load_NotFound memverifikasi error ketika template tidak ada.
func TestInMemoryTemplateStore_Load_NotFound(t *testing.T) {
	store := NewInMemoryTemplateStore()
	ctx := context.Background()

	_, err := store.Load(ctx, "NON_EXISTENT_TEMPLATE", ChannelEmail, "id-ID")
	if err == nil {
		t.Fatal("Load harus mengembalikan error untuk template yang tidak ada")
	}
}

// TestRenderTemplate_WorkflowSubmitted memverifikasi pesan SPESIFIK sesuai ux-patterns.md §2.2.
// DILARANG pesan generic "Berhasil"/"Gagal".
func TestRenderTemplate_WorkflowSubmitted(t *testing.T) {
	store := NewInMemoryTemplateStore()
	ctx := context.Background()

	vars := TemplateVars{
		"EntityLabel": "Penempatan Deposito",
		"EntityKode":  "DP-0042",
		"EntityType":  "DEPOSITO",
		"MakerName":   "Budi Santoso",
		"ActionURL":   "https://blips.tugu-re.com/penempatan/abc-123",
	}

	for _, ch := range []Channel{ChannelEmail, ChannelInApp} {
		tmpl, err := store.Load(ctx, TmplWorkflowSubmitted, ch, "id-ID")
		if err != nil {
			t.Fatalf("Load error: %v", err)
		}

		_, body, err := RenderTemplate(tmpl, vars)
		if err != nil {
			t.Fatalf("RenderTemplate error: %v", err)
		}

		// Pesan harus spesifik: menyebut kode entitas dan maker name.
		for _, want := range []string{"DP-0042", "Budi Santoso"} {
			if !contains(body, want) {
				t.Errorf("channel=%s: body tidak mengandung %q\nbody:\n%s", ch, want, body)
			}
		}

		// Pesan TIDAK boleh generic.
		for _, forbidden := range []string{"Berhasil", "Gagal"} {
			if contains(body, forbidden) {
				t.Errorf("channel=%s: body mengandung pesan generic %q — melanggar ux-patterns.md §2.2", ch, forbidden)
			}
		}
	}
}

// TestRenderTemplate_WorkflowRejected memverifikasi reject comment muncul di pesan.
func TestRenderTemplate_WorkflowRejected(t *testing.T) {
	store := NewInMemoryTemplateStore()
	ctx := context.Background()

	vars := TemplateVars{
		"EntityLabel":   "Obligasi Korporasi",
		"EntityKode":    "INST-001234",
		"EntityType":    "OBLIGASI",
		"MakerName":     "Citra Dewi",
		"RejectorName":  "Ahmad Reviewer",
		"RejectComment": "Dokumen pendukung belum lengkap, harap lampirkan bukti transaksi.",
		"ActionURL":     "https://blips.tugu-re.com/instrumen/inst-001234",
	}

	for _, ch := range []Channel{ChannelEmail, ChannelInApp} {
		tmpl, err := store.Load(ctx, TmplWorkflowRejected, ch, "id-ID")
		if err != nil {
			t.Fatalf("Load error: %v", err)
		}

		_, body, err := RenderTemplate(tmpl, vars)
		if err != nil {
			t.Fatalf("RenderTemplate error: %v", err)
		}

		// Harus menyebut kode entitas, rejector, dan alasan.
		for _, want := range []string{"INST-001234", "Ahmad Reviewer", "belum lengkap"} {
			if !contains(body, want) {
				t.Errorf("channel=%s: body tidak mengandung %q\nbody:\n%s", ch, want, body)
			}
		}
	}
}

// TestRenderTemplate_JobFailed memverifikasi pesan job gagal mengandung TraceID.
func TestRenderTemplate_JobFailed(t *testing.T) {
	store := NewInMemoryTemplateStore()
	ctx := context.Background()

	vars := TemplateVars{
		"JobType":      "ECL_CALC_RUN",
		"JobLabel":     "ECL Calc Run — Periode Juni 2026",
		"FailedAt":     "2026-06-02T10:35:00+07:00",
		"ErrorCode":    "INTERNAL",
		"ErrorMessage": "Divide by zero di EAD calculation stage 3",
		"TraceID":      "abc123def456",
		"ActionURL":    "https://blips.tugu-re.com/jobs/job_01HXYZ",
	}

	for _, ch := range []Channel{ChannelEmail, ChannelInApp} {
		tmpl, err := store.Load(ctx, TmplJobFailed, ch, "id-ID")
		if err != nil {
			t.Fatalf("Load error: %v", err)
		}

		_, body, err := RenderTemplate(tmpl, vars)
		if err != nil {
			t.Fatalf("RenderTemplate error: %v", err)
		}

		// Email template mengandung JobType secara eksplisit; INAPP template mengandung JobLabel.
		// Keduanya harus mengandung TraceID.
		if !contains(body, "abc123def456") {
			t.Errorf("channel=%s: body tidak mengandung traceId %q\nbody:\n%s", ch, "abc123def456", body)
		}
		// Setidaknya salah satu identifier job muncul.
		hasJobRef := contains(body, "ECL_CALC_RUN") || contains(body, "ECL Calc Run")
		if !hasJobRef {
			t.Errorf("channel=%s: body tidak mengandung referensi job\nbody:\n%s", ch, body)
		}
	}
}

// TestRenderTemplate_MissingVar memverifikasi template tidak error untuk variabel hilang
// (text/template Option("missingkey=zero") menggantikan dengan string kosong).
func TestRenderTemplate_MissingVar(t *testing.T) {
	store := NewInMemoryTemplateStore()
	ctx := context.Background()

	tmpl, _ := store.Load(ctx, TmplWorkflowSubmitted, ChannelInApp, "id-ID")
	// Sengaja kirim vars kosong — tidak boleh error.
	_, body, err := RenderTemplate(tmpl, TemplateVars{})
	if err != nil {
		t.Fatalf("RenderTemplate dengan vars kosong harus tidak error, got: %v", err)
	}
	if body == "" {
		t.Error("body tidak boleh kosong meski vars kosong")
	}
}

// TestDryRunMailer memverifikasi bahwa mailer tidak error saat SMTP_HOST kosong.
func TestDryRunMailer(t *testing.T) {
	cfg := SMTPConfig{Host: ""} // kosong = dry-run
	mailer := NewMailer(cfg, nil)

	if !mailer.cfg.IsDryRun() {
		t.Error("IsDryRun() harus true saat Host kosong")
	}

	err := mailer.SendEmail([]string{"test@example.com"}, "Test Subject", "Test Body")
	if err != nil {
		t.Errorf("dry-run SendEmail harus tidak error, got: %v", err)
	}
}

// TestLogInAppSink_Store memverifikasi stub tidak error.
func TestLogInAppSink_Store(t *testing.T) {
	sink := NewLogInAppSink(nil)
	err := sink.Store(context.Background(),
		[]string{"user-uuid-1", "user-uuid-2"},
		"DP-0042 menunggu review Anda.")
	if err != nil {
		t.Errorf("LogInAppSink.Store harus tidak error, got: %v", err)
	}
}

// TestService_Deliver_Email memverifikasi deliver ke EMAIL channel (dry-run mode).
func TestService_Deliver_Email(t *testing.T) {
	mailer := NewMailer(SMTPConfig{Host: ""}, nil) // dry-run
	sink := NewLogInAppSink(nil)
	store := NewInMemoryTemplateStore()
	svc := NewService(nil, store, mailer, sink, nil)

	req := DeliveryRequest{
		TemplateCode:    TmplWorkflowSubmitted,
		Channel:         ChannelEmail,
		RecipientEmails: []string{"reviewer@tugu-re.com"},
		Vars: TemplateVars{
			"EntityLabel": "Deposito BCA",
			"EntityKode":  "DP-0099",
			"EntityType":  "DEPOSITO",
			"MakerName":   "Rina Maker",
			"ActionURL":   "https://blips.tugu-re.com/penempatan/dp-0099",
		},
	}

	if err := svc.Deliver(context.Background(), req); err != nil {
		t.Errorf("Deliver EMAIL (dry-run) error = %v", err)
	}
}

// TestService_Deliver_InApp memverifikasi deliver ke INAPP channel.
func TestService_Deliver_InApp(t *testing.T) {
	mailer := NewMailer(SMTPConfig{Host: ""}, nil)
	sink := NewLogInAppSink(nil)
	store := NewInMemoryTemplateStore()
	svc := NewService(nil, store, mailer, sink, nil)

	req := DeliveryRequest{
		TemplateCode:     TmplWorkflowApproved,
		Channel:          ChannelInApp,
		RecipientUserIDs: []string{"00000000-0000-0000-0000-000000000002"},
		Vars: TemplateVars{
			"EntityLabel":  "Penempatan Deposito",
			"EntityKode":   "DP-0042",
			"EntityType":   "DEPOSITO",
			"MakerName":    "Budi Maker",
			"ApproverName": "Sari Approver",
			"ActionURL":    "https://blips.tugu-re.com/penempatan/dp-0042",
		},
	}

	if err := svc.Deliver(context.Background(), req); err != nil {
		t.Errorf("Deliver INAPP error = %v", err)
	}
}

// TestService_Deliver_UnknownChannel memverifikasi error untuk channel tidak dikenal.
func TestService_Deliver_UnknownChannel(t *testing.T) {
	svc := NewService(nil, NewInMemoryTemplateStore(), NewMailer(SMTPConfig{}, nil), NewLogInAppSink(nil), nil)

	req := DeliveryRequest{
		TemplateCode: TmplWorkflowSubmitted,
		Channel:      "TELEGRAM", // tidak dikenal
		Vars:         TemplateVars{},
	}

	err := svc.Deliver(context.Background(), req)
	if err == nil {
		t.Error("Deliver channel tidak dikenal harus mengembalikan error")
	}
}

// TestNotifyWorkflowTransition_NonFatal memverifikasi bahwa Enqueue error tidak panic
// (hanya log warn, tidak fatal).
func TestNotifyWorkflowTransition_NonFatal(t *testing.T) {
	// Service dengan nil asynq client = sync mode, dan channel yang tidak valid
	// agar Deliver mengembalikan error.
	svc := NewService(nil, NewInMemoryTemplateStore(), NewMailer(SMTPConfig{}, nil), NewLogInAppSink(nil), nil)

	// Kirim request dengan channel tidak valid — harus tidak panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NotifyWorkflowTransition panic: %v", r)
		}
	}()

	svc.NotifyWorkflowTransition(context.Background(), WorkflowTransitionParams{
		TemplateCode:    TmplWorkflowSubmitted,
		RecipientEmails: []string{"reviewer@tugu-re.com"},
		Vars: TemplateVars{
			"EntityLabel": "Test",
			"EntityKode":  "TEST-001",
			"EntityType":  "TEST",
			"MakerName":   "Test Maker",
			"ActionURL":   "https://example.com",
		},
		TraceID: "test-trace-id",
	})
}

// contains adalah helper untuk cek substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		indexInString(s, substr) >= 0)
}

func indexInString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
