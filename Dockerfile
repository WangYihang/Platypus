# Stage 0: Build the React web UI bundle.
#
# The platypus-server embeds this bundle via //go:embed (see
# internal/webui/), so the Go builder needs the dist-web/ output staged
# under internal/webui/dist/ before `go build`. Doing it in a separate
# stage keeps the Go image free of Node and lets pnpm install cache on
# the lockfile alone.
FROM node:22-alpine AS frontend-builder
WORKDIR /fe
# pnpm via corepack — matches CI's pnpm/action-setup@v6 (version: 10).
# desktop/frontend/pnpm-lock.yaml is lockfileVersion 9.0, the format
# pnpm 10 writes natively, so --frozen-lockfile stays strict.
RUN corepack enable
COPY desktop/frontend/package.json desktop/frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY desktop/frontend/ ./
RUN pnpm run build:web

# Stage 1: Build the binaries
FROM golang:1.25 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Overlay the freshly built web bundle on top of the committed stub so
# //go:embed picks up the real frontend.
COPY --from=frontend-builder /fe/dist-web/ ./internal/webui/dist/

# Pull the ip2region v4 + v6 xdb files. The Go binary stopped embedding
# them (saved ~11 MB v4 + ~36 MB v6 on the server binary) and now
# resolves them at runtime from <exec dir>/data/, the XDG data dir, or
# ./data/. We stage them under /out/xdb/ here and the server runtime
# stage copies both next to the binary so the loader's "<exec dir>/data/"
# probe finds them — without v6 the dual-stack public-IP geo lookup
# falls back to "classification only" for v6 addresses.
RUN ./scripts/fetch-ip2region.sh --v6 \
    && mkdir -p /out/xdb \
    && cp internal/ipinfo/data/ip2region_v4.xdb /out/xdb/ \
    && cp internal/ipinfo/data/ip2region_v6.xdb /out/xdb/

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/platypus-server ./cmd/platypus-server

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/platypus-agent ./cmd/platypus-agent

# Empty dir baked into the server image so a fresh `platypus_data`
# named volume mounted at /app/data inherits 65532:65532 from the
# image — Docker copies a mount target's image content & ownership
# into a new volume on first attach. Without this the volume comes
# up root:root and the distroless `nonroot` runtime can't open
# platypus.db (SQLITE_CANTOPEN).
RUN mkdir -p /out/data

# Stage 2: Server runtime
FROM gcr.io/distroless/static-debian12:nonroot AS server
WORKDIR /app
COPY --from=builder /out/platypus-server /usr/local/bin/platypus-server
# ip2region xdb at <exec dir>/data/ — first hit in the runtime
# resolution order, and unaffected by the platypus_data volume mount
# at /app/data (which would otherwise shadow it).
COPY --from=builder /out/xdb/ip2region_v4.xdb /usr/local/bin/data/ip2region_v4.xdb
COPY --from=builder /out/xdb/ip2region_v6.xdb /usr/local/bin/data/ip2region_v6.xdb
COPY --from=builder --chown=nonroot:nonroot /out/data /app/data
USER nonroot:nonroot
EXPOSE 9443
ENTRYPOINT ["/usr/local/bin/platypus-server"]

# Stage 3: Agent runtime
FROM gcr.io/distroless/static-debian12:nonroot AS agent
WORKDIR /app
COPY --from=builder /out/platypus-agent /usr/local/bin/platypus-agent
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/platypus-agent"]

# Stage 4: Dev agent publisher
#
# Cross-compiles platypus-agent for the release matrix, signs the
# manifest with a dev Ed25519 key, and writes the result onto the
# shared platypus_data volume at /output/releases/ (mounted by the
# compose `agent-publisher` sidecar). The platypus-server's LocalStore
# reads the same path through PLATYPUS_DATA_DIR=/app/data, so a fresh
# `docker compose up` lands a working enrollment flow without any
# extra credentials / object store. Not used in production —
# production releases run scripts/release-publish.sh from CI with a
# vault-stored signing key, then rsync the resulting tree onto the
# server's data volume.
FROM golang:1.25 AS publisher
WORKDIR /workspace
# python3 + pyyaml are for the marketplace index.json generator the
# publisher entrypoint runs after building the example plugins. They
# add ~25 MB after caches and avoid pulling in a Go-based equivalent.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        openssl ca-certificates \
        curl build-essential \
        python3 python3-yaml \
    && rm -rf /var/lib/apt/lists/*
# Rust toolchain for compiling the example wasm plugins. The publisher
# script `cargo build`s every plugin under examples/plugins/ for
# wasm32-unknown-unknown. Pinned to stable; bumping rustup just to
# chase a nightly is rarely worth it.
#
# Two CARGO_HOMEs at play:
#   - /usr/local/cargo (build-time)  — where rustup drops the proxy
#     binaries that delegate to RUSTUP_HOME. Read-only after install
#     so the runtime user can't tamper with the toolchain.
#   - /cache/cargo-home (run-time)   — where `cargo build` populates
#     the registry index + crate downloads. Owned by uid 65532 and
#     mounted as a docker volume so warm rebuilds skip the network.
# The runtime ENV below overrides CARGO_HOME to the writable cache;
# rustup's proxy binaries on PATH still resolve via $RUSTUP_HOME.
ENV RUSTUP_HOME=/usr/local/rustup \
    PATH=/usr/local/cargo/bin:$PATH
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
        | CARGO_HOME=/usr/local/cargo \
          sh -s -- -y --default-toolchain stable --target wasm32-unknown-unknown \
    && chmod -R a+rx /usr/local/cargo /usr/local/rustup
# Bind-mounted /workspace is owned by the host user (uid != 0) but the
# container runs as a non-root UID, so git refuses to read .git under
# its "safe.directory" rule and `go build` then fails on VCS stamping
# with `error obtaining VCS status: exit status 128`. Whitelisting any
# path is fine here — the publisher only ever sees source we ourselves
# bind-mount in.
RUN git config --system --add safe.directory '*'
COPY scripts/dev-publish-entrypoint.sh /usr/local/bin/dev-publish-entrypoint.sh
RUN chmod +x /usr/local/bin/dev-publish-entrypoint.sh
# Run as the same UID as the distroless `nonroot` server (65532) so
# files written into the shared platypus_data volume are owned by the
# user that later reads / replaces them — no runtime chown step. Each
# mount target is pre-created with 65532 ownership so Docker
# initializes the matching named volumes with the right perms on
# first attach, regardless of whether the publisher or the server
# runs first.
ENV HOME=/home/publisher \
    GOCACHE=/cache/go-build \
    GOMODCACHE=/cache/go-mod \
    CARGO_HOME=/cache/cargo-home
# CARGO_TARGET_DIR is intentionally NOT set globally — each example
# plugin gets its own per-plugin target dir from the entrypoint
# (BUILD_BASE_DIR/<plugin>/). Setting it globally would force every
# plugin's outputs into a shared dir, causing cross-plugin name
# collisions and breaking the staging logic that reads
# <plugin_dir>/target/ relative paths.
RUN mkdir -p /home/publisher /keys /output \
        /cache/go-build /cache/go-mod /cache/cargo-home /cache/cargo-target \
    && chown -R 65532:65532 /home/publisher /keys /output \
        /cache/go-build /cache/go-mod /cache/cargo-home /cache/cargo-target
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/dev-publish-entrypoint.sh"]
