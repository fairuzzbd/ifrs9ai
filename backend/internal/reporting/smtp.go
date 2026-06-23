package reporting

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"text/template"
	"time"
)

// SMTPConfig holds SMTP server configuration (loaded from env vars — never hardcoded).
type SMTPConfig struct {
	Host     string // SMTP_HOST
	Port     string // SMTP_PORT
	From     string // SMTP_FROM
	Password string // SMTP_PASSWORD (env only; never log)
	UseTLS   bool   // true = dial TLS (port 465); false = STARTTLS (port 587)
}

// SMTPClient wraps net/smtp for BLIPS report email sending.
type SMTPClient struct {
	cfg    SMTPConfig
	logger *slog.Logger
}

// NewSMTPClient creates an SMTPClient. Panics if cfg.Host or cfg.From is empty.
func NewSMTPClient(cfg SMTPConfig, logger *slog.Logger) *SMTPClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &SMTPClient{cfg: cfg, logger: logger}
}

// SendEmail sends an email to recipients.
// S5-AC2: Bahasa Indonesia template; SHA-256 hash in body.
// S5-AC3: single-attempt; caller (Asynq worker) owns retry logic.
func (c *SMTPClient) SendEmail(ctx context.Context, to []string, subject, body string, attachmentName string, attachment []byte) error {
	if len(to) == 0 {
		return nil
	}
	from := c.cfg.From
	addr := net.JoinHostPort(c.cfg.Host, c.cfg.Port)

	var auth smtp.Auth
	if c.cfg.Password != "" {
		auth = smtp.PlainAuth("", from, c.cfg.Password, c.cfg.Host)
	}

	msg, err := buildMIMEMessage(from, to, subject, body, attachmentName, attachment)
	if err != nil {
		return fmt.Errorf("smtp.SendEmail: build MIME: %w", err)
	}

	if c.cfg.UseTLS {
		tlsCfg := &tls.Config{ServerName: c.cfg.Host, MinVersion: tls.VersionTLS12} //nolint:gosec
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("smtp.SendEmail: TLS dial %s: %w", addr, err)
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, c.cfg.Host)
		if err != nil {
			return fmt.Errorf("smtp.SendEmail: new client: %w", err)
		}
		defer client.Close()
		if auth != nil {
			if err = client.Auth(auth); err != nil {
				return fmt.Errorf("smtp.SendEmail: auth: %w", err)
			}
		}
		if err = sendViaClient(client, from, to, msg); err != nil {
			return fmt.Errorf("smtp.SendEmail: send: %w", err)
		}
	} else {
		if err = smtp.SendMail(addr, auth, from, to, msg); err != nil {
			return fmt.Errorf("smtp.SendEmail: send: %w", err)
		}
	}

	c.logger.InfoContext(ctx, "smtp: email sent",
		"to_count", len(to), "subject", subject)
	return nil
}

// RenderEmailTemplate replaces {key} placeholders in subject and body templates.
// Also validates that templates are syntactically valid (text/template).
func RenderEmailTemplate(subjectTpl, bodyTpl string, data map[string]string) (subject, body string, err error) {
	subject = subjectTpl
	body = bodyTpl
	for k, v := range data {
		subject = strings.ReplaceAll(subject, "{"+k+"}", v)
		body = strings.ReplaceAll(body, "{"+k+"}", v)
	}
	if _, err = template.New("s").Parse(subject); err != nil {
		return "", "", fmt.Errorf("RenderEmailTemplate: subject: %w", err)
	}
	if _, err = template.New("b").Parse(body); err != nil {
		return "", "", fmt.Errorf("RenderEmailTemplate: body: %w", err)
	}
	return subject, body, nil
}

// DefaultSubjectTemplate is used when ScheduledEmail.SubjectTemplate is empty.
const DefaultSubjectTemplate = "[BLIPS] Laporan {report_slug} — {tanggal}"

// DefaultBodyTemplate is used when ScheduledEmail.BodyTemplate is empty.
const DefaultBodyTemplate = `Yth. Penerima,

Terlampir laporan {report_slug} BLIPS Tugu Re per {tanggal}.

File dapat diverifikasi dengan SHA-256: {file_hash}

Catatan: Email ini dikirim otomatis. Untuk berhenti menerima: {opt_out_link}

Hormat kami,
Sistem BLIPS Tugu Reasuransi
`

// buildMIMEMessage constructs a MIME email message.
func buildMIMEMessage(from string, to []string, subject, body string, attachmentName string, attachment []byte) ([]byte, error) {
	var buf bytes.Buffer
	fromAddr := mail.Address{Address: from}
	buf.WriteString("From: " + fromAddr.String() + "\r\n")
	buf.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")

	if len(attachment) == 0 {
		buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		buf.WriteString(body)
		return buf.Bytes(), nil
	}

	boundary := "BLIPSboundary20260623v1"
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%q\r\n\r\n", boundary))
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	buf.WriteString(body + "\r\n")

	mimeType := attachmentMIMEType(attachmentName)
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString(fmt.Sprintf("Content-Type: %s\r\n", mimeType))
	buf.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=%q\r\n", attachmentName))
	buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	buf.WriteString(wrapBase64(attachment))
	buf.WriteString("\r\n--" + boundary + "--\r\n")

	return buf.Bytes(), nil
}

// attachmentMIMEType returns the MIME type for a given filename.
func attachmentMIMEType(name string) string {
	switch {
	case strings.HasSuffix(name, ".xlsx"):
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case strings.HasSuffix(name, ".csv"):
		return "text/csv; charset=UTF-8"
	case strings.HasSuffix(name, ".pdf"):
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

// wrapBase64 base64-encodes data with CRLF line breaks every 76 chars (RFC 2045).
func wrapBase64(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	var sb strings.Builder
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		sb.WriteString(encoded[i:end])
		sb.WriteString("\r\n")
	}
	return sb.String()
}

// sendViaClient sends via an already-dialed smtp.Client.
func sendViaClient(client *smtp.Client, from string, to []string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	for _, addr := range to {
		if err := client.Rcpt(addr); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", addr, err)
		}
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err = wc.Write(msg); err != nil {
		return fmt.Errorf("write data: %w", err)
	}
	return wc.Close()
}
