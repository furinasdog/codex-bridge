# Codex Bridge

Codex Bridge is a small, local-first Go service that exposes the Anthropic Messages API and executes requests with a user's authorized ChatGPT plan through OpenAI's public Responses API. It is intended for open-source tools and personal local workflows, including using Codex models behind clients that speak the Anthropic protocol.

The design follows the provider and protocol-adapter separation used by [`@earendil-works/pi-ai`](https://github.com/earendil-works/pi/tree/main/packages/ai), but the implementation is native Go, uses Gin, and does not invoke Codex CLI.

> [!IMPORTANT]
> ChatGPT plan usage is an OpenAI preview capability with eligibility and usage limits. This project does not bypass a subscription, grant model access, or access ChatGPT conversations. Use it only in ways permitted by the applicable OpenAI terms and the [Sign in with ChatGPT documentation](https://developers.openai.com/siwc/token-sharing-open-source/overview).

## Features

- Anthropic-compatible `POST /v1/messages` with streaming and non-streaming responses
- Text, base64/URL image input, function tools, tool calls, and tool results
- `POST /v1/messages/count_tokens` for client-side context estimates
- Account-specific model discovery through `GET /v1/models`
- OAuth 2.0 authorization code flow with PKCE, state, nonce, and OIDC signature validation
- Automatic, concurrency-safe access-token refresh and rotating refresh-token persistence
- Public OpenAI Responses API only; every request uses `store: false` and `stream: true`
- Localhost-only default, optional bridge API key, explicit CORS allowlist, body limits, and graceful shutdown
- Structured logs with request IDs and no credential logging

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
- An eligible ChatGPT account that can grant ChatGPT plan usage
- A model available to that account

## Build

```bash
go build -trimpath -o bin/codex-bridge ./cmd/codex-bridge
```

On Windows:

```powershell
go build -trimpath -o bin/codex-bridge.exe ./cmd/codex-bridge
```

## Quick start

1. Sign in. This opens the system browser and listens temporarily on a random `127.0.0.1` callback port.

   ```bash
   ./bin/codex-bridge login
   ```

   In a headless environment, print the URL instead:

   ```bash
   ./bin/codex-bridge login --no-browser
   ```

2. Check the saved connection and discover the account's models.

   ```bash
   ./bin/codex-bridge status
   ./bin/codex-bridge serve
   curl http://127.0.0.1:8787/v1/models
   ```

3. Select an available model and start the bridge.

   ```bash
   CODEX_BRIDGE_MODEL=gpt-6.1-sol ./bin/codex-bridge serve
   ```

   PowerShell:

   ```powershell
   $env:CODEX_BRIDGE_MODEL = "gpt-6.1-sol"
   .\bin\codex-bridge.exe serve
   ```

4. Point Claude Code at the bridge. Claude Code requires a local API-key value even when bridge authentication is disabled.

   ```bash
   export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
   export ANTHROPIC_API_KEY=local-placeholder
   claude
   ```

   PowerShell:

   ```powershell
   $env:ANTHROPIC_BASE_URL = "http://127.0.0.1:8787"
   $env:ANTHROPIC_API_KEY = "local-placeholder"
   claude
   ```

When `CODEX_BRIDGE_API_KEY` is configured, use that same value as `ANTHROPIC_API_KEY` instead of the placeholder.

## Commands

```text
codex-bridge login [--no-browser]
codex-bridge serve [--address 127.0.0.1:8787] [--model MODEL]
codex-bridge status
codex-bridge logout
codex-bridge version
```

`logout` attempts to revoke the renewable OAuth session before removing the local credential file. If remote revocation cannot be confirmed, local credentials are still removed and the command reports the condition.

## Configuration

Configuration uses environment variables, with flags taking precedence for the listen address and model.

| Variable | Default | Purpose |
| --- | --- | --- |
| `CODEX_BRIDGE_ADDRESS` | `127.0.0.1:8787` | HTTP listen address |
| `CODEX_BRIDGE_MODEL` | `gpt-6.1-sol` | Upstream model used for all Anthropic model names |
| `CODEX_BRIDGE_API_KEY` | empty | Protects `/v1/*`; required for non-loopback binding |
| `CODEX_BRIDGE_CREDENTIALS` | OS user config directory | Credential JSON path |
| `CODEX_ACCESS_TOKEN` | empty | Non-persistent token override for automation |
| `CODEX_BRIDGE_ALLOWED_ORIGINS` | empty | Comma-separated browser origins allowed by CORS |
| `CODEX_BRIDGE_UPSTREAM_URL` | `https://api.openai.com/v1` | Responses API base URL, mainly for testing |
| `CODEX_BRIDGE_MAX_BODY_BYTES` | `33554432` | Maximum request body size |
| `CODEX_BRIDGE_REQUEST_TIMEOUT` | `10m` | Per-inference deadline |
| `CODEX_BRIDGE_LOG_LEVEL` | `info` | Structured log level |

The bridge maps every incoming Anthropic model name to `CODEX_BRIDGE_MODEL`. This lets clients keep their normal model aliases while the operator controls the actual Codex model in one place.

## API examples

Non-streaming request:

```bash
curl http://127.0.0.1:8787/v1/messages \
  -H 'content-type: application/json' \
  -H 'x-api-key: local-placeholder' \
  -d '{
    "model": "claude-compatible",
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
go test ./...
go vet ./...
go test -race ./...   # supported when the platform has CGO/race support
```

The test suite uses local `httptest` servers and does not require OpenAI credentials.

## Project status

The core text/tool/image translation path is implemented and tested. Anthropic and OpenAI evolve independently, so some less-common Anthropic features are intentionally unsupported or approximated. Review the [compatibility matrix](docs/protocol-compatibility.md) before relying on a feature.

## License

[MIT](LICENSE)
