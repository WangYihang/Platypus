# Stage 1: Build the binaries
FROM golang:1.25 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/platypus-server ./cmd/platypus-server

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/platypus-agent ./cmd/platypus-agent

# Stage 2: Server runtime
FROM gcr.io/distroless/static-debian12:nonroot AS server
WORKDIR /app
COPY --from=builder /out/platypus-server /usr/local/bin/platypus-server
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
# manifest with a dev Ed25519 key, and uploads everything to the
# compose-local MinIO. Wired into docker-compose.yml as the
# `agent-publisher` sidecar so a fresh `docker compose up` results in a
# working enrollment flow without manual `make` steps. Not used in
# production — production releases run scripts/release-publish.sh from
# CI with a vault-stored signing key.
FROM golang:1.25 AS publisher
WORKDIR /workspace
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        openssl ca-certificates curl \
    && ARCH="$(dpkg --print-architecture)" \
    && case "$ARCH" in \
        amd64) MC_ARCH="linux-amd64" ;; \
        arm64) MC_ARCH="linux-arm64" ;; \
        *) echo "unsupported build arch: $ARCH" >&2; exit 1 ;; \
       esac \
    && curl -fsSL "https://dl.min.io/client/mc/release/${MC_ARCH}/mc" \
        -o /usr/local/bin/mc \
    && chmod +x /usr/local/bin/mc \
    && rm -rf /var/lib/apt/lists/*
COPY scripts/dev-publish-entrypoint.sh /usr/local/bin/dev-publish-entrypoint.sh
RUN chmod +x /usr/local/bin/dev-publish-entrypoint.sh
ENTRYPOINT ["/usr/local/bin/dev-publish-entrypoint.sh"]
