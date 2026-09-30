# Protocol compatibility

Codex Bridge implements the subset of the Anthropic Messages API commonly used by Claude Code and similar agent clients.

| Anthropic feature | Status | Notes |
| --- | --- | --- |
| Text input and output | Supported | Strings and text content blocks |
| System prompt | Supported | String or text-block array, joined as Responses instructions |
| Message-level `system` and `developer` roles | Supported | Claude Code gateway scaffolding is merged into Responses instructions |
| Streaming | Supported | Anthropic SSE event sequence |
| Non-streaming | Supported | Aggregated from a mandatory upstream stream |
| Tools | Supported | JSON Schema is forwarded as a Responses function tool |
| Tool use/results | Supported | Maps to function calls and function-call outputs |
| Parallel tool calls | Supported | Controlled by `disable_parallel_tool_use` |
| Base64 images | Supported | Converted to data URLs |
| URL images | Supported | Forwarded as input-image URLs |
| Prompt caching controls | Ignored | OpenAI caching is automatic; request session IDs become a clamped cache key |
| `temperature` and `top_p` | Forwarded | Model support is decided upstream |
| `max_tokens` | Accepted, provider-managed | Required by Anthropic clients but not forwarded because the Codex subscription endpoint rejects `max_output_tokens` |
| `top_k` | Ignored | No direct Responses API equivalent |
| Stop sequences | Ignored | No portable equivalent for the supported Responses flow |
| Token counting | Estimated | UTF-8 JSON size divided by four; final response usage is authoritative |
| PDF/document blocks | Not supported | Returns an `invalid_request_error` |
| Citations | Not supported | No cross-provider lossless mapping |
| Extended thinking blocks | Not exposed | Reasoning remains provider-managed |

## Model mapping

The bridge exposes three stable virtual tiers and resolves native Claude aliases to them:

| Requested tier | Virtual model | Default upstream model |
| --- | --- | --- |
| Haiku | `codex-haiku` | `gpt-6-luna` |
| Sonnet | `codex-sonnet` | `gpt-6.1-sol` |
| Opus | `codex-opus` | `gpt-6-astra` |

The defaults can be changed independently with `CODEX_BRIDGE_HAIKU_MODEL`,
`CODEX_BRIDGE_SONNET_MODEL`, and `CODEX_BRIDGE_OPUS_MODEL`. Unknown model IDs
pass through unchanged to support explicitly configured provider models. The
inbound `model` value is preserved in Anthropic responses for client consistency.
`GET /v1/models` always advertises the three configured virtual tiers; the Codex
inference endpoint remains authoritative for account access because its
model-list response can omit models that are callable.

## Claude Code setup

`codex-bridge configure-claude` merges the base URL, Bearer authentication token,
three tier aliases, display names, and a `modelPicker` lineup into Claude Code's
user-level `settings.json`. Each generated row carries a `behavesAs` mapping so
Claude Code applies a known capability profile without warning about an unknown
model. Existing picker rows and unrelated keys are preserved, an existing file
is backed up before modification, and an invalid JSON file is left untouched.
The command uses `CLAUDE_CONFIG_DIR` when set and otherwise uses
`~/.claude/settings.json`.

## Errors

Errors before streaming begins use the Anthropic JSON error envelope. After SSE headers have been sent, errors use an Anthropic `error` event. Upstream status codes are retained when meaningful and mapped to Anthropic error types:

- `401` and `403`: `authentication_error`
- `429`: `rate_limit_error`
- `400` and `404`: `invalid_request_error`
- other upstream and transport failures: `api_error`
- local deadline: `timeout_error`

Usage-limit failures may arrive after streaming has started. They are emitted as stream errors and are never converted into a successful `message_stop`.
