GO         ?= go
BUILD_DIR  := build
LDFLAGS    := -s -w
BINS       := platypus-server platypus-agent platypus-cli
BIN_PATHS  := $(addprefix $(BUILD_DIR)/,$(BINS))

# Dev signing keypair for the local-dev release pipeline. Auto-minted
# on first `make releases`; persists across runs so previously-enrolled
# agents keep verifying upgrade manifests. AGENT_SIGNING_PUBKEY picks
# up the dev pubkey automatically when present so `make build` and
# `make releases` produce binaries with matching trust roots. CI /
# production override AGENT_SIGNING_PUBKEY explicitly via env.
DEV_SIGNING_KEY    := scripts/.agent-signing.pem
DEV_SIGNING_PUBKEY := scripts/.agent-signing.pub.b64
# `=` (deferred) so the shell read of the sentinel happens each time
# AGENT_LDFLAGS expands — `make all` mints the keypair part-way through,
# and the host build (later in the chain) needs to pick up the freshly-
# minted pubkey. Immediate expansion (`:=`) would freeze the empty
# parse-time value.
AGENT_SIGNING_PUBKEY ?= $(shell [ -f $(DEV_SIGNING_PUBKEY) ] && cat $(DEV_SIGNING_PUBKEY))
AGENT_LDFLAGS         = $(LDFLAGS) -X github.com/WangYihang/Platypus/internal/agent.SigningPublicKey=$(AGENT_SIGNING_PUBKEY)

# Local-dev release tree: signed manifest + cross-platform agent
# binaries that the enrollment download endpoint serves out of
# <data-dir>/releases/. Without this tree every install link 404s.
DATA_DIR          ?= data
RELEASES_VERSION  ?= 0.0.0-dev
RELEASES_CHANNEL  ?= stable
RELEASES_MANIFEST := $(DATA_DIR)/releases/manifest/$(RELEASES_CHANNEL).json

PROTO_V2_SRC := $(wildcard proto/v2/*.proto)
PROTO_V2_OUT := pkg/proto/v2/common.pb.go
IP2REGION_V4 := internal/ipinfo/data/ip2region_v4.xdb

.DEFAULT_GOAL := all
.PHONY: all build proto test lint fmt vet tidy clean release snapshot help swag \
        hooks pre-commit data data-v6 releases check-deps \
        example-plugins stage-system-plugins \
        desktop-deps desktop-dev desktop-build desktop-test desktop-bindings \
        web-ui web-ui-embed web-ui-serve e2e e2e-deps screenshots \
        $(BIN_PATHS)

# ---------- Dependency preflight ----------
# Recipes call `$(call require-bin,<tool>,<install hint>)` inline so
# the failure happens at the point of need, with an actionable fix.
# `make check-deps` runs the same survey across every tool the local-
# dev `make` flow expects, marking optional ones (upx, protoc) so a
# missing one doesn't look fatal.
require-bin = command -v $(1) >/dev/null 2>&1 || { \
  printf >&2 '\033[31m✗ %s not found on PATH\033[0m\n  → install: %s\n' '$(1)' '$(2)'; \
  exit 1; \
}
check-bin = if command -v $(1) >/dev/null 2>&1; then \
              printf '  \033[32m✓\033[0m %-12s %s\n' '$(1)' "$$(command -v $(1))"; \
            else \
              printf '  \033[31m✗\033[0m %-12s missing — %s\n' '$(1)' '$(2)'; \
            fi

check-deps:
	@printf "Build dependencies for the local-dev \`make\` flow:\n\n"
	@printf "Required:\n"
	@$(call check-bin,go,install Go from https://go.dev/dl/)
	@$(call check-bin,openssl,apt install openssl  /  brew install openssl)
	@$(call check-bin,goreleaser,go install github.com/goreleaser/goreleaser/v2@latest)
	@$(call check-bin,pnpm,npm i -g pnpm  /  curl -fsSL https://get.pnpm.io/install.sh | sh -)
	@$(call check-bin,rsync,apt install rsync  /  brew install rsync)
	@$(call check-bin,curl,apt install curl  /  brew install curl)
	@printf "\nOptional:\n"
	@$(call check-bin,upx,apt install upx-ucl  /  brew install upx  (halves the agent binary size))
	@$(call check-bin,protoc,apt install protobuf-compiler  /  brew install protobuf  (only when proto sources change))
	@$(call check-bin,cargo,https://rustup.rs/  (only for plugin authoring))

# `make` after a fresh clone produces a fully-functioning ./build/*:
# data fetched, web UI baked in, signed cross-platform agent releases
# staged, host binaries built. Everything is file-tracked, so re-runs
# only redo stale artefacts. `releases` runs before `build` so the
# host agent links against the same dev signing pubkey staged for
# cross-platform downloads.
all: $(IP2REGION_V4) web-ui-embed releases build

help:
	@echo "Quick start:"
	@echo "  make                  data + web UI + binaries (full local dev build)"
	@echo "  make build            Go binaries only (fast iteration)"
	@echo "  make check-deps       Survey required + optional toolchain (install hints)"
	@echo ""
	@echo "Server / agent / CLI:"
	@echo "  proto                 Regenerate protobuf code"
	@echo "  test                  Run tests with race detector"
	@echo "  lint / fmt / vet      golangci-lint / go fmt / go vet"
	@echo "  tidy                  go mod tidy"
	@echo "  swag                  Regenerate docs/swagger.{yaml,json}"
	@echo "  snapshot / release    goreleaser builds"
	@echo "  data / data-v6        Fetch ip2region xdb (v4 / v4+v6)"
	@echo "  releases              Cross-compile + sign agent releases under data/releases/ (via goreleaser)"
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
	@$(call require-bin,protoc,apt install protobuf-compiler  /  brew install protobuf)
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

example-plugins:
	@$(call require-bin,cargo,https://rustup.rs/  +  rustup target add wasm32-unknown-unknown)
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
	@$(call require-bin,cargo,https://rustup.rs/  +  rustup target add wasm32-unknown-unknown)
	@for d in example/plugins/system/*/Cargo.toml; do \
	  dir=$$(dirname $$d); echo "→ $$(basename $$dir)"; \
	  (cd $$dir && cargo build --release --target wasm32-unknown-unknown) || exit 1; \
	done
	$(GO) run ./scripts/stage_system_plugins

# ---------- Geo data ----------
# The Go binary used to embed ip2region but stopped doing so to slim
# the binary by ~11 MB. The fetch script no-ops when the file's
# already there.

$(IP2REGION_V4):
	@$(call require-bin,curl,apt install curl  /  brew install curl)
	./scripts/fetch-ip2region.sh

data: $(IP2REGION_V4)

data-v6:
	@$(call require-bin,curl,apt install curl  /  brew install curl)
	./scripts/fetch-ip2region.sh --v6

# ---------- Local-dev releases ----------
# `releases` mints a dev Ed25519 keypair, runs goreleaser to cross-
# compile platypus-agent for every supported GOOS/GOARCH, then signs
# + lays out a release tree the server's enrollment endpoint serves
# out of <data-dir>/releases/. Idempotent — sentinel files keep warm
# runs cheap. Override RELEASES_VERSION / RELEASES_CHANNEL / DATA_DIR
# via env to target a non-default tree.

# Six-line openssl recipe lifted verbatim from
# scripts/dev-publish-entrypoint.sh:28-37 (dev compose sidecar).
$(DEV_SIGNING_KEY) $(DEV_SIGNING_PUBKEY):
	@$(call require-bin,openssl,apt install openssl  /  brew install openssl)
	@mkdir -p scripts
	@openssl genpkey -algorithm ED25519 -out $(DEV_SIGNING_KEY)
	@openssl pkey -in $(DEV_SIGNING_KEY) -pubout -outform DER \
	  | tail -c 32 | base64 -w0 > $(DEV_SIGNING_PUBKEY)
	@chmod 0600 $(DEV_SIGNING_KEY)
	@echo "→ minted dev signing keypair under scripts/"

# `--config .goreleaser.dev.yaml` selects a focused config that only
# builds platypus-agent for the proven-compiling matrix (the production
# .goreleaser.yaml promises broader targets that don't all build under
# our pinned gopsutil version — that's a release-pipeline concern, not
# a local-dev one). AGENT_SIGNING_PUBKEY_B64 feeds the ldflags entry
# so installed agents trust the same dev key the manifest is signed
# with.
$(RELEASES_MANIFEST): $(DEV_SIGNING_KEY) $(DEV_SIGNING_PUBKEY)
	@$(call require-bin,goreleaser,go install github.com/goreleaser/goreleaser/v2@latest)
	@command -v upx >/dev/null 2>&1 || \
	  printf >&2 '\033[33mℹ\033[0m upx not on PATH — agent binaries will ship uncompressed (~2x download size). install: apt install upx-ucl  /  brew install upx\n'
	@echo "→ building cross-platform agents via goreleaser"
	AGENT_SIGNING_PUBKEY_B64=$$(cat $(DEV_SIGNING_PUBKEY)) \
	  goreleaser build --config .goreleaser.dev.yaml --snapshot --clean
	@echo "→ staging + signing manifest"
	@$(GO) run ./scripts/stage_releases \
	  --dist dist \
	  --releases-dir $(DATA_DIR)/releases \
	  --version $(RELEASES_VERSION) \
	  --channel $(RELEASES_CHANNEL) \
	  --privkey $(DEV_SIGNING_KEY)

releases: $(RELEASES_MANIFEST)

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
	@$(call require-bin,pnpm,npm i -g pnpm)
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
	@$(call require-bin,pnpm,npm i -g pnpm  /  curl -fsSL https://get.pnpm.io/install.sh | sh -)
	cd desktop/frontend && pnpm install && pnpm run build:web

# rsync --delete drops stale files from a previous build; the
# excludes preserve the committed stub + .gitignore.
web-ui-embed: web-ui
	@$(call require-bin,rsync,apt install rsync  /  brew install rsync)
	@mkdir -p internal/webui/dist
	rsync -a --delete --exclude='.gitignore' --exclude='.gitkeep' \
	  desktop/frontend/dist-web/ internal/webui/dist/

# Vite preview has SPA history fallback, so React Router routes
# survive a refresh — `python -m http.server` can't.
web-ui-serve:
	@$(call require-bin,pnpm,npm i -g pnpm)
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
