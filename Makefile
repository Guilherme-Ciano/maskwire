GO        ?= go
LINT      ?= golangci-lint
BIN       := bin
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X main.version=$(VERSION)
FUZZTIME  ?= 30s
TOOLS     := $(GO) tool -modfile=tools/go.mod

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-16s %s\n", $$1, $$2}'

.PHONY: fmt
fmt: ## Format code with the golangci-lint formatters
	$(LINT) fmt ./...

.PHONY: lint
lint: ## Lint, check for comments and tidy go.mod
	$(LINT) run ./...
	$(GO) run ./internal/tools/nocomments
	$(GO) mod tidy -diff

.PHONY: test
test: ## Unit and integration tests with the race detector
	$(GO) test -race -shuffle=on -count=1 ./...

.PHONY: cover
cover: ## Coverage report and gate (80% of internal packages)
	$(GO) test -race -covermode=atomic -coverpkg=./internal/... -coverprofile=cover.out ./...
	$(GO) run ./internal/tools/covergate -profile cover.out -min 80

.PHONY: fuzz
fuzz: ## Run every fuzz target for FUZZTIME
	$(GO) test -run='^$$' -fuzz='^FuzzRestoreSplit$$' -fuzztime=$(FUZZTIME) ./internal/restore
	$(GO) test -run='^$$' -fuzz='^FuzzMatcher$$' -fuzztime=$(FUZZTIME) ./internal/restore
	$(GO) test -run='^$$' -fuzz='^FuzzDetect$$' -fuzztime=$(FUZZTIME) ./internal/detect
	$(GO) test -run='^$$' -fuzz='^FuzzMaskRoundTrip$$' -fuzztime=$(FUZZTIME) ./internal/mask
	$(GO) test -run='^$$' -fuzz='^FuzzMaskRequest$$' -fuzztime=$(FUZZTIME) ./internal/codec/openai
	$(GO) test -run='^$$' -fuzz='^FuzzReader$$' -fuzztime=$(FUZZTIME) ./internal/sse
	$(GO) test -run='^$$' -fuzz='^FuzzRewriteIdentity$$' -fuzztime=$(FUZZTIME) ./internal/jsonx

.PHONY: bench
bench: ## Micro-benchmarks (10 runs) into bench/results/raw
	@mkdir -p bench/results/raw
	$(GO) test -run='^$$' -bench=. -benchmem -count=10 ./internal/... | tee bench/results/raw/$(shell git rev-parse --short HEAD 2>/dev/null || echo local).txt

.PHONY: load
load: ## HTTP overhead load test (needs make build)
	cd bench && $(GO) run ./cmd/loadgen -out results

.PHONY: vuln
vuln: ## govulncheck
	$(TOOLS) govulncheck ./...

.PHONY: build
build: ## Build maskwire and mockllm into bin/
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags='$(LDFLAGS)' -o $(BIN)/maskwire ./cmd/maskwire
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags='$(LDFLAGS)' -o $(BIN)/mockllm ./cmd/mockllm

.PHONY: docker
docker: ## Build container images
	docker build --target maskwire -t maskwire:dev .
	docker build --target mockllm -t mockllm:dev .

.PHONY: up
up: ## Start the compose demo
	docker compose up --build -d --wait

.PHONY: demo
demo: ## Run the demo client in a container against the compose stack
	docker compose --profile demo run --rm --build demo

.PHONY: demo-local
demo-local: ## Run the demo client with the local Go toolchain
	$(GO) run ./cmd/demo

.PHONY: down
down: ## Stop the compose demo
	docker compose down -v

.PHONY: snapshot
snapshot: ## goreleaser snapshot build
	goreleaser release --snapshot --clean --skip=sign,publish

.PHONY: clean
clean: ## Remove build output
	rm -rf $(BIN) dist cover.out
