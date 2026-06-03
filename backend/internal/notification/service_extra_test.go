package notification

import (
	"context"
	"testing"
)

// TestNotifyJobCompleted_NonFatal memverifikasi NotifyJobCompleted tidak panic.
func TestNotifyJobCompleted_NonFatal(t *testing.T) {
	mailer := NewMailer(SMTPConfig{Host: ""}, nil)
	sink := NewLogInAppSink(nil)
	svc := NewService(nil, NewInMemoryTemplateStore(), mailer, sink, nil)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NotifyJobCompleted panic: %v", r)
		}
	}()

	svc.NotifyJobCompleted(context.Background(), JobNotificationParams{
		JobType:          "ECL_CALC_RUN",
		JobLabel:         "ECL Calc Run — Periode Juni 2026",
		CompletedAt:      "2026-06-02T10:35:00+07:00",
		ResultSummary:    "Total ECL: Rp 1.234.567.890,00. 2600 instrumen diproses.",
		ActionURL:        "https://blips.tugu-re.com/ecl/run/abc123",
		RecipientEmails:  []string{"risk@tugu-re.com"},
		RecipientUserIDs: []string{"00000000-0000-0000-0000-000000000002"},
		TraceID:          "trace-xyz",
	})
}

// TestNotifyJobFailed_NonFatal memverifikasi NotifyJobFailed tidak panic.
func TestNotifyJobFailed_NonFatal(t *testing.T) {
	mailer := NewMailer(SMTPConfig{Host: ""}, nil)
	sink := NewLogInAppSink(nil)
	svc := NewService(nil, NewInMemoryTemplateStore(), mailer, sink, nil)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NotifyJobFailed panic: %v", r)
		}
	}()

	svc.NotifyJobFailed(context.Background(), JobNotificationParams{
		JobType:          "ECL_CALC_RUN",
		JobLabel:         "ECL Calc Run — Periode Juni 2026",
		CompletedAt:      "2026-06-02T10:35:00+07:00",
		ErrorCode:        "INTERNAL",
		ErrorMessage:     "Divide by zero",
		ActionURL:        "https://blips.tugu-re.com/jobs/job123",
		RecipientEmails:  []string{"it-admin@tugu-re.com", "risk@tugu-re.com"},
		RecipientUserIDs: []string{"00000000-0000-0000-0000-000000000001"},
		TraceID:          "trace-abc",
	})
}

// TestNotifyJobCompleted_InAppOnly memverifikasi NotifyJobCompleted tanpa email.
func TestNotifyJobCompleted_InAppOnly(t *testing.T) {
	sink := NewLogInAppSink(nil)
	svc := NewService(nil, NewInMemoryTemplateStore(),
		NewMailer(SMTPConfig{Host: ""}, nil), sink, nil)

	svc.NotifyJobCompleted(context.Background(), JobNotificationParams{
		JobType:          "EXPORT_CSV",
		JobLabel:         "Export CSV Instrumen",
		CompletedAt:      "2026-06-02T11:00:00+07:00",
		ResultSummary:    "5000 baris diekspor.",
		ActionURL:        "https://blips.tugu-re.com/jobs/export-abc",
		RecipientEmails:  nil, // tidak ada email
		RecipientUserIDs: []string{"00000000-0000-0000-0000-000000000002"},
	})
}

// TestBuildWorkflowRequests_BothChannels memverifikasi kedua channel di-generate.
func TestBuildWorkflowRequests_BothChannels(t *testing.T) {
	p := WorkflowTransitionParams{
		TemplateCode:     TmplWorkflowSubmitted,
		RecipientEmails:  []string{"reviewer@tugu-re.com"},
		RecipientUserIDs: []string{"user-uuid-1"},
		Vars:             TemplateVars{"EntityLabel": "Test"},
		TraceID:          "trace-1",
	}
	reqs := buildWorkflowRequests(p)
	if len(reqs) != 2 {
		t.Errorf("buildWorkflowRequests dengan kedua channel = %d requests, want 2", len(reqs))
	}
	channels := map[Channel]bool{}
	for _, r := range reqs {
		channels[r.Channel] = true
	}
	if !channels[ChannelEmail] {
		t.Error("harus ada request EMAIL")
	}
	if !channels[ChannelInApp] {
		t.Error("harus ada request INAPP")
	}
}

// TestBuildWorkflowRequests_EmailOnly memverifikasi hanya EMAIL jika no user IDs.
func TestBuildWorkflowRequests_EmailOnly(t *testing.T) {
	p := WorkflowTransitionParams{
		TemplateCode:    TmplWorkflowApproved,
		RecipientEmails: []string{"maker@tugu-re.com"},
		Vars:            TemplateVars{},
	}
	reqs := buildWorkflowRequests(p)
	if len(reqs) != 1 {
		t.Errorf("want 1 request (EMAIL only), got %d", len(reqs))
	}
	if reqs[0].Channel != ChannelEmail {
		t.Errorf("channel = %s, want EMAIL", reqs[0].Channel)
	}
}

// TestBuildWorkflowRequests_InAppOnly memverifikasi hanya INAPP jika no emails.
func TestBuildWorkflowRequests_InAppOnly(t *testing.T) {
	p := WorkflowTransitionParams{
		TemplateCode:     TmplWorkflowRejected,
		RecipientUserIDs: []string{"user-1"},
		Vars:             TemplateVars{},
	}
	reqs := buildWorkflowRequests(p)
	if len(reqs) != 1 {
		t.Errorf("want 1 request (INAPP only), got %d", len(reqs))
	}
	if reqs[0].Channel != ChannelInApp {
		t.Errorf("channel = %s, want INAPP", reqs[0].Channel)
	}
}

// TestBuildWorkflowRequests_NoneEmpty memverifikasi tidak ada request jika tidak ada penerima.
func TestBuildWorkflowRequests_NoneEmpty(t *testing.T) {
	p := WorkflowTransitionParams{
		TemplateCode: TmplWorkflowSubmitted,
		Vars:         TemplateVars{},
	}
	reqs := buildWorkflowRequests(p)
	if len(reqs) != 0 {
		t.Errorf("want 0 requests tanpa penerima, got %d", len(reqs))
	}
}

// TestBuildMIMEMessage memverifikasi format MIME message.
func TestBuildMIMEMessage(t *testing.T) {
	msg := buildMIMEMessage(
		"BLIPS <noreply@blips.tugu-re.com>",
		[]string{"user@tugu-re.com"},
		"Test Subject",
		"Test body content",
	)

	for _, want := range []string{
		"From: BLIPS <noreply@blips.tugu-re.com>",
		"To: user@tugu-re.com",
		"Subject: Test Subject",
		"Content-Type: text/plain; charset=UTF-8",
		"Test body content",
	} {
		if !contains(msg, want) {
			t.Errorf("MIME message tidak mengandung %q\nmessage:\n%s", want, msg)
		}
	}
}

// TestTruncate memverifikasi perilaku truncate.
func TestTruncate(t *testing.T) {
	s := "hello world this is a long string"
	if got := truncate(s, 5); got != "hello..." {
		t.Errorf("truncate(len>maxLen) = %q, want %q", got, "hello...")
	}
	if got := truncate("short", 100); got != "short" {
		t.Errorf("truncate(len<=maxLen) = %q, want %q", got, "short")
	}
	if got := truncate("", 10); got != "" {
		t.Errorf("truncate empty = %q, want %q", got, "")
	}
}
