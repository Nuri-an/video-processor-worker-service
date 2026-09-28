package main

import (
	"fmt"
	"net/smtp"
)

type SMTPNotifier struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

func (n SMTPNotifier) NotifyProcessingError(recipient string, job VideoJob, processingError error) error {
	if recipient == "" {
		return fmt.Errorf("destinatario de notificacao nao configurado")
	}
	address := n.Host + ":" + n.Port
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Falha no processamento do video %s\r\n\r\nO processamento falhou: %v\r\n", n.From, recipient, job.ID, processingError)
	var auth smtp.Auth
	if n.Username != "" || n.Password != "" {
		auth = smtp.PlainAuth("", n.Username, n.Password, n.Host)
	}
	return smtp.SendMail(address, auth, n.From, []string{recipient}, []byte(body))
}

func smtpNotifierConfig() SMTPNotifier {
	return SMTPNotifier{
		Host:     envOr("SMTP_HOST", "localhost"),
		Port:     envOr("SMTP_PORT", "25"),
		Username: envOr("SMTP_USERNAME", ""),
		Password: envOr("SMTP_PASSWORD", ""),
		From:     envOr("SMTP_FROM", "noreply@video-processor.local"),
	}
}