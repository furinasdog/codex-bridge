package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/codex-bridge/codex-bridge/internal/app"
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
		flags.StringVar(&cfg.UpstreamModel, "model", cfg.UpstreamModel, "OpenAI model used for Anthropic requests")
		if err := flags.Parse(args); err != nil {
			return err
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
  codex-bridge serve [--address 127.0.0.1:8787] [--model MODEL]
  codex-bridge status
  codex-bridge logout
  codex-bridge version`)
}
