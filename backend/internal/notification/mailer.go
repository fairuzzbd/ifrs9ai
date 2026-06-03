package notification

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

// SMTPConfig adalah konfigurasi SMTP untuk pengiriman email.
// Nilai dibaca dari environment — TIDAK pernah hardcode.
type SMTPConfig struct {
	Host     string // SMTP_HOST
	Port     string // SMTP_PORT, default "587"
	Username string // SMTP_USERNAME
	Password string // SMTP_PASSWORD (dari KMS/Vault di prod)
	From     string // SMTP_FROM, mis. "BLIPS IFRS9 <noreply@blips.tugu-re.com>"
	UseTLS   bool   // SMTP_USE_TLS=true → STARTTLS
}

// IsDryRun mengembalikan true bila SMTP_HOST kosong (dev-safe mode).
func (c SMTPConfig) IsDryRun() bool {
	return strings.TrimSpace(c.Host) == ""
}

// Mailer mengirim email via SMTP.
// Bila SMTPConfig.IsDryRun() == true, tidak ada koneksi nyata — hanya log.
type Mailer struct {
	cfg    SMTPConfig
	logger *slog.Logger
}

// NewMailer membuat Mailer baru.
func NewMailer(cfg SMTPConfig, logger *slog.Logger) *Mailer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Mailer{cfg: cfg, logger: logger}
}

// SendEmail mengirim satu email.
// Bila dry-run, log pesan ke slog dan return nil (tidak error agar handler tidak terblok).
func (m *Mailer) SendEmail(to []string, subject, body string) error {
	if m.cfg.IsDryRun() {
		m.logger.Info("[SMTP dry-run] email would be sent",
			"to", strings.Join(to, ","),
			"subject", subject,
			"body_preview", truncate(body, 120),
		)
		return nil
	}

	addr := m.cfg.Host + ":" + m.cfg.Port
	msg := buildMIMEMessage(m.cfg.From, to, subject, body)

	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	}

	if m.cfg.UseTLS {
		tlsCfg := &tls.Config{
			ServerName: m.cfg.Host,
			MinVersion: tls.VersionTLS12,
		}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("mailer: TLS dial %s: %w", addr, err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, m.cfg.Host)
		if err != nil {
			return fmt.Errorf("mailer: SMTP client: %w", err)
		}
		defer client.Close()

		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("mailer: SMTP auth: %w", err)
			}
		}
		if err := client.Mail(m.cfg.From); err != nil {
			return fmt.Errorf("mailer: MAIL FROM: %w", err)
		}
		for _, rcpt := range to {
			if err := client.Rcpt(rcpt); err != nil {
				return fmt.Errorf("mailer: RCPT TO %s: %w", rcpt, err)
			}
		}
		wc, err := client.Data()
		if err != nil {
			return fmt.Errorf("mailer: DATA: %w", err)
		}
		if _, err = wc.Write([]byte(msg)); err != nil {
			return fmt.Errorf("mailer: write message: %w", err)
		}
		return wc.Close()
	}

	// STARTTLS / plain
	if err := smtp.SendMail(addr, auth, m.cfg.From, to, []byte(msg)); err != nil {
		return fmt.Errorf("mailer: send mail: %w", err)
	}
	return nil
}

// buildMIMEMessage membangun email MIME sederhana (text/plain UTF-8).
func buildMIMEMessage(from string, to []string, subject, body string) string {
	var sb strings.Builder
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)
	return sb.String()
}

// truncate memotong string ke maxLen karakter untuk preview log.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
