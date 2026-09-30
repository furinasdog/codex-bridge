.DEFAULT_GOAL := build

ifeq ($(OS),Windows_NT)
BINARY := bin/codex-bridge.exe
CREATE_BIN := powershell -NoProfile -Command "New-Item -ItemType Directory -Force bin | Out-Null"
else
BINARY := bin/codex-bridge
CREATE_BIN := mkdir -p bin
endif

DASHBOARD_OUTPUT := internal/dashboard/dist/index.html
DASHBOARD_SOURCES := web/package.json web/package-lock.json web/index.html \
	web/vite.config.ts web/tsconfig.json web/tsconfig.app.json web/tsconfig.node.json \
	web/public/favicon.svg $(wildcard web/src/* web/src/components/*)

.PHONY: build dashboard test test-race lint fmt

build: dashboard
	$(CREATE_BIN)
	go build -trimpath -o $(BINARY) ./cmd/codex-bridge

dashboard: $(DASHBOARD_OUTPUT)

$(DASHBOARD_OUTPUT): $(DASHBOARD_SOURCES)
	npm run --prefix web build

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	go vet ./...

fmt:
	go fmt ./...
