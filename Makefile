AIR := $(shell which air 2>/dev/null)

.PHONY: dev build test

dev: ## Start hot-reload dev server (installs air if missing)
ifndef AIR
	go install github.com/air-verse/air@latest
endif
	air

build: ## Build the bot binary
	go build -o ./tmp/bot ./cmd/bot

test: ## Run tests with race detector
	go test -race ./...
