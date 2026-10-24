package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/config"
)

func TestChannelAndMessagingCLI(t *testing.T) {
	ws, cfgPath := modeWorkspace(t)
	t.Setenv(config.DefaultTelegramTokenEnv, "")

	var stdout, stderr bytes.Buffer

	// 1. Initial status shows disabled and not configured
	if code := run([]string{"channel", "--config", cfgPath, "status"}, &stdout, &stderr); code != 0 {
		t.Fatalf("channel status exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Telegram Bot Gateway : disabled") || !strings.Contains(stdout.String(), "not configured") {
		t.Fatalf("unexpected initial status:\n%s", stdout.String())
	}

	// 2. Enabling without token/allow fails
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"messaging", "--config", cfgPath, "enable"}, &stdout, &stderr); code == 0 {
		t.Fatal("expected enable without token/allow to fail")
	}

	// 3. Configure via `apw messaging set --token ... --allow ... --enable`
	stdout.Reset()
	stderr.Reset()
	secretTok := "123456789:ABCdefGHIjklMNOpqrSTUvwxYZ"
	if code := run([]string{"messaging", "--config", cfgPath, "set", "--token", secretTok, "--allow", "111222333, 444555666", "--enable"}, &stdout, &stderr); code != 0 {
		t.Fatalf("messaging set exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "enabled=true") || !strings.Contains(stdout.String(), "111222333, 444555666") {
		t.Fatalf("unexpected set output:\n%s", stdout.String())
	}

	// Verify secret token is NEVER written to apw.json
	rawCfgBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawCfgBytes), secretTok) {
		t.Fatal("secret bot token was leaked into apw.json")
	}

	// Verify secret token is stored in .apw/auth.json and resolved properly
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Channels.Telegram.Enabled {
		t.Fatal("expected Telegram channel to be enabled")
	}
	if got := cfg.Channels.Telegram.ResolveToken(ws); got != secretTok {
		t.Fatalf("ResolveToken = %q, want %q", got, secretTok)
	}

	// 4. Status masks the token
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"channel", "--config", cfgPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("channel default status exit=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), secretTok) {
		t.Fatal("status output leaked full secret token")
	}
	if !strings.Contains(stdout.String(), config.MaskSecret(secretTok)) {
		t.Fatalf("status output missing masked token:\n%s", stdout.String())
	}

	// 5. Test interactive shell `/channel` and `/messaging` slash commands
	stdout.Reset()
	stderr.Reset()
	shellIn := "/channel disable\n/messaging status\n/channel enable\nexit\n"
	if code := runShellConfig(cfgPath, strings.NewReader(shellIn), &stdout, &stderr); code != 0 {
		t.Fatalf("shell exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Disabled Telegram channel") || !strings.Contains(stdout.String(), "Enabled Telegram channel") {
		t.Fatalf("unexpected shell channel output:\n%s", stdout.String())
	}

	// 6. Clear token disables channel and removes key from .apw/auth.json
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"channel", "--config", cfgPath, "clear-token"}, &stdout, &stderr); code != 0 {
		t.Fatalf("clear-token exit=%d stderr=%s", code, stderr.String())
	}
	cfgAfter, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfgAfter.Channels.Telegram.Enabled {
		t.Fatal("expected channel to be disabled after clear-token")
	}
	if got := cfgAfter.Channels.Telegram.ResolveToken(ws); got != "" {
		t.Fatalf("expected empty token after clear-token, got %q", got)
	}
	authBytes, err := os.ReadFile(filepath.Join(ws, ".apw", "auth.json"))
	if err == nil && strings.Contains(string(authBytes), secretTok) {
		t.Fatal("auth.json still contains cleared secret token")
	}
}
