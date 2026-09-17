package main

import "testing"

// Neutralize ambient FLIMAP_* variables so the tests are hermetic regardless
// of the developer's shell environment.
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"FLIMAP_USERNAME", "FLIMAP_PASSWORD",
		"FLIMAP_IMAP_HOST", "FLIMAP_IMAP_PORT",
		"FLIMAP_SMTP_HOST", "FLIMAP_SMTP_PORT",
		"FLIMAP_TLS_MODE", "FLIMAP_FROM_ADDRESS",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadConfigRequiresCredentials(t *testing.T) {
	clearConfigEnv(t)
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected LoadConfig to fail without FLIMAP_USERNAME/FLIMAP_PASSWORD")
	}

	clearConfigEnv(t)
	t.Setenv("FLIMAP_USERNAME", "u@example.com")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected LoadConfig to fail without FLIMAP_PASSWORD")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("FLIMAP_USERNAME", "u@example.com")
	t.Setenv("FLIMAP_PASSWORD", "pw")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.IMAPHost != "127.0.0.1" || cfg.IMAPPort != 1143 {
		t.Fatalf("unexpected IMAP defaults: %s:%d", cfg.IMAPHost, cfg.IMAPPort)
	}
	if cfg.SMTPHost != "127.0.0.1" || cfg.SMTPPort != 1025 {
		t.Fatalf("unexpected SMTP defaults: %s:%d", cfg.SMTPHost, cfg.SMTPPort)
	}
	if cfg.TLSMode != "starttls" {
		t.Fatalf("TLSMode = %q, want starttls", cfg.TLSMode)
	}
	if cfg.FromAddress != "u@example.com" {
		t.Fatalf("FromAddress = %q, want the username", cfg.FromAddress)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("FLIMAP_USERNAME", "u@example.com")
	t.Setenv("FLIMAP_PASSWORD", "pw")
	t.Setenv("FLIMAP_IMAP_HOST", "imap.example.com")
	t.Setenv("FLIMAP_IMAP_PORT", "993")
	t.Setenv("FLIMAP_SMTP_HOST", "smtp.example.com")
	t.Setenv("FLIMAP_SMTP_PORT", "465")
	t.Setenv("FLIMAP_TLS_MODE", "ssl")
	t.Setenv("FLIMAP_FROM_ADDRESS", "alias@example.com")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.IMAPHost != "imap.example.com" || cfg.IMAPPort != 993 {
		t.Fatalf("unexpected IMAP override: %s:%d", cfg.IMAPHost, cfg.IMAPPort)
	}
	if cfg.SMTPHost != "smtp.example.com" || cfg.SMTPPort != 465 {
		t.Fatalf("unexpected SMTP override: %s:%d", cfg.SMTPHost, cfg.SMTPPort)
	}
	if cfg.TLSMode != "ssl" {
		t.Fatalf("TLSMode = %q, want ssl", cfg.TLSMode)
	}
	if cfg.FromAddress != "alias@example.com" {
		t.Fatalf("FromAddress = %q, want alias@example.com", cfg.FromAddress)
	}
}
