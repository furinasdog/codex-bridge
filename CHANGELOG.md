# Changelog

All notable changes to this project will be documented in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and releases use Semantic Versioning.

## [Unreleased]

### Added

- Anthropic Messages API compatibility for text, images, tools, streaming, and non-streaming calls.
- OpenAI Responses API provider using ChatGPT plan OAuth authorization.
- PKCE login, OIDC validation, credential refresh, logout, and model discovery.
- Local security controls, structured logging, tests, and project documentation.
- Safe automatic Claude Code settings configuration with backups and atomic writes.
- Configurable Haiku, Sonnet, and Opus tiers with stable virtual model discovery.

### Fixed

- Accept Claude Code's message-level system and developer scaffolding roles.
- Describe custom Codex tiers with `modelPicker.behavesAs` to avoid unknown-model warnings.
- Omit `max_output_tokens`, which the Codex subscription Responses endpoint rejects.
