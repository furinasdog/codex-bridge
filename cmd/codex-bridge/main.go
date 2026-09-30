package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/codex-bridge/codex-bridge/internal/app"
	"github.com/codex-bridge/codex-bridge/internal/claude"
	"github.com/codex-bridge/codex-bridge/internal/config"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	command := "serve"
	args := os.Args[1:]
	if len(args) > 0 && args[0][0] != '-' {
		command, args = args[0], args[1:]
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	switch command {
	case "serve":
		flags := flag.NewFlagSet("serve", flag.ContinueOnError)
		flags.StringVar(&cfg.Address, "address", cfg.Address, "HTTP listen address")
		flags.StringVar(&cfg.HaikuModel, "haiku-model", cfg.HaikuModel, "OpenAI model used for the Haiku tier")
		flags.StringVar(&cfg.SonnetModel, "sonnet-model", cfg.SonnetModel, "OpenAI model used for the Sonnet tier")
		flags.StringVar(&cfg.OpusModel, "opus-model", cfg.OpusModel, "OpenAI model used for the Opus tier")
		legacyModel := flags.String("model", "", "deprecated: use one OpenAI model for every tier")
		if err := flags.Parse(args); err != nil {
			return err
		}
		if *legacyModel != "" {
			cfg.HaikuModel = *legacyModel
			cfg.SonnetModel = *legacyModel
			cfg.OpusModel = *legacyModel
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return app.Serve(ctx, cfg, version)
	case "login":
		flags := flag.NewFlagSet("login", flag.ContinueOnError)
		noBrowser := flags.Bool("no-browser", false, "print the authorization URL without opening a browser")
		if err := flags.Parse(args); err != nil {
			return err
		}
		return app.Login(context.Background(), cfg, !*noBrowser)
	case "configure-claude":
		defaultSettings, err := claudeSettingsPath()
		if err != nil {
			return err
		}
		flags := flag.NewFlagSet("configure-claude", flag.ContinueOnError)
		settingsPath := flags.String("settings", defaultSettings, "Claude Code user settings path")
		baseURL := flags.String("base-url", bridgeBaseURL(cfg.Address), "Codex Bridge base URL")
		authToken := flags.String("auth-token", localAuthToken(cfg.APIKey), "token Claude Code sends to Codex Bridge")
		if err := flags.Parse(args); err != nil {
			return err
		}
		return app.ConfigureClaude(cfg, *settingsPath, *baseURL, *authToken)
	case "logout":
		return app.Logout(context.Background(), cfg)
	case "status":
		return app.Status(cfg)
	case "version", "--version", "-version":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", command)
	}
}

func printUsage() {
	_, _ = fmt.Fprintln(os.Stderr, `codex-bridge converts the Anthropic Messages API to OpenAI Responses.

Usage:
  codex-bridge login [--no-browser]
  codex-bridge configure-claude [--settings PATH] [--base-url URL] [--auth-token TOKEN]
  codex-bridge serve [--address 127.0.0.1:8787] [--haiku-model MODEL] [--sonnet-model MODEL] [--opus-model MODEL]
  codex-bridge status
  codex-bridge logout
  codex-bridge version`)
}

func claudeSettingsPath() (string, error) {
	return claude.DefaultSettingsPath()
}

func bridgeBaseURL(address string) string {
	if strings.HasPrefix(address, ":") {
		address = "127.0.0.1" + address
	}
	return "http://" + address
}

func localAuthToken(configured string) string {
	if configured != "" {
		return configured
	}
	return "local-codex-bridge"
}
