package services

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

type EmailSender interface {
	Send(ctx context.Context, recipient, subject, body string) error
}

type SMTPEmailSender struct {
	host, port, username, password, from string
}

func NewSMTPEmailSender(host, port, username, password, from string) *SMTPEmailSender {
	return &SMTPEmailSender{host: host, port: port, username: username, password: password, from: from}
}

func (s *SMTPEmailSender) Send(ctx context.Context, recipient, subject, body string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.host == "" || s.from == "" {
		return fmt.Errorf("email transport is not configured")
	}
	if strings.ContainsAny(recipient+s.from+subject, "\r\n") {
		return fmt.Errorf("invalid email message")
	}
	message := fmt.Sprintf(
		"To: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		recipient, subject, body,
	)
	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}
	return smtp.SendMail(s.host+":"+s.port, auth, s.from, []string{recipient}, []byte(message))
}
