VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test test-int lint run proto release-dry plugin-dev plugin-test clean

build:
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
	@echo "proto: buf generate (placeholder — no .proto files yet)"

release-dry:
	@which goreleaser > /dev/null 2>&1 && goreleaser release --snapshot --clean || echo "goreleaser not installed, skipping"

plugin-dev:
	@echo "plugin-dev: placeholder — no plugins yet"

plugin-test:
	@echo "plugin-test: placeholder — no plugin tests yet"

clean:
	rm -rf ./bin/ ./dist/ ./siply
