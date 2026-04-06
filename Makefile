VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build test test-int lint run proto proto-lint release-dry plugin-dev plugin-test clean license-check

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

plugin-dev:
	@echo "plugin-dev: placeholder — no plugins yet"

plugin-test:
	@echo "plugin-test: placeholder — no plugin tests yet"

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

clean:
	rm -rf ./bin/ ./dist/ ./siply
