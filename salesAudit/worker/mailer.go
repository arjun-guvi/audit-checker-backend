package worker

import (
	"fmt"
	"net/smtp"
	"strings"
)

// Mailer sends one HTML mail.
type Mailer interface {
	Configured() bool
	Send(to []string, subject, htmlBody string) error
}

// SMTPMailer sends through an SMTP server; settings come from SMTP_* env vars.
type SMTPMailer struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

func (m SMTPMailer) Configured() bool {
	return m.Host != "" && m.Username != "" && m.Password != "" && m.From != ""
}

func (m SMTPMailer) Send(to []string, subject, htmlBody string) error {
	message := "From: " + m.From + "\r\n" +
		"To: " + strings.Join(to, ",") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n" +
		htmlBody
	auth := smtp.PlainAuth("", m.Username, m.Password, m.Host)
	return smtp.SendMail(fmt.Sprintf("%s:%s", m.Host, m.Port), auth, m.From, to, []byte(message))
}
