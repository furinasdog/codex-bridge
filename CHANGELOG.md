# Changelog

All notable changes to this project will be documented in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and releases use Semantic Versioning.

## [Unreleased]

### Added

- Anthropic Messages API compatibility for text, images, tools, streaming, and non-streaming calls.
- OpenAI Responses API provider using ChatGPT plan OAuth authorization.
- PKCE login, OIDC validation, credential refresh, logout, and model discovery.
- Local security controls, structured logging, tests, and project documentation.
- Safe automatic Claude Code settings configuration with backups and atomic writes.
- Configurable Haiku, Sonnet, and Opus choices with real model discovery.
- Embedded bilingual Vue dashboard with rolling token usage, service metrics, and Codex quota reporting.
- Persistent 24-hour local telemetry with no prompt or credential storage.
- Automated multi-platform GitHub Releases with generated notes and SHA-256 checksums.

### Fixed

- Accept Claude Code's message-level system and developer scaffolding roles.
- Describe custom Codex tiers with `modelPicker.behavesAs` to avoid unknown-model warnings.
- Omit `max_output_tokens`, which the Codex subscription Responses endpoint rejects.

### Changed

- Write real configured model IDs into Claude Code settings and forward request model names without server-side translation.
- Replace JSON request logging with Gin's colored console logger and readable text service logs.
- Produce the native `.exe` filename from `make` on Windows and avoid unnecessary dashboard rebuilds.
- Read Codex subscription windows and credit state from `codex.rate_limits` stream events when response headers omit them.
- Give input and output token series independent chart scales so small output totals remain visible.
