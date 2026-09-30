# Contributing

Contributions are welcome. Keep changes focused, add tests for behavior changes, and use English for code, comments, documentation, issues, and commit messages.

## Development workflow

1. Create a branch from `main`.
2. Run `gofmt` on changed Go files.
3. Run `go test ./...` and `go vet ./...`.
4. Add or update protocol compatibility documentation when translation behavior changes.
5. Use a concise Conventional Commit message such as `fix: preserve rotated refresh tokens`.

Tests must not require a real OpenAI account or network access. Use `httptest.Server` and injectable HTTP/token interfaces.

## Pull requests

Describe the user-visible behavior, security implications, tests performed, and compatibility changes. Do not include credentials, captured authorization URLs, or personal account data.
