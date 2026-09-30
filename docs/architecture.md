# Architecture

## Design goals

Codex Bridge is designed around six boundaries:

1. The Gin server owns HTTP concerns, authentication, CORS, limits, request IDs, and shutdown.
2. The model catalog describes independently configured Haiku, Sonnet, and Opus choices using their real upstream IDs.
3. The Anthropic adapter validates Messages API input and converts it to provider-neutral OpenAI Responses structures.
4. The OpenAI client owns bearer authentication, Responses API requests, model discovery, and strict SSE completion handling.
5. The authentication package owns OAuth, OIDC verification, credential storage, and refresh serialization.
6. The Claude configurator safely merges real model IDs and gateway settings into the user's existing Claude Code configuration.
7. The telemetry store retains a rolling 24-hour local usage history and publishes it to the embedded Vue dashboard.

This is similar to the provider separation in `pi-ai`: protocol code does not know how credentials are acquired, and credential code does not know how Messages requests are represented.

## Request lifecycle

```text
POST /v1/messages
  -> request size and optional API-key checks
  -> Anthropic JSON validation
  -> unchanged request model forwarding
  -> system/messages/tools conversion
  -> access token lookup or serialized refresh
  -> POST https://api.openai.com/v1/responses
       store=false
       stream=true
  -> strict SSE event consumption
  -> local token and quota telemetry update
  -> Anthropic SSE output or aggregated JSON output
```

A turn is successful only after `response.completed`. A failed, incomplete, timed-out, or truncated stream is never presented as a successful response.

## Dashboard and telemetry

The Vue single-page application is compiled into static assets and embedded in
the Go binary. `GET /api/dashboard` accepts only loopback clients. Successful and
failed upstream turns are aggregated into fixed buckets for the last 1, 5, 12,
and 24 hours. The store persists only the rolling 24-hour window in the user's
configuration directory.

Codex plan, credit, and rate-window values are captured from response headers
when OpenAI provides them. Missing values remain unknown; the bridge does not
estimate subscription balance. OAuth tokens and request content never enter the
telemetry store or dashboard response.

## Authentication lifecycle

The `login` command implements OpenAI's public-client flow:

1. Create or reuse a stable installation host ID.
2. Start an HTTP callback listener on a random `127.0.0.1` port.
3. Generate fresh state, nonce, and PKCE values.
4. Open the OpenAI authorization endpoint with ChatGPT plan usage scopes.
5. Validate state and retain the issued dynamic client ID.
6. Exchange the code without a client secret.
7. Discover OpenAI's OIDC metadata and JWKS.
8. Verify the ID token's RS256 signature, issuer, audience, expiry, nonce, and subject.
9. Confirm the `chatgpt.tokens.use.direct` grant and atomically save the connection.

The access-token manager refreshes near expiry under a process mutex. It saves the access token and any rotating refresh token as one record, avoiding concurrent refresh races inside one bridge process.

## Streaming translation

The stream adapter is a state machine. It assigns stable Anthropic content-block indices while OpenAI emits text deltas or function-call argument deltas. It then emits:

```text
message_start
content_block_start
content_block_delta ...
content_block_stop
message_delta
message_stop
```

The same state machine aggregates blocks for non-streaming Anthropic callers. OpenAI still receives a streaming request because the ChatGPT plan usage flow requires it.

## Trust model

This service is a single-user local adapter. The host account, process environment, configuration directory, and local clients are inside the trust boundary. Internet clients, browser origins, incoming request bodies, OAuth callbacks, upstream JSON, and upstream SSE are untrusted.

The service is deliberately not a shared subscription gateway. Deployments outside a local workstation require an independent threat model, TLS termination, rate limiting, secret management, and authorization suitable for every user.
