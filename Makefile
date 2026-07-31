.DEFAULT_GOAL := help

APP := ooshdaq
BIN_DIR := bin
APP_FILES := $(shell find cmd internal web -type f \
	\( -name '*.go' -o -name '*.sql' -o -name '*.json' -o -name '*.html' -o -name '*.css' -o -name '*.js' -o -name '*.svg' \) \
	2>/dev/null)

.PHONY: help run build test test-race test-web test-browser cover vet fmt check \
	docker-build container-smoke qualify clean

help: ## Show available commands.
	@awk 'BEGIN {FS = ":.*## "; printf "$$OOSHDAQ development commands:\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run: ## Run the local server with mock authentication.
	OOSHDAQ_AUTH_MODE=mock \
	OOSHDAQ_DATA_PATH=./data/ooshdaq.sqlite \
	OOSHDAQ_BACKUP_DIR=./backups \
	go run ./cmd/ooshdaq

build: $(BIN_DIR)/$(APP) ## Build the server binary.

$(BIN_DIR)/$(APP): $(APP_FILES) go.mod go.sum
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -mod=readonly -trimpath -buildvcs=false -o $@ ./cmd/ooshdaq

test: ## Run all tests.
	go test ./...

test-race: ## Run all tests with the race detector.
	go test -race ./...

test-web: ## Run dependency-free browser contract tests.
	node --test web/tests/*.test.mjs

test-browser: ## Run live Chromium journeys (after npm ci in web/).
	cd web && npm run test:e2e

cover: ## Write a coverage profile and print package coverage.
	@mkdir -p coverage
	go test -coverprofile=coverage/unit.coverprofile ./...
	go tool cover -func=coverage/unit.coverprofile

vet: ## Run Go's static analyzer.
	go vet ./...

fmt: ## Format Go sources and fail if formatting changes remain.
	gofmt -w $$(find cmd internal web -type f -name '*.go')

check: fmt vet test ## Run the routine local quality gate.
	@git diff --exit-code -- '*.go'

docker-build: ## Build the hardened local container image.
	docker build --pull --tag ooshdaq:local .

container-smoke: ## Prove image hardening, persistence, backup, and restore.
	scripts/container-smoke.sh

qualify: ## Run the fixed 10k-user/100-member release workload.
	@set -eu; \
	qualification_directory="$$(mktemp -d /tmp/ooshdaq-qualification.XXXXXX)"; \
	trap 'case "$$qualification_directory" in /tmp/ooshdaq-qualification.*) rm -rf -- "$$qualification_directory";; esac' EXIT; \
	go build -trimpath -o "$$qualification_directory/ooshdaq-qualify" ./cmd/ooshdaq-qualify; \
	systemd-run --user --scope --quiet \
		-p CPUQuota=100% \
		-p MemoryMax=256M \
		-p MemorySwapMax=0 \
		"$$qualification_directory/ooshdaq-qualify" \
		-mode full \
		-database "$$qualification_directory/qualification.sqlite"

clean: ## Remove generated build and coverage output.
	rm -rf $(BIN_DIR) coverage
