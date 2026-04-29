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
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        openssl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
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
    GOMODCACHE=/cache/go-mod
RUN mkdir -p /home/publisher /keys /output /cache/go-build /cache/go-mod \
    && chown -R 65532:65532 /home/publisher /keys /output /cache/go-build /cache/go-mod
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/dev-publish-entrypoint.sh"]
