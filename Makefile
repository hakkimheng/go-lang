.DEFAULT_GOAL := help
BINARY := bin/api

.PHONY: help run build test cover fmt vet tidy clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2}'

run: ## Run the server (loads .env if present)
	@set -a; [ -f .env ] && . ./.env; set +a; go run ./cmd/api

build: ## Compile the server to bin/api
	go build -o $(BINARY) ./cmd/api

test: ## Run all tests with the race detector
	go test -race ./...

# -coverpkg credits the handler tests for the middleware and store code they
# exercise; without it each package only counts its own tests.
cover: ## Run tests and report total coverage
	go test -race -coverpkg=./... -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

fmt: ## Format all Go files
	gofmt -s -w .

vet: ## Run go vet
	go vet ./...

tidy: ## Sync go.mod/go.sum
	go mod tidy

clean: ## Remove build artifacts
	rm -rf bin coverage.out
