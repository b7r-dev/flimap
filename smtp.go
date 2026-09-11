package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// SendMessageResult holds the result of sending a message.
type SendMessageResult struct {
	Sent       bool     `json:"sent"`
	Recipients []string `json:"recipients"`
}

// SendMessage composes and sends an email via SMTP.
func (s *MailService) SendMessage(to []string, subject, body string, cc, bcc []string) (*SendMessageResult, error) {
	if len(to) == 0 {
		return nil, fmt.Errorf("at least one recipient (to) is required")
	}

	// Collect all recipients for RCPT TO (includes cc and bcc)
	allRecipients := make([]string, 0, len(to)+len(cc)+len(bcc))
	allRecipients = append(allRecipients, to...)
	allRecipients = append(allRecipients, cc...)
	allRecipients = append(allRecipients, bcc...)

	// Compose RFC822 message
	msg, err := composeMessage(s.cfg.FromAddress, to, cc, bcc, subject, body)
	if err != nil {
		return nil, fmt.Errorf("composing message: %w", err)
	}

	// Send via SMTP with STARTTLS
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)
	if err := s.sendSMTP(addr, s.cfg.FromAddress, allRecipients, msg); err != nil {
		return nil, fmt.Errorf("sending SMTP: %w", err)
	}

	return &SendMessageResult{
		Sent:       true,
		Recipients: allRecipients,
	}, nil
}

// sendSMTP handles the SMTP connection with STARTTLS and PLAIN auth.
func (s *MailService) sendSMTP(addr string, from string, recipients []string, msg []byte) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("connecting to SMTP server: %w", err)
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, s.cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("creating SMTP client: %w", err)
	}
	defer c.Quit()

	// EHLO/HELO
	host := s.cfg.SMTPHost
	if err := c.Hello(host); err != nil {
		return fmt.Errorf("SMTP HELO: %w", err)
	}

	// STARTTLS
	if s.cfg.TLSMode != "none" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP server does not support STARTTLS")
		}
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         s.cfg.SMTPHost,
		}
		if err := c.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("SMTP STARTTLS: %w", err)
		}
	}

	// AUTH (PLAIN)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.SMTPHost)
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth: %w", err)
	}

	// MAIL FROM
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}

	// RCPT TO for each recipient
	for _, rcpt := range recipients {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("SMTP RCPT TO %s: %w", rcpt, err)
		}
	}

	// DATA
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("writing message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing DATA: %w", err)
	}

	return nil
}

// composeMessage builds an RFC822 message with proper headers and UTF-8 body.
func composeMessage(from string, to, cc, bcc []string, subject, body string) ([]byte, error) {
	var buf strings.Builder

	// Headers
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	if len(cc) > 0 {
		buf.WriteString("Cc: " + strings.Join(cc, ", ") + "\r\n")
	}
	// BCC is not included in headers (by design), but recipients get it via RCPT TO

	// Subject with RFC 2047 encoding for non-ASCII
	encodedSubject := mime.QEncoding.Encode("utf-8", subject)
	buf.WriteString("Subject: " + encodedSubject + "\r\n")

	buf.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")

	// Message-ID
	buf.WriteString(fmt.Sprintf("Message-ID: <%d.flymap@%s>\r\n", time.Now().UnixNano(), strings.Split(from, "@")[1]))

	// MIME version and content type
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")

	buf.WriteString("\r\n")

	// Body encoded as quoted-printable for UTF-8 safety
	qp := quotedprintable.NewWriter(&buf)
	if _, err := io.WriteString(qp, body); err != nil {
		return nil, fmt.Errorf("encoding body: %w", err)
	}
	if err := qp.Close(); err != nil {
		return nil, fmt.Errorf("closing QP encoder: %w", err)
	}

	return []byte(buf.String()), nil
}
