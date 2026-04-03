VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build test test-int lint run proto proto-lint release-dry plugin-dev plugin-test clean

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

clean:
	rm -rf ./bin/ ./dist/ ./siply
