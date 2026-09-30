.PHONY: build dashboard test lint fmt

build: dashboard
	go build -trimpath -o bin/codex-bridge ./cmd/codex-bridge

dashboard:
	npm run --prefix web build

test:
	go test -race ./...

lint:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')
