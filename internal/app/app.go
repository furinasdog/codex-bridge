package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/codex-bridge/codex-bridge/internal/auth"
	"github.com/codex-bridge/codex-bridge/internal/claude"
	"github.com/codex-bridge/codex-bridge/internal/config"
	"github.com/codex-bridge/codex-bridge/internal/models"
	"github.com/codex-bridge/codex-bridge/internal/server"
)

func Serve(ctx context.Context, cfg config.Config, version string) error {
	configureLogger(cfg.LogLevel)
	manager := auth.NewManager(cfg.CredentialPath, cfg.AccessToken, nil)
	return server.New(cfg, manager, version).Run(ctx)
}

func Login(ctx context.Context, cfg config.Config, openBrowser bool) error {
	credential, err := auth.Login(ctx, auth.LoginOptions{
		CredentialPath: cfg.CredentialPath,
		Timeout:        cfg.OAuthTimeout,
		OpenBrowser:    openBrowser,
		AppName:        "Codex Bridge",
	})
	if err != nil {
		return err
	}
	fmt.Printf("Signed in as %s. Credentials saved to %s.\n", credential.Email, cfg.CredentialPath)
	return nil
}

func Logout(ctx context.Context, cfg config.Config) error {
	if err := auth.Logout(ctx, cfg.CredentialPath, nil); err != nil {
		return err
	}
	fmt.Println("Signed out and removed local credentials.")
	return nil
}

func Status(cfg config.Config) error {
	if cfg.AccessToken != "" {
		fmt.Println("Authentication: CODEX_ACCESS_TOKEN environment variable")
		return nil
	}
	credential, err := auth.LoadCredential(cfg.CredentialPath)
	if err != nil {
		return err
	}
	fmt.Printf("Authentication: saved ChatGPT connection\nAccount: %s\nExpires: %s\n", credential.Email, credential.ExpiresAt().Format("2006-01-02 15:04:05Z07:00"))
	return nil
}

func ConfigureClaude(cfg config.Config, settingsPath, baseURL, authToken string) error {
	result, err := claude.Configure(claude.ConfigureOptions{
		Path: settingsPath, BaseURL: baseURL, AuthToken: authToken,
		Models: models.NewCatalog(cfg.HaikuModel, cfg.SonnetModel, cfg.OpusModel),
	})
	if err != nil {
		return err
	}
	if !result.Changed {
		fmt.Printf("Claude Code settings are already configured at %s.\n", result.Path)
		return nil
	}
	fmt.Printf("Claude Code settings updated at %s.\n", result.Path)
	if result.BackupPath != "" {
		fmt.Printf("Previous settings backed up to %s.\n", result.BackupPath)
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		fmt.Println("Warning: ANTHROPIC_API_KEY is set in the current shell. Unset it before starting Claude Code so ANTHROPIC_AUTH_TOKEN can select the bridge.")
	}
	fmt.Println("Restart Claude Code, then use /model to choose Codex Haiku, Codex Sonnet, or Codex Opus.")
	return nil
}

func configureLogger(level string) {
	var slogLevel slog.Level
	if err := slogLevel.UnmarshalText([]byte(level)); err != nil {
		slogLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel})))
}
