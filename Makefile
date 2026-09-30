.PHONY: build test lint fmt

build:
	go build -trimpath -o bin/codex-bridge ./cmd/codex-bridge

test:
	go test -race ./...

lint:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')
