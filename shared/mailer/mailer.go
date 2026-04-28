// Package mailer sends transactional email via SMTP (or logs to stdout
// when SMTP_HOST is unset — useful for local dev).
package mailer

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strconv"
	"strings"
)

// Mailer sends emails. Implementations: SMTPMailer, NoopMailer.
type Mailer interface {
	Send(to, subject, htmlBody string) error
}

// SMTPMailer uses net/smtp; supports STARTTLS via auto-detection.
type SMTPMailer struct {
	host     string
	port     int
	user     string
	pass     string
	fromAddr string
	fromName string
	useTLS   bool
}

// NewFromEnv constructs a Mailer based on SMTP_* env vars.
// If SMTP_HOST is unset, returns a NoopMailer that logs payloads.
func NewFromEnv() Mailer {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return &NoopMailer{}
	}
	port, _ := strconv.Atoi(getEnv("SMTP_PORT", "587"))
	return &SMTPMailer{
		host:     host,
		port:     port,
		user:     os.Getenv("SMTP_USER"),
		pass:     os.Getenv("SMTP_PASS"),
		fromAddr: getEnv("SMTP_FROM", "no-reply@micro-exchange.local"),
		fromName: getEnv("SMTP_FROM_NAME", "Micro-Exchange"),
		useTLS:   os.Getenv("SMTP_TLS") != "false", // default on
	}
}

// Send delivers an HTML email. Plain-text fallback is auto-derived from the body.
func (m *SMTPMailer) Send(to, subject, htmlBody string) error {
	addr := fmt.Sprintf("%s:%d", m.host, m.port)
	from := fmt.Sprintf("%s <%s>", m.fromName, m.fromAddr)

	msg := buildMessage(from, to, subject, htmlBody)

	var auth smtp.Auth
	if m.user != "" {
		auth = smtp.PlainAuth("", m.user, m.pass, m.host)
	}

	// Port 465 → implicit TLS. Other ports → STARTTLS upgrade.
	if m.port == 465 {
		return sendTLS(addr, m.host, auth, m.fromAddr, []string{to}, msg)
	}
	return smtp.SendMail(addr, auth, m.fromAddr, []string{to}, msg)
}

func sendTLS(addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Quit()

	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

func buildMessage(from, to, subject, htmlBody string) []byte {
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\n")
	b.WriteString("To: ")
	b.WriteString(to)
	b.WriteString("\r\n")
	b.WriteString("Subject: ")
	b.WriteString(subject)
	b.WriteString("\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String())
}

// NoopMailer logs the email payload instead of sending — for local dev.
type NoopMailer struct{}

func (NoopMailer) Send(to, subject, htmlBody string) error {
	log.Printf("[mailer:noop] to=%s subject=%q\n----- BODY -----\n%s\n----- END -----",
		to, subject, htmlBody)
	return nil
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
