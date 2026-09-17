package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

const (
	testUsername = "user@test.local"
	testPassword = "secret"
)

// testEnv wires a MailService to an in-process IMAP server (go-imap's
// imapmemserver behind imapserver with STARTTLS) and a minimal fake SMTP
// server, both on loopback. No real mail server or mailbox is ever touched.
type testEnv struct {
	svc     *MailService
	cfg     *Config
	user    *imapmemserver.User
	imapSrv *imapserver.Server
	smtpSrv *fakeSMTPServer
}

// newTestEnv starts a test environment with capabilities matching what
// Protonmail Bridge advertises (MOVE, plus UIDPLUS which the client uses in
// its MOVE fallback).
func newTestEnv(t *testing.T, extraCaps ...imap.Cap) *testEnv {
	t.Helper()
	caps := imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapMove: {}, imap.CapUIDPlus: {}}
	for _, c := range extraCaps {
		caps[c] = struct{}{}
	}
	return newTestEnvWithCapSet(t, caps)
}

// newTestEnvWithCaps starts a test environment advertising exactly the given
// capabilities, to exercise client fallback paths (e.g. MOVE without MOVE).
func newTestEnvWithCaps(t *testing.T, caps ...imap.Cap) *testEnv {
	t.Helper()
	capSet := imap.CapSet{imap.CapIMAP4rev1: {}}
	for _, c := range caps {
		capSet[c] = struct{}{}
	}
	return newTestEnvWithCapSet(t, capSet)
}

func newTestEnvWithCapSet(t *testing.T, caps imap.CapSet) *testEnv {
	t.Helper()

	cert := generateTLSCert(t)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	user := imapmemserver.NewUser(testUsername, testPassword)
	for _, folder := range []string{"INBOX", "Trash"} {
		if err := user.Create(folder, nil); err != nil {
			t.Fatalf("creating %s: %v", folder, err)
		}
	}
	memSrv := imapmemserver.New()
	memSrv.AddUser(user)

	imapSrv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return memSrv.NewSession(), nil, nil
		},
		Caps:      caps,
		TLSConfig: tlsConfig,
	})
	imapLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening for IMAP server: %v", err)
	}
	go imapSrv.Serve(imapLn)
	t.Cleanup(func() {
		imapLn.Close()
		imapSrv.Close()
	})

	smtpSrv := newFakeSMTPServer(t, tlsConfig)

	cfg := &Config{
		IMAPHost:    "127.0.0.1",
		IMAPPort:    imapLn.Addr().(*net.TCPAddr).Port,
		SMTPHost:    "127.0.0.1",
		SMTPPort:    smtpSrv.port(),
		Username:    testUsername,
		Password:    testPassword,
		TLSMode:     "starttls",
		FromAddress: testUsername,
	}
	return &testEnv{
		svc:     NewMailService(cfg, slog.New(slog.NewTextHandler(io.Discard, nil))),
		cfg:     cfg,
		user:    user,
		imapSrv: imapSrv,
		smtpSrv: smtpSrv,
	}
}

// createFolder creates a folder directly in the backend.
func (e *testEnv) createFolder(t *testing.T, name string) {
	t.Helper()
	if err := e.user.Create(name, nil); err != nil {
		t.Fatalf("creating backend folder %q: %v", name, err)
	}
}

// seed appends a simple text message to a folder and returns its UID.
func (e *testEnv) seed(t *testing.T, folder, subject string, flags ...imap.Flag) uint32 {
	t.Helper()
	raw := buildMessage("sender@example.com", testUsername, subject, "Body of "+subject)
	data, err := e.user.Append(folder, bytes.NewReader(raw), &imap.AppendOptions{Flags: flags})
	if err != nil {
		t.Fatalf("seeding %q into %q: %v", subject, folder, err)
	}
	return uint32(data.UID)
}

// requireCount asserts a folder's message count via the backend.
func (e *testEnv) requireCount(t *testing.T, folder string, want int) {
	t.Helper()
	status, err := e.user.Status(folder, &imap.StatusOptions{NumMessages: true})
	if err != nil {
		t.Fatalf("status of %q: %v", folder, err)
	}
	if got := int(*status.NumMessages); got != want {
		t.Fatalf("folder %q has %d messages, want %d", folder, got, want)
	}
}

// listSummaries lists all message summaries in a folder via the service.
func (e *testEnv) listSummaries(t *testing.T, folder string) []MessageSummary {
	t.Helper()
	msgs, err := e.svc.ListMessages(folder, 200, false)
	if err != nil {
		t.Fatalf("ListMessages(%q): %v", folder, err)
	}
	return msgs
}

// listFolders lists folders via the service.
func (e *testEnv) listFolders(t *testing.T) []FolderInfo {
	t.Helper()
	folders, err := e.svc.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	return folders
}

func findSummary(t *testing.T, msgs []MessageSummary, subject string) MessageSummary {
	t.Helper()
	for _, m := range msgs {
		if m.Subject == subject {
			return m
		}
	}
	t.Fatalf("no message with subject %q in listing of %d messages", subject, len(msgs))
	return MessageSummary{}
}

func folderByName(t *testing.T, folders []FolderInfo, name string) FolderInfo {
	t.Helper()
	for _, f := range folders {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("folder %q not found in listing of %d folders", name, len(folders))
	return FolderInfo{}
}

func folderExists(folders []FolderInfo, name string) bool {
	for _, f := range folders {
		if f.Name == name {
			return true
		}
	}
	return false
}

func hasFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if f == flag {
			return true
		}
	}
	return false
}

// buildMessage produces a minimal valid RFC822 text message.
func buildMessage(from, to, subject, body string) []byte {
	return []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%d@test.local>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n",
		from, to, subject, time.Now().Format(time.RFC1123Z), time.Now().UnixNano(), body,
	))
}

// generateTLSCert creates a throwaway self-signed certificate. flimap's
// client uses InsecureSkipVerify, so any valid cert will do.
func generateTLSCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "flimap-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

// fakeSMTPServer is a minimal SMTP server supporting EHLO, STARTTLS,
// AUTH PLAIN, MAIL, RCPT, DATA and QUIT — just enough for flimap's
// sendSMTP path. It records every message it accepts.
type fakeSMTPServer struct {
	ln        net.Listener
	tlsConfig *tls.Config

	mu         sync.Mutex
	rejectAuth bool
	sent       []smtpSend
}

// smtpSend is a message accepted by the fake SMTP server.
type smtpSend struct {
	From       string
	Recipients []string
	AuthUser   string
	AuthPass   string
	Data       []byte
}

func newFakeSMTPServer(t *testing.T, tlsConfig *tls.Config) *fakeSMTPServer {
	t.Helper()
	s := &fakeSMTPServer{tlsConfig: tlsConfig}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening for SMTP server: %v", err)
	}
	s.ln = ln
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.handleConn(conn)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *fakeSMTPServer) port() int {
	return s.ln.Addr().(*net.TCPAddr).Port
}

func (s *fakeSMTPServer) setRejectAuth(reject bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejectAuth = reject
}

func (s *fakeSMTPServer) sentMessages(t *testing.T) []smtpSend {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]smtpSend(nil), s.sent...)
}

func (s *fakeSMTPServer) record(msg smtpSend) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, msg)
}

func (s *fakeSMTPServer) handleConn(conn net.Conn) {
	defer conn.Close()
	text := textproto.NewConn(conn)
	text.PrintfLine("220 flimap-test ESMTP ready")

	var pending *smtpSend
	var authUser, authPass string
	for {
		line, err := text.ReadLine()
		if err != nil {
			return
		}
		cmd, rest := splitSMTPCmd(line)
		switch cmd {
		case "EHLO", "HELO":
			text.PrintfLine("250-flimap-test")
			text.PrintfLine("250-STARTTLS")
			text.PrintfLine("250-AUTH PLAIN")
			text.PrintfLine("250 8BITMIME")
		case "STARTTLS":
			text.PrintfLine("220 2.0.0 Ready to start TLS")
			tlsConn := tls.Server(conn, s.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			text = textproto.NewConn(tlsConn)
		case "AUTH":
			user, pass, err := s.handleAuth(text, rest)
			if err != nil {
				return
			}
			if user != "" {
				authUser, authPass = user, pass
			}
		case "MAIL":
			pending = &smtpSend{From: extractAddr(rest), AuthUser: authUser, AuthPass: authPass}
			text.PrintfLine("250 2.1.0 OK")
		case "RCPT":
			if pending != nil {
				pending.Recipients = append(pending.Recipients, extractAddr(rest))
			}
			text.PrintfLine("250 2.1.5 OK")
		case "DATA":
			if pending == nil {
				text.PrintfLine("503 5.5.1 Need MAIL first")
				continue
			}
			text.PrintfLine("354 End data with <CR><LF>.<CR><LF>")
			data, err := readSMTPData(text)
			if err != nil {
				return
			}
			pending.Data = data
			s.record(*pending)
			pending = nil
			text.PrintfLine("250 2.0.0 OK: queued")
		case "RSET":
			pending = nil
			text.PrintfLine("250 2.0.0 OK")
		case "NOOP":
			text.PrintfLine("250 2.0.0 OK")
		case "QUIT":
			text.PrintfLine("221 2.0.0 Bye")
			return
		default:
			text.PrintfLine("500 5.5.2 Unrecognized command")
		}
	}
}

// handleAuth answers an AUTH PLAIN command. It returns the authenticated
// identity on success (or empty strings when auth was rejected).
func (s *fakeSMTPServer) handleAuth(text *textproto.Conn, rest string) (user, pass string, err error) {
	s.mu.Lock()
	reject := s.rejectAuth
	s.mu.Unlock()

	parts := strings.SplitN(rest, " ", 2)
	if len(parts) < 2 || !strings.EqualFold(parts[0], "PLAIN") {
		text.PrintfLine("504 5.5.4 Unrecognized authentication type")
		return "", "", nil
	}
	if reject {
		text.PrintfLine("535 5.7.8 Authentication credentials invalid")
		return "", "", nil
	}
	b64 := parts[1]
	if b64 == "" {
		// Client didn't send an initial response; issue a challenge.
		text.PrintfLine("334 ")
		if b64, err = text.ReadLine(); err != nil {
			return "", "", err
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		text.PrintfLine("501 5.5.2 Invalid base64")
		return "", "", nil
	}
	fields := strings.SplitN(string(decoded), "\x00", 3)
	if len(fields) != 3 {
		text.PrintfLine("501 5.5.2 Malformed PLAIN response")
		return "", "", nil
	}
	text.PrintfLine("235 2.7.0 Accepted")
	return fields[1], fields[2], nil
}

// splitSMTPCmd splits a command line into its first word and the remainder.
func splitSMTPCmd(line string) (cmd, rest string) {
	if i := strings.IndexByte(line, ' '); i >= 0 {
		return strings.ToUpper(line[:i]), line[i+1:]
	}
	return strings.ToUpper(line), ""
}

// extractAddr pulls the address out of "FROM:<a@b> ARGS" / "TO:<a@b>".
func extractAddr(rest string) string {
	start := strings.IndexByte(rest, '<')
	end := strings.LastIndexByte(rest, '>')
	if start >= 0 && end > start {
		return rest[start+1 : end]
	}
	if i := strings.IndexByte(rest, ':'); i >= 0 {
		return strings.TrimSpace(rest[i+1:])
	}
	return strings.TrimSpace(rest)
}

// readSMTPData reads a DATA payload with dot-unstuffing until the "." terminator.
func readSMTPData(text *textproto.Conn) ([]byte, error) {
	var buf bytes.Buffer
	for {
		line, err := text.ReadLine()
		if err != nil {
			return nil, err
		}
		if line == "." {
			return buf.Bytes(), nil
		}
		if strings.HasPrefix(line, "..") {
			line = line[1:]
		}
		buf.WriteString(line)
		buf.WriteString("\r\n")
	}
}
