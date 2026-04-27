GO         ?= go
LDFLAGS    := -s -w
BUILD_DIR  := build
BINS       := platypus-server platypus-agent
PROTO_V2_SRC := $(wildcard proto/v2/*.proto)
PROTO_V2_OUT := pkg/proto/v2/common.pb.go

# AGENT_SIGNING_PUBKEY is the base64-encoded Ed25519 public key baked
# into the platypus-agent binary. The release pipeline signs the update
# manifest with the matching private key; agents refuse to self-update
# if this is empty (an unsigned channel would be worse than none).
# Provide it via environment variable at build time.
AGENT_SIGNING_PUBKEY ?=
AGENT_LDFLAGS := $(LDFLAGS) -X github.com/WangYihang/Platypus/internal/agent.SigningPublicKey=$(AGENT_SIGNING_PUBKEY)

.PHONY: all build proto test lint fmt vet tidy clean release snapshot help swag \
        hooks pre-commit \
        desktop-deps desktop-dev desktop-build desktop-test desktop-bindings \
        web-ui web-ui-serve e2e e2e-deps screenshots

all: build

help:
	@echo "Server / agent (./cmd/...):"
	@echo "  build           Build both binaries to ./$(BUILD_DIR)/"
	@echo "  proto           Regenerate protobuf code"
	@echo "  test            Run tests with race detector"
	@echo "  lint            Run golangci-lint"
	@echo "  fmt             Format Go source"
	@echo "  vet             Run go vet"
	@echo "  tidy            Run go mod tidy"
	@echo "  hooks           Install git pre-commit hooks (requires 'pip install pre-commit')"
	@echo "  pre-commit      Run all pre-commit hooks against every tracked file"
	@echo "  snapshot        Build cross-platform snapshot via goreleaser"
	@echo "  release         Cut a release via goreleaser (requires tag + GITHUB_TOKEN)"
	@echo "  clean           Remove build artifacts"
	@echo ""
	@echo "Desktop app (./desktop):"
	@echo "  desktop-deps     Install Wails CLI + frontend pnpm deps"
	@echo "  desktop-bindings Regenerate Wails JS↔Go bindings under desktop/frontend/wailsjs/"
	@echo "  desktop-dev      Run Wails dev mode (hot reload)"
	@echo "  desktop-build    Build a native binary for the current platform"
	@echo "  desktop-test     Run desktop Go tests with race detector"
	@echo ""
	@echo "Standalone web UI (no server embed):"
	@echo "  web-ui           Build browser bundle to desktop/frontend/dist-web/"
	@echo "  web-ui-serve     Preview dist-web/ at http://localhost:7777"
	@echo ""
	@echo "End-to-end tests + screenshot gallery:"
	@echo "  e2e-deps         Install Playwright + browsers under e2e/"
	@echo "  e2e              Run the full Playwright suite (boots backend + agent + vite, writes docs/screenshots/)"
	@echo "  screenshots      Alias for e2e — run the suite and rebuild docs/screenshots/README.md"

$(PROTO_V2_OUT): $(PROTO_V2_SRC)
	protoc \
	  --proto_path=proto/v2 \
	  --go_out=pkg/proto/v2 \
	  --go_opt=paths=source_relative \
	  $(notdir $(PROTO_V2_SRC))

proto: $(PROTO_V2_OUT)

build: proto
	@mkdir -p $(BUILD_DIR)
	@for b in $(BINS); do \
	  echo "→ $$b"; \
	  if [ "$$b" = "platypus-agent" ]; then \
	    $(GO) build -ldflags="$(AGENT_LDFLAGS)" -trimpath \
	      -o $(BUILD_DIR)/$$b ./cmd/$$b || exit 1; \
	  else \
	    $(GO) build -ldflags="$(LDFLAGS)" -trimpath \
	      -o $(BUILD_DIR)/$$b ./cmd/$$b || exit 1; \
	  fi \
	done

test:
	# 300s per test binary covers slow packages (mesh / storage /
	# auth) under -race overhead. Individual packages stay well
	# below this; the timeout is a safety net, not a target.
	$(GO) test -race -count=1 -timeout=300s ./...

lint:
	golangci-lint run ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

hooks:
	pre-commit install

pre-commit:
	pre-commit run --all-files

# swag regenerates docs/swagger.yaml + docs/swagger.json from the //@... tags
# on the API handlers. Run this any time those tags change; the result is
# committed so the binary can embed them without a build-time codegen step.
SWAG ?= $(shell $(GO) env GOPATH)/bin/swag

swag:
	$(SWAG) init --generalInfo cmd/platypus-server/main.go --output docs --parseDependency --parseInternal

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
# GOEXE resolves to ".exe" on Windows (empty elsewhere). GOPATH on Windows
# uses backslashes which bash strips when interpreting the recipe (so
# "C:\Users\x\go/bin/wails.exe" becomes "C:Usersxgo/bin/wails.exe" and
# dies with exit 127). subst normalises separators.
WAILS      ?= $(subst \,/,$(shell $(GO) env GOPATH))/bin/wails$(shell $(GO) env GOEXE)

desktop-deps:
	$(GO) install github.com/wailsapp/wails/v2/cmd/wails@latest
	cd desktop/frontend && pnpm install

desktop-bindings:
	cd desktop && $(WAILS) generate module

desktop-dev:
	cd desktop && $(WAILS) dev -tags "$(WAILS_TAGS)"

desktop-build:
	cd desktop && $(WAILS) build -clean -tags "$(WAILS_TAGS)" -ldflags "$(LDFLAGS)"

desktop-test:
	cd desktop && $(GO) test -race -count=1 -timeout=120s ./internal/...

# ---------- Standalone web UI ----------
#
# Reuses desktop/frontend/src/* with vite mode=web. Output is a static
# bundle you can open in any browser — no server embed, no /ui/ route.
# Point it at any running platypus-server via the login form.

web-ui:
	cd desktop/frontend && pnpm install && pnpm run build:web

# ---------- End-to-end tests + screenshot gallery ----------
#
# `e2e` boots a fresh backend (temp SQLite + bootstrap), spawns one
# baseline platypus-agent against the seeded listener, starts the vite
# dev server, runs every spec in e2e/specs/, writes screenshots into
# docs/screenshots/, and rebuilds the gallery README. Both server and
# agent binaries must already be built (`make build`).

e2e-deps:
	cd e2e && pnpm install && pnpm exec playwright install chromium

e2e: build e2e-deps
	cd e2e && pnpm run e2e

screenshots: e2e

# Tiny preview so you can `make web-ui-serve` and browse
# http://localhost:7777 without needing nginx/caddy. Uses scripts/spa-serve.py
# so React Router routes (e.g. /projects/<slug>/enrollment) survive a refresh
# instead of returning 404 — plain `python -m http.server` can't do that.
web-ui-serve:
	@echo "Serving desktop/frontend/dist-web/ at http://localhost:7777 (Ctrl-C to stop)"
	@( \
		command -v python3 >/dev/null && python3 scripts/spa-serve.py 7777 desktop/frontend/dist-web || \
		command -v python  >/dev/null && python  scripts/spa-serve.py 7777 desktop/frontend/dist-web || \
		(echo "No python available — install python3, or use any static host with SPA fallback." && exit 1) \
	)
