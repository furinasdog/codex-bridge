# Security Policy

## Supported versions

Security fixes are applied to the latest release and the `main` branch.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Contact the repository maintainers privately through the security advisory feature of the hosting repository. Include reproduction steps, affected versions, impact, and any proposed mitigation.

Do not include real OAuth tokens, credential files, email addresses, or unredacted authorization URLs in a report.

## Deployment guidance

Codex Bridge is intended to run as a single-user local process.

- Keep the default `127.0.0.1` listener whenever possible.
- Set a long, random `CODEX_BRIDGE_API_KEY` before binding to any non-loopback interface.
- Use a trusted TLS reverse proxy, network allowlist, rate limiter, and process isolation for any non-local deployment.
- Restrict access to the credential directory to the service account.
- Never commit credentials or pass tokens in URLs.
- Do not expose this service as a multi-user subscription-sharing gateway.

The bridge API key protects inbound HTTP access. It is separate from the OAuth credential used with OpenAI.
