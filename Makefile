VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build test test-int lint run proto proto-lint release-dry plugin-dev plugin-test marketplace-seed clean license-check completions

all: build

build:
	mkdir -p ./bin
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ./bin/siply ./cmd/siply

test:
	go test -race -parallel 4 ./...

test-int:
	go test -race -tags integration ./test/integration/...

lint:
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

run: build
	./bin/siply

proto:
	@which buf > /dev/null 2>&1 || (echo "buf CLI not installed — see https://buf.build/docs/installation"; exit 1)
	rm -rf api/proto/gen
	cd api/proto && buf generate

proto-lint:
	cd api/proto && buf lint

release-dry:
	@which goreleaser > /dev/null 2>&1 && goreleaser release --snapshot --clean || echo "goreleaser not installed, skipping"

plugin-dev: build
	@test -n "$(NAME)" || (echo "Usage: make plugin-dev NAME=<plugin-name>" && exit 1)
	@echo "Installing plugin $(NAME) from local path..."
	./bin/siply plugins install --dev ./$(NAME)

plugin-build-tree-sitter:
	mkdir -p ./bin
	cd plugins/tree-sitter && CGO_ENABLED=1 go build -ldflags "-s -w" -o ../../bin/siply-tree-sitter .

plugin-build-context-distillation:
	mkdir -p ./bin
	cd plugins/context-distillation && CGO_ENABLED=0 go build -ldflags "-s -w" -o ../../bin/siply-context-distillation .

plugin-test:
	@echo "Running plugin tests..."
	cd plugins/tree-sitter && CGO_ENABLED=1 go test -race ./...
	cd plugins/context-distillation && CGO_ENABLED=0 go test ./...

marketplace-seed:
	@bash scripts/seed-marketplace-index.sh

license-check:
	@missing_spdx=$$(find . -name '*.go' -type f -not -path './api/proto/gen/*' -not -path './vendor/*' -print0 | xargs -0 grep -L 'SPDX-License-Identifier: Apache-2.0'); \
	missing_copy=$$(find . -name '*.go' -type f -not -path './api/proto/gen/*' -not -path './vendor/*' -print0 | xargs -0 grep -L 'Copyright.*Simply Devly contributors'); \
	missing="$$missing_spdx$$missing_copy"; \
	if [ -n "$$missing_spdx" ] || [ -n "$$missing_copy" ]; then \
		echo "ERROR: Missing license header in:"; \
		[ -n "$$missing_spdx" ] && echo "Missing SPDX identifier:" && echo "$$missing_spdx"; \
		[ -n "$$missing_copy" ] && echo "Missing Copyright line:" && echo "$$missing_copy"; \
		echo ""; \
		echo "Add this header to the top of each file:"; \
		echo "// SPDX-License-Identifier: Apache-2.0"; \
		echo "// Copyright 2026 Simply Devly contributors"; \
		exit 1; \
	else \
		echo "All .go files have license headers ✓"; \
	fi

completions: build
	@mkdir -p ./scripts/completions
	./bin/siply completion bash > ./scripts/completions/siply.bash
	./bin/siply completion zsh > ./scripts/completions/_siply
	./bin/siply completion fish > ./scripts/completions/siply.fish
	@echo "Completion scripts generated in scripts/completions/"

clean:
	rm -rf ./bin/ ./dist/ ./siply
