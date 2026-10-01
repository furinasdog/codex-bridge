package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/codex-bridge/codex-bridge/internal/models"
)

func TestConfigureMergesAndBacksUpSettings(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "settings.json")
	original := []byte("{\n  \"includeCoAuthoredBy\": false,\n  \"effortLevel\": \"medium\",\n  \"env\": {\"KEEP_ME\": \"yes\", \"ANTHROPIC_API_KEY\": \"remove-me\", \"CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY\": \"1\"},\n  \"modelPicker\": {\"options\": [{\"model\": \"foreign-model\", \"label\": \"Keep me\"}]}\n}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 30, 6, 7, 8, 9, time.UTC)
	options := ConfigureOptions{
		Path: path, BaseURL: "http://127.0.0.1:8787", AuthToken: "bridge-token",
		Models: models.NewCatalog("fast", "balanced", "powerful"),
		Now:    func() time.Time { return now },
	}

	result, err := Configure(options)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.BackupPath == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	backup, err := os.ReadFile(result.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backup, original) {
		t.Fatal("backup does not match the original settings")
	}

	configured, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(configured, &root); err != nil {
		t.Fatal(err)
	}
	if root["includeCoAuthoredBy"] != false || root["effortLevel"] != "medium" || root["model"] != "balanced" {
		t.Fatalf("existing settings were not preserved: %#v", root)
	}
	environment := root["env"].(map[string]any)
	if environment["KEEP_ME"] != "yes" || environment["ANTHROPIC_AUTH_TOKEN"] != "bridge-token" {
		t.Fatalf("environment was not merged: %#v", environment)
	}
	if _, exists := environment["ANTHROPIC_API_KEY"]; exists {
		t.Fatal("ANTHROPIC_API_KEY must be removed from file-scoped settings")
	}
	if _, exists := environment["CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"]; exists {
		t.Fatal("gateway discovery must be removed")
	}
	if environment["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "fast" ||
		environment["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "balanced" ||
		environment["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "powerful" {
		t.Fatalf("tier aliases were not configured: %#v", environment)
	}
	if environment["CLAUDE_CODE_MAX_CONTEXT_TOKENS"] != "272000" {
		t.Fatalf("context window was not configured: %#v", environment)
	}
	picker := root["modelPicker"].(map[string]any)
	rows := picker["options"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["model"] != "foreign-model" {
		t.Fatalf("existing modelPicker was changed: %#v", picker)
	}

	second, err := Configure(options)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed || second.BackupPath != "" {
		t.Fatalf("idempotent configuration unexpectedly changed settings: %#v", second)
	}
}

func TestConfigureDoesNotAddModelPicker(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.json")
	_, err := Configure(ConfigureOptions{
		Path: path, BaseURL: "http://127.0.0.1:8787", AuthToken: "token",
		Models: models.NewCatalog("fast", "balanced", "powerful"),
	})
	if err != nil {
		t.Fatal(err)
	}
	configured, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(configured, &root); err != nil {
		t.Fatal(err)
	}
	if _, exists := root["modelPicker"]; exists {
		t.Fatalf("configure-claude added modelPicker: %#v", root["modelPicker"])
	}
}

func TestConfigureRejectsInvalidJSONWithoutChangingIt(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.json")
	original := []byte("{not-json")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Configure(ConfigureOptions{
		Path: path, BaseURL: "http://127.0.0.1:8787", AuthToken: "token",
		Models: models.NewCatalog("fast", "balanced", "powerful"),
	})
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
	actual, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(actual, original) {
		t.Fatal("invalid settings file was modified")
	}
}
