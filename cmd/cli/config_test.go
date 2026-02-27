package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigSetURL(t *testing.T) {
	// Use a temp dir for config
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	var buf bytes.Buffer
	err := runConfigSetURL([]string{"http://localhost:8080"}, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "http://localhost:8080") {
		t.Errorf("expected URL in output, got: %s", buf.String())
	}

	// Verify config was saved
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.APIURL != "http://localhost:8080" {
		t.Errorf("expected URL in config, got: %s", cfg.APIURL)
	}
}

func TestConfigSetToken(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	var buf bytes.Buffer
	err := runConfigSetToken([]string{"my-secret-token"}, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Token saved") {
		t.Errorf("expected confirmation in output, got: %s", buf.String())
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Token != "my-secret-token" {
		t.Errorf("expected token in config, got: %s", cfg.Token)
	}
}

func TestConfigShow_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	var buf bytes.Buffer
	err := runConfigShow(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "(not set)") {
		t.Errorf("expected '(not set)' in output, got: %s", out)
	}
}

func TestConfigShow_WithValues(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Set values first
	runConfigSetURL([]string{"http://example.com"}, &bytes.Buffer{})
	runConfigSetToken([]string{"super-secret-token-123"}, &bytes.Buffer{})

	var buf bytes.Buffer
	err := runConfigShow(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "http://example.com") {
		t.Errorf("expected URL in output, got: %s", out)
	}
	// Token should be masked
	if !strings.Contains(out, "super-se...") {
		t.Errorf("expected masked token in output, got: %s", out)
	}
	if strings.Contains(out, "super-secret-token-123") {
		t.Error("token should be masked, not shown in full")
	}
}

func TestConfigSetURL_NoArgs(t *testing.T) {
	var buf bytes.Buffer
	err := runConfigSetURL(nil, &buf)
	if err == nil {
		t.Error("expected error for missing URL arg")
	}
}

func TestConfigSetToken_NoArgs(t *testing.T) {
	var buf bytes.Buffer
	err := runConfigSetToken(nil, &buf)
	if err == nil {
		t.Error("expected error for missing token arg")
	}
}

func TestConfigFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	runConfigSetToken([]string{"test-token"}, &bytes.Buffer{})

	path := filepath.Join(tmpDir, ".loghunter", "config.yaml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("config file not found: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected file permissions 0600, got %o", perm)
	}
}

func TestLoadConfig_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIURL != "" || cfg.Token != "" {
		t.Error("expected empty config when no file exists")
	}
}

func TestResolveCredentials_EnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	origURL := os.Getenv("LOGHUNTER_API_URL")
	origToken := os.Getenv("LOGHUNTER_TOKEN")
	os.Setenv("LOGHUNTER_API_URL", "http://env-url.com")
	os.Setenv("LOGHUNTER_TOKEN", "env-token")
	defer func() {
		os.Setenv("LOGHUNTER_API_URL", origURL)
		os.Setenv("LOGHUNTER_TOKEN", origToken)
	}()

	fs := newFlagSetWithGlobals("test")
	fs.Parse([]string{})

	url, token, err := resolveCredentials(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://env-url.com" {
		t.Errorf("expected env URL, got: %s", url)
	}
	if token != "env-token" {
		t.Errorf("expected env token, got: %s", token)
	}
}

func TestResolveCredentials_FlagOverridesEnv(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	origURL := os.Getenv("LOGHUNTER_API_URL")
	origToken := os.Getenv("LOGHUNTER_TOKEN")
	os.Setenv("LOGHUNTER_API_URL", "http://env-url.com")
	os.Setenv("LOGHUNTER_TOKEN", "env-token")
	defer func() {
		os.Setenv("LOGHUNTER_API_URL", origURL)
		os.Setenv("LOGHUNTER_TOKEN", origToken)
	}()

	fs := newFlagSetWithGlobals("test")
	fs.Parse([]string{"--url", "http://flag-url.com", "--token", "flag-token"})

	url, token, err := resolveCredentials(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://flag-url.com" {
		t.Errorf("expected flag URL, got: %s", url)
	}
	if token != "flag-token" {
		t.Errorf("expected flag token, got: %s", token)
	}
}

func TestResolveCredentials_ConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Clear env vars
	origURL := os.Getenv("LOGHUNTER_API_URL")
	origToken := os.Getenv("LOGHUNTER_TOKEN")
	os.Unsetenv("LOGHUNTER_API_URL")
	os.Unsetenv("LOGHUNTER_TOKEN")
	defer func() {
		os.Setenv("LOGHUNTER_API_URL", origURL)
		os.Setenv("LOGHUNTER_TOKEN", origToken)
	}()

	// Save config
	runConfigSetURL([]string{"http://config-url.com"}, &bytes.Buffer{})
	runConfigSetToken([]string{"config-token"}, &bytes.Buffer{})

	fs := newFlagSetWithGlobals("test")
	fs.Parse([]string{})

	url, token, err := resolveCredentials(fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://config-url.com" {
		t.Errorf("expected config URL, got: %s", url)
	}
	if token != "config-token" {
		t.Errorf("expected config token, got: %s", token)
	}
}

func TestResolveCredentials_MissingURL(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	origURL := os.Getenv("LOGHUNTER_API_URL")
	origToken := os.Getenv("LOGHUNTER_TOKEN")
	os.Unsetenv("LOGHUNTER_API_URL")
	os.Unsetenv("LOGHUNTER_TOKEN")
	defer func() {
		os.Setenv("LOGHUNTER_API_URL", origURL)
		os.Setenv("LOGHUNTER_TOKEN", origToken)
	}()

	fs := newFlagSetWithGlobals("test")
	fs.Parse([]string{})

	_, _, err := resolveCredentials(fs)
	if err == nil {
		t.Error("expected error for missing URL")
	}
}

// newFlagSetWithGlobals creates a flag set with --url and --token registered.
func newFlagSetWithGlobals(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.String("url", "", "API server URL")
	fs.String("token", "", "API token")
	return fs
}
