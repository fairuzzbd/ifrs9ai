package reporting

// smtp_helpers_test.go — white-box tests for unexported smtp helpers.
// Uses package-level access (same package, not _test).

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── buildMIMEMessage ─────────────────────────────────────────────────────────

func TestBuildMIMEMessage_WithoutAttachment(t *testing.T) {
	msg, err := buildMIMEMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"Test Subject",
		"Test body",
		"", nil, // no attachment
	)
	require.NoError(t, err)
	s := string(msg)
	assert.Contains(t, s, "From:")
	assert.Contains(t, s, "To:")
	assert.Contains(t, s, "Subject: Test Subject")
	assert.Contains(t, s, "text/plain")
	assert.Contains(t, s, "Test body")
	assert.NotContains(t, s, "multipart")
}

func TestBuildMIMEMessage_WithCSVAttachment(t *testing.T) {
	attachment := []byte("col1,col2\nval1,val2\n")
	msg, err := buildMIMEMessage(
		"from@tugu-re.com",
		[]string{"to1@x.com", "to2@x.com"},
		"Report",
		"Body text",
		"report.csv", attachment,
	)
	require.NoError(t, err)
	s := string(msg)
	assert.Contains(t, s, "multipart/mixed")
	assert.Contains(t, s, "report.csv")
	assert.Contains(t, s, "text/csv")
	assert.Contains(t, s, "Content-Transfer-Encoding: base64")
}

func TestBuildMIMEMessage_WithXLSXAttachment(t *testing.T) {
	attachment := []byte("XLSX content placeholder")
	msg, err := buildMIMEMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"XLSX Report",
		"Body",
		"report.xlsx", attachment,
	)
	require.NoError(t, err)
	s := string(msg)
	assert.Contains(t, s, "spreadsheetml")
}

func TestBuildMIMEMessage_WithPDFAttachment(t *testing.T) {
	attachment := []byte("%PDF-placeholder")
	msg, err := buildMIMEMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"PDF Report",
		"Body",
		"report.pdf", attachment,
	)
	require.NoError(t, err)
	s := string(msg)
	assert.Contains(t, s, "application/pdf")
}

func TestBuildMIMEMessage_UnknownExtension(t *testing.T) {
	attachment := []byte("data")
	msg, err := buildMIMEMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"Subject",
		"Body",
		"data.bin", attachment,
	)
	require.NoError(t, err)
	s := string(msg)
	assert.Contains(t, s, "application/octet-stream")
}

// ─── attachmentMIMEType ───────────────────────────────────────────────────────

func TestAttachmentMIMEType_CSV(t *testing.T) {
	assert.Equal(t, "text/csv; charset=UTF-8", attachmentMIMEType("file.csv"))
}

func TestAttachmentMIMEType_XLSX(t *testing.T) {
	assert.Contains(t, attachmentMIMEType("file.xlsx"), "spreadsheetml")
}

func TestAttachmentMIMEType_PDF(t *testing.T) {
	assert.Equal(t, "application/pdf", attachmentMIMEType("file.pdf"))
}

func TestAttachmentMIMEType_Unknown(t *testing.T) {
	assert.Equal(t, "application/octet-stream", attachmentMIMEType("file.bin"))
}

// ─── wrapBase64 ───────────────────────────────────────────────────────────────

func TestWrapBase64_ShortData(t *testing.T) {
	data := []byte("hello")
	encoded := wrapBase64(data)
	// Should not be empty and should have CRLF terminator
	assert.NotEmpty(t, encoded)
	assert.Contains(t, encoded, "\r\n")
}

func TestWrapBase64_LongData(t *testing.T) {
	// More than 76 encoded chars → should wrap
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i % 255)
	}
	encoded := wrapBase64(data)
	lines := strings.Split(encoded, "\r\n")
	// Each non-empty line should be ≤ 76 chars
	for _, line := range lines {
		if line != "" {
			assert.LessOrEqual(t, len(line), 76, "line too long: %q", line)
		}
	}
}

// ─── SMTPClient.SendEmail — empty recipients short-circuit ───────────────────

func TestSMTPClient_SendEmail_EmptyRecipients(t *testing.T) {
	client := NewSMTPClient(SMTPConfig{
		Host: "localhost",
		Port: "587",
		From: "test@tugu-re.com",
	}, nil)
	// Empty recipients → returns nil immediately (no network call)
	err := client.SendEmail(context.Background(), nil, "subj", "body", "", nil)
	assert.NoError(t, err)
}

func TestSMTPClient_SendEmail_EmptyRecipientsSlice(t *testing.T) {
	client := NewSMTPClient(SMTPConfig{
		Host: "localhost",
		Port: "587",
		From: "test@tugu-re.com",
	}, nil)
	err := client.SendEmail(context.Background(), []string{}, "s", "b", "", nil)
	assert.NoError(t, err)
}
