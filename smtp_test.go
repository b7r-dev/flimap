package main

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/quotedprintable"
	"reflect"
	"strings"
	"testing"
)

var errNoBody = errors.New("message body not found")

func TestSendMessage(t *testing.T) {
	env := newTestEnv(t)
	to := []string{"alice@example.com", "bob@example.com"}
	cc := []string{"carol@example.com"}
	bcc := []string{"dave@example.com"}

	res, err := env.svc.SendMessage(to, "Hello from the test suite", "Line one.\r\nLine two.", cc, bcc)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !res.Sent {
		t.Fatal("expected Sent=true")
	}
	wantRecipients := []string{"alice@example.com", "bob@example.com", "carol@example.com", "dave@example.com"}
	if !reflect.DeepEqual(res.Recipients, wantRecipients) {
		t.Fatalf("unexpected Recipients: %+v", res.Recipients)
	}

	sent := env.smtpSrv.sentMessages(t)
	if len(sent) != 1 {
		t.Fatalf("SMTP server received %d messages, want 1", len(sent))
	}
	m := sent[0]
	if m.From != testUsername {
		t.Fatalf("MAIL FROM = %q, want %q", m.From, testUsername)
	}
	if !reflect.DeepEqual(m.Recipients, wantRecipients) {
		t.Fatalf("RCPT TO = %+v, want %+v", m.Recipients, wantRecipients)
	}
	// Credentials must flow from the config to the SMTP AUTH.
	if m.AuthUser != testUsername || m.AuthPass != testPassword {
		t.Fatalf("unexpected AUTH identity %q/%q", m.AuthUser, m.AuthPass)
	}

	text := string(m.Data)
	for _, want := range []string{
		"From: " + testUsername,
		"To: alice@example.com, bob@example.com",
		"Cc: carol@example.com",
		"MIME-Version: 1.0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("message missing header %q:\n%s", want, text)
		}
	}
	// BCC recipients get the message via RCPT TO only — never in headers.
	if strings.Contains(text, "dave@example.com") {
		t.Fatalf("BCC recipient leaked into message headers:\n%s", text)
	}
	if !strings.Contains(text, "Message-ID: <") {
		t.Fatalf("message missing Message-ID:\n%s", text)
	}

	decoder := new(mime.WordDecoder)
	gotSubject, err := decoder.DecodeHeader(headerValue(t, m.Data, "Subject"))
	if err != nil {
		t.Fatalf("decoding Subject: %v", err)
	}
	if gotSubject != "Hello from the test suite" {
		t.Fatalf("Subject = %q, want %q", gotSubject, "Hello from the test suite")
	}
	gotBody, err := decodeQPBody(m.Data)
	if err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	// quotedprintable.Writer terminates the final line with CRLF.
	if gotBody != "Line one.\r\nLine two.\r\n" {
		t.Fatalf("body = %q, want %q", gotBody, "Line one.\r\nLine two.\r\n")
	}
}

func TestSendMessageNonASCII(t *testing.T) {
	env := newTestEnv(t)
	subject := "Grüße aus Wien"
	body := "Umlaute: äöü ß\r\nZweite Zeile mit — Gedankenstrich."

	if _, err := env.svc.SendMessage([]string{"x@example.com"}, subject, body, nil, nil); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	sent := env.smtpSrv.sentMessages(t)
	if len(sent) != 1 {
		t.Fatalf("SMTP server received %d messages, want 1", len(sent))
	}
	m := sent[0]

	subjectHeader := headerValue(t, m.Data, "Subject")
	if !strings.Contains(subjectHeader, "=?utf-8?q?") {
		t.Fatalf("Subject not RFC 2047 encoded: %q", subjectHeader)
	}
	decoder := new(mime.WordDecoder)
	gotSubject, err := decoder.DecodeHeader(subjectHeader)
	if err != nil {
		t.Fatalf("decoding Subject: %v", err)
	}
	if gotSubject != subject {
		t.Fatalf("Subject = %q, want %q", gotSubject, subject)
	}
	gotBody, err := decodeQPBody(m.Data)
	if err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	wantBody := body + "\r\n" // quotedprintable.Writer terminates the final line
	if gotBody != wantBody {
		t.Fatalf("body = %q, want %q", gotBody, wantBody)
	}
}

// Sending without recipients must fail before anything reaches the wire.
func TestSendMessageRequiresRecipient(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.svc.SendMessage(nil, "subj", "body", nil, nil); err == nil {
		t.Fatal("expected SendMessage without recipients to fail")
	}
	if sent := env.smtpSrv.sentMessages(t); len(sent) != 0 {
		t.Fatalf("SMTP server received %d messages, want 0", len(sent))
	}
}

// An SMTP auth failure must surface as an error, not a silent success.
func TestSendMessageSMTPAuthFailure(t *testing.T) {
	env := newTestEnv(t)
	env.smtpSrv.setRejectAuth(true)

	if _, err := env.svc.SendMessage([]string{"x@example.com"}, "subj", "body", nil, nil); err == nil {
		t.Fatal("expected SendMessage to fail when SMTP auth is rejected")
	} else if !strings.Contains(err.Error(), "auth") {
		t.Fatalf("expected auth error, got: %v", err)
	}
	if sent := env.smtpSrv.sentMessages(t); len(sent) != 0 {
		t.Fatalf("SMTP server accepted %d messages despite auth failure", len(sent))
	}
}

func TestComposeMessage(t *testing.T) {
	msg, err := composeMessage(
		"me@example.com",
		[]string{"a@example.com", "b@example.com"},
		[]string{"c@example.com"},
		[]string{"secret@example.com"},
		"Sübject",
		"the body",
	)
	if err != nil {
		t.Fatalf("composeMessage: %v", err)
	}
	text := string(msg)

	for _, want := range []string{
		"From: me@example.com\r\n",
		"To: a@example.com, b@example.com\r\n",
		"Cc: c@example.com\r\n",
		"Message-ID: <",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("composed message missing %q:\n%s", want, text)
		}
	}
	// BCC must never appear in the composed headers.
	if strings.Contains(text, "secret@example.com") {
		t.Fatalf("BCC address leaked into composed message:\n%s", text)
	}

	decoder := new(mime.WordDecoder)
	gotSubject, err := decoder.DecodeHeader(headerValue(t, msg, "Subject"))
	if err != nil {
		t.Fatalf("decoding Subject: %v", err)
	}
	if gotSubject != "Sübject" {
		t.Fatalf("Subject = %q, want %q", gotSubject, "Sübject")
	}
	gotBody, err := decodeQPBody(msg)
	if err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if gotBody != "the body" {
		t.Fatalf("body = %q, want %q", gotBody, "the body")
	}
}

// headerValue returns the value of a header from the message's header block.
func headerValue(t *testing.T, msg []byte, key string) string {
	t.Helper()
	headers, _, ok := bytes.Cut(msg, []byte("\r\n\r\n"))
	if !ok {
		t.Fatalf("message has no header/body separator:\n%s", msg)
	}
	for _, line := range strings.Split(string(headers), "\r\n") {
		name, val, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), key) {
			return strings.TrimSpace(val)
		}
	}
	t.Fatalf("header %q not found in:\n%s", key, headers)
	return ""
}

// decodeQPBody extracts and decodes the quoted-printable body of a message.
func decodeQPBody(msg []byte) (string, error) {
	_, body, ok := bytes.Cut(msg, []byte("\r\n\r\n"))
	if !ok {
		return "", errNoBody
	}
	r := quotedprintable.NewReader(bytes.NewReader(body))
	decoded, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
