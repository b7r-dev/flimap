package main

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/emersion/go-imap/v2/imapclient"
)

// Config holds all connection configuration.
type Config struct {
	IMAPHost       string
	IMAPPort       int
	SMTPHost       string
	SMTPPort       int
	Username       string
	Password       string
	TLSMode        string // "starttls", "ssl", or "none"
	FromAddress    string
	FolderAddresses []string // virtual folders filtered by To: header
}

// allMailFolder is the IMAP folder that contains all messages across addresses.
const allMailFolder = "All Mail"

// LoadConfig reads configuration from FLIMAP_ environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		IMAPHost: getEnv("FLIMAP_IMAP_HOST", "127.0.0.1"),
		IMAPPort: getEnvInt("FLIMAP_IMAP_PORT", 1143),
		SMTPHost: getEnv("FLIMAP_SMTP_HOST", "127.0.0.1"),
		SMTPPort: getEnvInt("FLIMAP_SMTP_PORT", 1025),
		Username: os.Getenv("FLIMAP_USERNAME"),
		Password: os.Getenv("FLIMAP_PASSWORD"),
		TLSMode:  getEnv("FLIMAP_TLS_MODE", "starttls"),
	}

	if cfg.Username == "" {
		return nil, fmt.Errorf("FLIMAP_USERNAME is required")
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("FLIMAP_PASSWORD is required")
	}

	cfg.FromAddress = getEnv("FLIMAP_FROM_ADDRESS", cfg.Username)

	// FolderAddresses can be set programmatically (e.g. via code), not via env var.
	// Virtual folder functionality is retained for programmatic use.

	return cfg, nil
}

// MailService provides IMAP and SMTP operations against Protonmail Bridge.
type MailService struct {
	cfg *Config
	log *slog.Logger
}

// NewMailService creates a new MailService.
func NewMailService(cfg *Config, log *slog.Logger) *MailService {
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return &MailService{cfg: cfg, log: log}
}

// dialIMAP creates a new IMAP connection, authenticates, and returns the client.
// The caller must call Logout (or Close) when done.
func (s *MailService) dialIMAP() (*imapclient.Client, error) {
	addr := fmt.Sprintf("%s:%d", s.cfg.IMAPHost, s.cfg.IMAPPort)
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	opts := &imapclient.Options{
		TLSConfig:  tlsConfig,
		DebugWriter: nil,
	}

	var c *imapclient.Client
	var err error
	switch s.cfg.TLSMode {
	case "starttls":
		c, err = imapclient.DialStartTLS(addr, opts)
	case "ssl":
		c, err = imapclient.DialTLS(addr, opts)
	case "none":
		c, err = imapclient.DialInsecure(addr, opts)
	default:
		return nil, fmt.Errorf("unknown TLS mode %q (use starttls, ssl, or none)", s.cfg.TLSMode)
	}
	if err != nil {
		return nil, fmt.Errorf("connecting to IMAP server: %w", err)
	}

	if err := c.Login(s.cfg.Username, s.cfg.Password).Wait(); err != nil {
		c.Close()
		return nil, fmt.Errorf("IMAP login: %w", err)
	}

	return c, nil
}

// withIMAP creates a connection, runs fn, and ensures logout.
func (s *MailService) withIMAP(fn func(*imapclient.Client) error) error {
	c, err := s.dialIMAP()
	if err != nil {
		return err
	}
	defer func() {
		c.Logout().Wait()
	}()
	return fn(c)
}

// resolveVirtualFolder checks if a folder name is a virtual address folder.
// Returns the email address and true if it is, "" and false otherwise.
func (s *MailService) resolveVirtualFolder(folder string) (string, bool) {
	for _, addr := range s.cfg.FolderAddresses {
		if strings.EqualFold(folder, addr) {
			return addr, true
		}
	}
	return "", false
}

// hasVirtualFolders returns true if any virtual folder addresses are configured.
func (s *MailService) hasVirtualFolders() bool {
	return len(s.cfg.FolderAddresses) > 0
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
