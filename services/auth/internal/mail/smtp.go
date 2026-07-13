package mail

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/smtp"

	"github.com/teamboard/services/auth/internal/domain"
)

//go:embed templates/*.html
var templates embed.FS

// Config holds SMTP connection parameters.
type Config struct {
	Host     string
	Port     int
	From     string
	Username string
	Password string
}

type smtpSender struct {
	cfg  Config
	tmpl *template.Template
}

// NewSMTPSender constructs a MailSender that delivers via SMTP.
func NewSMTPSender(cfg Config) (domain.MailSender, error) {
	tmpl, err := template.ParseFS(templates, "templates/password_reset.html")
	if err != nil {
		return nil, fmt.Errorf("parse email templates: %w", err)
	}
	return &smtpSender{cfg: cfg, tmpl: tmpl}, nil
}

var _ domain.MailSender = (*smtpSender)(nil)

func (s *smtpSender) SendPasswordReset(ctx context.Context, toEmail, resetURL string) error {
	var body bytes.Buffer
	if err := s.tmpl.Execute(&body, map[string]string{
		"ResetURL": resetURL,
		"Email":    toEmail,
	}); err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	msg := buildMessage(s.cfg.From, toEmail, "Reset your TeamBoard password", body.String())

	var auth smtp.Auth
	if s.cfg.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}

	return smtp.SendMail(addr, auth, s.cfg.From, []string{toEmail}, []byte(msg))
}

func buildMessage(from, to, subject, htmlBody string) string {
	return fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		from, to, subject, htmlBody,
	)
}
