package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/codex-bridge/codex-bridge/internal/models"
)

const settingsSchema = "https://json.schemastore.org/claude-code-settings.json"

type ConfigureOptions struct {
	Path      string
	BaseURL   string
	AuthToken string
	Models    models.Router
	Now       func() time.Time
}

type ConfigureResult struct {
	Path       string
	BackupPath string
	Changed    bool
}

func DefaultSettingsPath() (string, error) {
	if directory := os.Getenv("CLAUDE_CONFIG_DIR"); directory != "" {
		return filepath.Join(directory, "settings.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func Configure(options ConfigureOptions) (ConfigureResult, error) {
	if options.Path == "" {
		return ConfigureResult{}, fmt.Errorf("Claude settings path is required")
	}
	if options.BaseURL == "" || options.AuthToken == "" {
		return ConfigureResult{}, fmt.Errorf("bridge base URL and authentication token are required")
	}
	if options.Now == nil {
		options.Now = time.Now
	}

	original, err := os.ReadFile(options.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ConfigureResult{}, fmt.Errorf("read Claude settings: %w", err)
	}
	root := make(map[string]any)
	if len(bytes.TrimSpace(original)) > 0 {
		if err := json.Unmarshal(original, &root); err != nil {
			return ConfigureResult{}, fmt.Errorf("parse Claude settings: %w", err)
		}
	}
	if _, exists := root["$schema"]; !exists {
		root["$schema"] = settingsSchema
	}
	if _, exists := root["model"]; !exists {
		root["model"] = "sonnet"
	}

	environment := make(map[string]any)
	if existing, exists := root["env"]; exists {
		object, ok := existing.(map[string]any)
		if !ok {
			return ConfigureResult{}, fmt.Errorf("Claude settings env value must be a JSON object")
		}
		environment = object
	}
	values := map[string]string{
		"ANTHROPIC_BASE_URL":                             options.BaseURL,
		"ANTHROPIC_AUTH_TOKEN":                           options.AuthToken,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":                  models.HaikuAlias,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL_NAME":             "Codex Haiku",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL_DESCRIPTION":      "Fast Codex tier backed by " + options.Models.Haiku,
		"ANTHROPIC_DEFAULT_SONNET_MODEL":                 models.SonnetAlias,
		"ANTHROPIC_DEFAULT_SONNET_MODEL_NAME":            "Codex Sonnet",
		"ANTHROPIC_DEFAULT_SONNET_MODEL_DESCRIPTION":     "Balanced Codex tier backed by " + options.Models.Sonnet,
		"ANTHROPIC_DEFAULT_OPUS_MODEL":                   models.OpusAlias,
		"ANTHROPIC_DEFAULT_OPUS_MODEL_NAME":              "Codex Opus",
		"ANTHROPIC_DEFAULT_OPUS_MODEL_DESCRIPTION":       "Powerful Codex tier backed by " + options.Models.Opus,
		"CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY":     "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC":       "1",
		"CLAUDE_CODE_ENABLE_FINE_GRAINED_TOOL_STREAMING": "1",
	}
	for key, value := range values {
		environment[key] = value
	}
	// A real Anthropic API key takes precedence over gateway authentication in
	// Claude Code. Remove a file-scoped value so the bridge Bearer token wins.
	delete(environment, "ANTHROPIC_API_KEY")
	root["env"] = environment

	formatted, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return ConfigureResult{}, fmt.Errorf("encode Claude settings: %w", err)
	}
	formatted = append(formatted, '\n')
	result := ConfigureResult{Path: options.Path}
	if bytes.Equal(original, formatted) {
		return result, nil
	}
	if err := os.MkdirAll(filepath.Dir(options.Path), 0o700); err != nil {
		return ConfigureResult{}, fmt.Errorf("create Claude settings directory: %w", err)
	}
	if len(original) > 0 {
		result.BackupPath = options.Path + ".codex-bridge." + options.Now().UTC().Format("20060102T150405.000000000Z") + ".bak"
		if err := os.WriteFile(result.BackupPath, original, 0o600); err != nil {
			return ConfigureResult{}, fmt.Errorf("back up Claude settings: %w", err)
		}
	}
	if err := writeAtomic(options.Path, formatted); err != nil {
		return ConfigureResult{}, err
	}
	result.Changed = true
	return result, nil
}

func writeAtomic(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".settings-*.json")
	if err != nil {
		return fmt.Errorf("create temporary Claude settings: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect temporary Claude settings: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary Claude settings: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary Claude settings: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary Claude settings: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("replace Claude settings: %w", err)
		}
		if err := os.Rename(temporaryPath, path); err != nil {
			return fmt.Errorf("replace Claude settings: %w", err)
		}
	}
	return nil
}
