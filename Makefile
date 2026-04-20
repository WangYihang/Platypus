GO         ?= go
LDFLAGS    := -s -w
BUILD_DIR  := build
BINS       := platypus-server platypus-admin platypus-agent
PROTO_SRC  := proto/agent/v1/agent.proto
PROTO_OUT  := pkg/proto/agent/v1/agent.pb.go

.PHONY: all build proto test lint fmt vet tidy clean release snapshot help \
        desktop-deps desktop-dev desktop-build desktop-test desktop-bindings

all: build

help:
	@echo "Server / agent / admin (./cmd/...):"
	@echo "  build           Build all three binaries to ./$(BUILD_DIR)/"
	@echo "  proto           Regenerate protobuf code"
	@echo "  test            Run tests with race detector"
	@echo "  lint            Run golangci-lint"
	@echo "  fmt             Format Go source"
	@echo "  vet             Run go vet"
	@echo "  tidy            Run go mod tidy"
	@echo "  snapshot        Build cross-platform snapshot via goreleaser"
	@echo "  release         Cut a release via goreleaser (requires tag + GITHUB_TOKEN)"
	@echo "  clean           Remove build artifacts"
	@echo ""
	@echo "Desktop app (./desktop):"
	@echo "  desktop-deps     Install Wails CLI + frontend npm deps"
	@echo "  desktop-bindings Regenerate Wails JS↔Go bindings under desktop/frontend/wailsjs/"
	@echo "  desktop-dev      Run Wails dev mode (hot reload)"
	@echo "  desktop-build    Build a native binary for the current platform"
	@echo "  desktop-test     Run desktop Go tests with race detector"

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
	rm -rf desktop/build/bin desktop/frontend/dist desktop/frontend/wailsjs

# ---------- Desktop app ----------

# `webkit2_41` is a no-op on macOS / Windows but required on Linux where only
# webkit2gtk-4.1 ships (Ubuntu 22.04+, Fedora 37+, Debian 12+). Wails picks the
# right binding at compile time based on this tag.
WAILS_TAGS ?= webkit2_41
WAILS      ?= $(shell $(GO) env GOPATH)/bin/wails

desktop-deps:
	$(GO) install github.com/wailsapp/wails/v2/cmd/wails@latest
	cd desktop/frontend && npm install

desktop-bindings:
	cd desktop && $(WAILS) generate module

desktop-dev:
	cd desktop && $(WAILS) dev -tags "$(WAILS_TAGS)"

desktop-build:
	cd desktop && $(WAILS) build -clean -tags "$(WAILS_TAGS)"

desktop-test:
	cd desktop && $(GO) test -race -count=1 -timeout=120s ./internal/...
