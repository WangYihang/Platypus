GO         ?= go
LDFLAGS    := -s -w
BUILD_DIR  := build
BINS       := platypus-server platypus-admin platypus-agent
PROTO_SRC  := proto/agent/v1/agent.proto
PROTO_OUT  := pkg/proto/agent/v1/agent.pb.go

.PHONY: all build proto test lint fmt vet tidy clean release snapshot help

all: build

help:
	@echo "Targets:"
	@echo "  build      Build all three binaries to ./$(BUILD_DIR)/"
	@echo "  proto      Regenerate protobuf code"
	@echo "  test       Run tests with race detector"
	@echo "  lint       Run golangci-lint"
	@echo "  fmt        Format Go source"
	@echo "  vet        Run go vet"
	@echo "  tidy       Run go mod tidy"
	@echo "  snapshot   Build cross-platform snapshot via goreleaser"
	@echo "  release    Cut a release via goreleaser (requires tag + GITHUB_TOKEN)"
	@echo "  clean      Remove build artifacts"

$(PROTO_OUT): $(PROTO_SRC)
	protoc --go_out=pkg/proto/agent/v1 --go_opt=paths=source_relative $(PROTO_SRC)

proto: $(PROTO_OUT)

build: proto
	@mkdir -p $(BUILD_DIR)
	@for b in $(BINS); do \
	  echo "→ $$b"; \
	  $(GO) build -ldflags="$(LDFLAGS)" -trimpath \
	    -o $(BUILD_DIR)/$$b ./cmd/$$b || exit 1; \
	done

test:
	$(GO) test -race -count=1 -timeout=120s ./...

lint:
	golangci-lint run ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

snapshot:
	goreleaser build --snapshot --clean

release:
	goreleaser release --clean

clean:
	rm -rf $(BUILD_DIR) dist
