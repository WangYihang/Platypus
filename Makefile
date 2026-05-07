GO         ?= go
BUILD_DIR  := build
LDFLAGS    := -s -w
BINS       := platypus-server platypus-agent platypus-cli
BIN_PATHS  := $(addprefix $(BUILD_DIR)/,$(BINS))

# Base64 Ed25519 pubkey baked into platypus-agent — the agent refuses
# to self-update if empty (better than an unsigned channel).
AGENT_SIGNING_PUBKEY ?=
AGENT_LDFLAGS        := $(LDFLAGS) -X github.com/WangYihang/Platypus/internal/agent.SigningPublicKey=$(AGENT_SIGNING_PUBKEY)

PROTO_V2_SRC := $(wildcard proto/v2/*.proto)
PROTO_V2_OUT := pkg/proto/v2/common.pb.go
IP2REGION_V4 := internal/ipinfo/data/ip2region_v4.xdb

.DEFAULT_GOAL := all
.PHONY: all build proto test lint fmt vet tidy clean release snapshot help swag \
        hooks pre-commit data data-v6 \
        example-plugins stage-system-plugins \
        desktop-deps desktop-dev desktop-build desktop-test desktop-bindings \
        web-ui web-ui-embed web-ui-serve e2e e2e-deps screenshots \
        $(BIN_PATHS)

# `make` after a fresh clone produces a fully-functioning ./build/*:
# data fetched, web UI baked in, all binaries built. Everything is
# file-tracked, so re-runs only redo stale artefacts.
all: $(IP2REGION_V4) web-ui-embed build

help:
	@echo "Quick start:"
	@echo "  make                  data + web UI + binaries (full local dev build)"
	@echo "  make build            Go binaries only (fast iteration)"
	@echo ""
	@echo "Server / agent / CLI:"
	@echo "  proto                 Regenerate protobuf code"
	@echo "  test                  Run tests with race detector"
	@echo "  lint / fmt / vet      golangci-lint / go fmt / go vet"
	@echo "  tidy                  go mod tidy"
	@echo "  swag                  Regenerate docs/swagger.{yaml,json}"
	@echo "  snapshot / release    goreleaser builds"
	@echo "  data / data-v6        Fetch ip2region xdb (v4 / v4+v6)"
	@echo "  hooks / pre-commit    Install / run pre-commit hooks"
	@echo "  clean                 Remove build artefacts"
	@echo ""
	@echo "Plugins:"
	@echo "  example-plugins       Build + sign every plugin under example/plugins/"
	@echo "  stage-system-plugins  Build + sign + stage plugins into the server embed"
	@echo ""
	@echo "Desktop app:"
	@echo "  desktop-deps          Wails CLI + frontend deps"
	@echo "  desktop-bindings      Regenerate Wails JS↔Go bindings"
	@echo "  desktop-dev           Hot-reload dev mode"
	@echo "  desktop-build         Native binary"
	@echo "  desktop-test          Desktop Go tests (race)"
	@echo ""
	@echo "Web UI / e2e:"
	@echo "  web-ui                Build standalone bundle to desktop/frontend/dist-web/"
	@echo "  web-ui-embed          Build + stage into internal/webui/dist/"
	@echo "  web-ui-serve          Preview at http://localhost:7777"
	@echo "  e2e-deps              Install Playwright"
	@echo "  e2e / screenshots     Run the full Playwright suite"

# ---------- Protobuf ----------

$(PROTO_V2_OUT): $(PROTO_V2_SRC)
	protoc --proto_path=proto/v2 --go_out=pkg/proto/v2 --go_opt=paths=source_relative $(notdir $(PROTO_V2_SRC))

proto: $(PROTO_V2_OUT)

# ---------- Go binaries ----------
# Phony so Go's build cache (not Make) decides whether to do real
# work. The agent gets AGENT_LDFLAGS so the signing pubkey is baked
# in at link time; the others use plain LDFLAGS.

build: $(BIN_PATHS)

$(BIN_PATHS): $(PROTO_V2_OUT)
	@mkdir -p $(@D)
	@echo "→ $(@F)"
	@$(GO) build \
	  -ldflags="$(if $(filter %-agent,$(@F)),$(AGENT_LDFLAGS),$(LDFLAGS))" \
	  -trimpath -o $@ ./cmd/$(@F)

# ---------- Quality gates ----------

# 600s covers slow packages (api / storage / mesh) under -race; the
# api suite alone runs ~150 tests that each spin up a fresh sqlite.
test:
	$(GO) test -race -count=1 -timeout=600s ./...

lint:    ; golangci-lint run ./...
fmt:     ; $(GO) fmt ./...
vet:     ; $(GO) vet ./...
tidy:    ; $(GO) mod tidy
hooks:       ; pre-commit install
pre-commit:  ; pre-commit run --all-files
snapshot:    ; goreleaser build --snapshot --clean
release:     ; goreleaser release --clean

SWAG ?= $(shell $(GO) env GOPATH)/bin/swag
swag:
	$(SWAG) init --generalInfo cmd/platypus-server/main.go --output docs --parseDependency --parseInternal

# ---------- Plugins ----------
# Both targets need rustup + wasm32-unknown-unknown. example-plugins
# also needs PLATYPUS_PUBLISHER_KEY (from `platypus-cli plugin keygen`).
# stage-system-plugins picks up TinyGo plugins too, but skips them
# (with a warning) when tinygo isn't on PATH.

example-plugins:
	@: $${PLATYPUS_PUBLISHER_KEY:?required: path to a plugin keygen secret}
	@test -x $(BUILD_DIR)/platypus-cli || { echo "run \`make build\` first"; exit 1; }
	@for d in example/plugins/*/Cargo.toml; do \
	  dir=$$(dirname $$d); echo "→ $$(basename $$dir)"; \
	  (cd $$dir && cargo build --release --target wasm32-unknown-unknown) || exit 1; \
	  wasm=$$(ls -1 $$dir/target/wasm32-unknown-unknown/release/*.wasm 2>/dev/null); \
	  [ "$$(echo "$$wasm" | wc -l)" = 1 ] || { echo "expected exactly 1 .wasm under $$dir"; exit 1; }; \
	  $(BUILD_DIR)/platypus-cli plugin sign --force --key $$PLATYPUS_PUBLISHER_KEY --wasm $$wasm || exit 1; \
	done

stage-system-plugins:
	@for d in example/plugins/system/*/Cargo.toml; do \
	  dir=$$(dirname $$d); echo "→ rust:$$(basename $$dir)"; \
	  (cd $$dir && cargo build --release --target wasm32-unknown-unknown) || exit 1; \
	done
	@if command -v tinygo >/dev/null 2>&1; then \
	  for d in example/plugins/system-go/*/go.mod; do \
	    dir=$$(dirname $$d); entry=$$(awk '/^  entry:/ {print $$2}' $$dir/plugin.yaml); \
	    echo "→ go:$$(basename $$dir) ($$entry)"; \
	    (cd $$dir && tinygo build -target wasi -o $$entry .) || exit 1; \
	  done; \
	else echo "warning: tinygo not on PATH — system-go/ plugins skipped"; fi
	$(GO) run ./hack/stage_system_plugins

# ---------- Geo data ----------
# The Go binary used to embed ip2region but stopped doing so to slim
# the binary by ~11 MB. The fetch script no-ops when the file's
# already there.

$(IP2REGION_V4):
	./scripts/fetch-ip2region.sh

data: $(IP2REGION_V4)

data-v6:
	./scripts/fetch-ip2region.sh --v6

# ---------- Clean ----------
# Strip the staged web bundle but keep the committed stub so a plain
# `go build` still works without re-running web-ui-embed.

clean:
	rm -rf $(BUILD_DIR) dist
	rm -rf desktop/build/bin desktop/frontend/dist desktop/frontend/wailsjs
	@find internal/webui/dist -mindepth 1 \
	  ! -name index.html ! -name .gitkeep ! -name .gitignore -delete 2>/dev/null || true

# ---------- Desktop app ----------
# webkit2_41 is a no-op on macOS / Windows, required on Linux (only
# webkit2gtk-4.1 ships on Ubuntu 22.04+ / Fedora 37+ / Debian 12+).
# subst normalises GOPATH separators so Wails resolves on Windows too.

WAILS_TAGS ?= webkit2_41
WAILS      ?= $(subst \,/,$(shell $(GO) env GOPATH))/bin/wails$(shell $(GO) env GOEXE)

desktop-deps:
	$(GO) install github.com/wailsapp/wails/v2/cmd/wails@latest
	cd desktop/frontend && pnpm install

desktop-bindings:  ; cd desktop && $(WAILS) generate module
desktop-dev:       ; cd desktop && $(WAILS) dev -tags "$(WAILS_TAGS)"
desktop-build:     ; cd desktop && $(WAILS) build -clean -tags "$(WAILS_TAGS)" -ldflags "$(LDFLAGS)"
desktop-test:      ; cd desktop && $(GO) test -race -count=1 -timeout=120s ./internal/...

# ---------- Web UI ----------
# Reuses desktop/frontend/src/* with vite mode=web — no server embed,
# no /ui/ route. Login form points at any platypus-server.

web-ui:
	cd desktop/frontend && pnpm install && pnpm run build:web

# rsync --delete drops stale files from a previous build; the
# excludes preserve the committed stub + .gitignore.
web-ui-embed: web-ui
	@mkdir -p internal/webui/dist
	rsync -a --delete --exclude='.gitignore' --exclude='.gitkeep' \
	  desktop/frontend/dist-web/ internal/webui/dist/

# Vite preview has SPA history fallback, so React Router routes
# survive a refresh — `python -m http.server` can't.
web-ui-serve:
	@cd desktop/frontend && pnpm preview:web --port 7777

# ---------- E2E ----------
# Boots a fresh backend (temp SQLite + bootstrap), spawns one agent,
# starts vite, runs every spec in e2e/specs/, writes screenshots
# into docs/screenshots/, rebuilds the gallery README.

e2e-deps:
	cd e2e && pnpm install && pnpm exec playwright install chromium

e2e: build e2e-deps
	cd e2e && pnpm run e2e

screenshots: e2e
