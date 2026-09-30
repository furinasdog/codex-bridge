# Codex Bridge

Codex Bridge is a small, local-first Go service that exposes the Anthropic Messages API and executes requests with a user's authorized ChatGPT plan through OpenAI's public Responses API. It is intended for open-source tools and personal local workflows, including using Codex models behind clients that speak the Anthropic protocol.

The design follows the provider and protocol-adapter separation used by [`@earendil-works/pi-ai`](https://github.com/earendil-works/pi/tree/main/packages/ai), but the implementation is native Go, uses Gin, and does not invoke Codex CLI.

> [!IMPORTANT]
> ChatGPT plan usage is an OpenAI preview capability with eligibility and usage limits. This project does not bypass a subscription, grant model access, or access ChatGPT conversations. Use it only in ways permitted by the applicable OpenAI terms and the [Sign in with ChatGPT documentation](https://developers.openai.com/siwc/token-sharing-open-source/overview).

## Features

- Anthropic-compatible `POST /v1/messages` with streaming and non-streaming responses
- Text, base64/URL image input, function tools, tool calls, and tool results
- `POST /v1/messages/count_tokens` for client-side context estimates
- Configured Codex model discovery through `GET /v1/models`
- Native Haiku, Sonnet, and Opus choices that use real upstream model IDs end to end
- Safe, idempotent Claude Code `settings.json` configuration with automatic backups and `modelPicker` capability mappings
- Embedded Vue dashboard with English and Chinese UI, 1/5/12/24-hour usage charts, service health, and Codex quota metadata
- OAuth 2.0 authorization code flow with PKCE, state, nonce, and OIDC signature validation
- Automatic, concurrency-safe access-token refresh and rotating refresh-token persistence
- Public OpenAI Responses API only; every request uses `store: false` and `stream: true`
- Localhost-only default, optional bridge API key, explicit CORS allowlist, body limits, and graceful shutdown
- Colored Gin request logs, readable service logs, request IDs, and no credential logging

## Architecture

```text
Claude Code or another Anthropic client
                  |
          Anthropic Messages API
                  |
             Gin server
                  |
       protocol conversion layer
                  |
        OpenAI Responses API (SSE)
                  |
    ChatGPT plan OAuth access token
```

See [Architecture](docs/architecture.md) and [Protocol compatibility](docs/protocol-compatibility.md) for details.

## Requirements

- Go 1.23 or newer
- Node.js 22 or newer and npm (only when rebuilding the dashboard)
- An eligible ChatGPT account that can grant ChatGPT plan usage
- A model available to that account

## Build

```bash
npm ci --prefix web
npm run --prefix web build
go build -trimpath -o bin/codex-bridge ./cmd/codex-bridge
```

On Windows:

```powershell
npm ci --prefix web
npm run --prefix web build
go build -trimpath -o bin/codex-bridge.exe ./cmd/codex-bridge
```

GNU Make selects the native executable name automatically:

```bash
make
```

This produces `bin/codex-bridge.exe` on Windows and `bin/codex-bridge` on
Linux and macOS. The dashboard is rebuilt only when its source files change.

The compiled dashboard is embedded in the Go binary. A source archive or Git
checkout already includes the generated assets, so Go-only builds also work.

## Quick start

1. Sign in. This opens the system browser and listens temporarily on a random `127.0.0.1` callback port.

   ```bash
   ./bin/codex-bridge login
   ```

   In a headless environment, print the URL instead:

   ```bash
   ./bin/codex-bridge login --no-browser
   ```

2. Configure Claude Code. This merges the bridge settings into the user-level
   `~/.claude/settings.json` file and backs up an existing file before changing it.

   ```bash
   ./bin/codex-bridge configure-claude
   ```

   PowerShell:

   ```powershell
   .\bin\codex-bridge.exe configure-claude
   ```

   The command respects `CLAUDE_CONFIG_DIR`, preserves unrelated settings, and can
   be run repeatedly. If `ANTHROPIC_API_KEY` is already set in the current shell,
   remove it before launching Claude Code because it takes precedence over the
   configured gateway token:

   ```powershell
   Remove-Item Env:ANTHROPIC_API_KEY -ErrorAction SilentlyContinue
   ```

   Re-run this command after upgrading Codex Bridge so newly supported Claude
   Code configuration fields are merged into an existing setup.

3. Start the bridge in one terminal.

   ```bash
   ./bin/codex-bridge serve
   ```

   PowerShell:

   ```powershell
   .\bin\codex-bridge.exe serve
   ```

   Open `http://127.0.0.1:8787/` for the local dashboard. Its metrics endpoint
   is restricted to loopback clients and never exposes OAuth credentials.

4. Start or restart Claude Code, then use `/model` to choose **Codex Haiku**,
   **Codex Sonnet**, or **Codex Opus**.

   ```bash
   claude
   ```

To inspect the active account and the bridge's configured tiers:

```bash
./bin/codex-bridge status
curl http://127.0.0.1:8787/v1/models
```

## Commands

```text
codex-bridge login [--no-browser]
codex-bridge configure-claude [--settings PATH] [--base-url URL] [--auth-token TOKEN]
codex-bridge serve [--address 127.0.0.1:8787] [--haiku-model MODEL] [--sonnet-model MODEL] [--opus-model MODEL]
codex-bridge status
codex-bridge logout
codex-bridge version
```

`logout` attempts to revoke the renewable OAuth session before removing the local credential file. If remote revocation cannot be confirmed, local credentials are still removed and the command reports the condition.

## Configuration

Configuration uses environment variables, with flags taking precedence for the listen address and tier models.

| Variable | Default | Purpose |
| --- | --- | --- |
| `CODEX_BRIDGE_ADDRESS` | `127.0.0.1:8787` | HTTP listen address |
| `CODEX_BRIDGE_HAIKU_MODEL` | `gpt-6-luna` | Fast, efficient model behind the Haiku tier |
| `CODEX_BRIDGE_SONNET_MODEL` | `gpt-6.1-sol` | Balanced model behind the Sonnet tier |
| `CODEX_BRIDGE_OPUS_MODEL` | `gpt-6-astra` | Highest-capability model behind the Opus tier |
| `CODEX_BRIDGE_MODEL` | empty | Legacy fallback for the Sonnet tier; `--model` maps all three tiers for backward compatibility |
| `CODEX_BRIDGE_API_KEY` | empty | Protects `/v1/*`; required for non-loopback binding |
| `CODEX_BRIDGE_CREDENTIALS` | OS user config directory | Credential JSON path |
| `CODEX_BRIDGE_TELEMETRY` | OS user config directory | Rolling 24-hour dashboard metrics path |
| `CODEX_ACCESS_TOKEN` | empty | Non-persistent token override for automation |
| `CODEX_BRIDGE_ALLOWED_ORIGINS` | empty | Comma-separated browser origins allowed by CORS |
| `CODEX_BRIDGE_UPSTREAM_URL` | `https://api.openai.com/v1` | Responses API base URL, mainly for testing |
| `CODEX_BRIDGE_MAX_BODY_BYTES` | `33554432` | Maximum request body size |
| `CODEX_BRIDGE_REQUEST_TIMEOUT` | `10m` | Per-inference deadline |
| `CODEX_BRIDGE_LOG_LEVEL` | `info` | Service log level |

`configure-claude` writes the real configured IDs into Claude Code's default
Haiku, Sonnet, and Opus settings. The server forwards every inbound `model` value
unchanged to OpenAI and returns the same value in the Anthropic response. There
is no server-side alias or tier translation.

## API examples

Non-streaming request:

```bash
curl http://127.0.0.1:8787/v1/messages \
  -H 'content-type: application/json' \
  -H 'x-api-key: local-placeholder' \
  -d '{
    "model": "gpt-6.1-sol",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "Explain this repository."}]
  }'
```

For streaming, add `"stream": true`; the response is Anthropic-style server-sent events.

## Security notes

- The default listener is loopback-only. The process refuses non-loopback addresses unless `CODEX_BRIDGE_API_KEY` is set.
- OAuth credentials are stored atomically under the user's OS configuration directory. The directory and files request owner-only permissions; on Windows they also inherit the current user's profile ACL.
- Tokens are never returned by HTTP endpoints or written to logs.
- Browser access is disabled unless an origin is explicitly listed in `CODEX_BRIDGE_ALLOWED_ORIGINS`.
- Treat the host running this service as trusted. This bridge is not designed as a multi-tenant hosted gateway.

Read [SECURITY.md](SECURITY.md) before exposing the service beyond localhost.

## Development

```bash
npm ci --prefix web
npm run --prefix web typecheck
npm run --prefix web build
go test ./...
go vet ./...
go test -race ./...   # supported when the platform has CGO/race support
```

The test suite uses local `httptest` servers and does not require OpenAI credentials.

## Releases

Pushing a semantic version tag such as `v1.0.0` runs the release workflow. It
builds amd64 and arm64 archives for Linux, macOS, and Windows, publishes a
`SHA256SUMS` file, and creates a GitHub Release with automatically generated
release notes.

```bash
git tag v1.0.0
git push origin v1.0.0
```

## Project status

The core text/tool/image translation path is implemented and tested. Anthropic and OpenAI evolve independently, so some less-common Anthropic features are intentionally unsupported or approximated. Review the [compatibility matrix](docs/protocol-compatibility.md) before relying on a feature.

## License

[MIT](LICENSE)
