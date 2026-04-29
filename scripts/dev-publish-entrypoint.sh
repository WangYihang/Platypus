#!/usr/bin/env bash
#
# dev-publish-entrypoint.sh — `agent-publisher` compose sidecar entrypoint.
#
# Wraps scripts/release-publish.sh so a fresh `docker compose up` lands a
# signed manifest + agent binaries on the platypus-server's data volume
# at /app/data/releases/. Without this the enrollment flow renders but
# every install link 404s on the manifest fetch. Runs once per `up`;
# the platypus-server service waits on it completing successfully.
#
# Persistent state lives in /keys (a docker volume) so the dev signing
# keypair survives compose down/up — agents installed in earlier sessions
# keep validating manifests on upgrade. First invocation generates the
# pair; subsequent invocations reuse it.
#
# Source tree is bind-mounted read-only at /workspace; Go module + build
# caches mount at /go/pkg/mod and /root/.cache/go-build to keep iteration
# fast (cold publish ~90 s on this matrix; warm publish ~10 s).
set -euo pipefail

KEYS_DIR="/keys"
PRIVKEY="$KEYS_DIR/agent-signing.pem"
PUBKEY_B64="$KEYS_DIR/agent-signing.pub.b64"

mkdir -p "$KEYS_DIR"

if [[ ! -f "$PRIVKEY" || ! -f "$PUBKEY_B64" ]]; then
    echo "→ generating dev Ed25519 signing keypair at $KEYS_DIR/"
    openssl genpkey -algorithm ED25519 -out "$PRIVKEY"
    # Ed25519 SubjectPublicKeyInfo (DER) is a fixed 44-byte envelope; the
    # last 32 bytes are the raw public key — exactly what the agent's
    # SigningPublicKey ldflag expects (base64 of those 32 bytes).
    openssl pkey -in "$PRIVKEY" -pubout -outform DER \
        | tail -c 32 | base64 -w0 > "$PUBKEY_B64"
    chmod 0600 "$PRIVKEY"
fi

cd /workspace

export VERSION="${VERSION:-0.0.0-dev}"
export CHANNEL="${CHANNEL:-stable}"
export AGENT_SIGNING_PRIVKEY_PEM="$PRIVKEY"
export AGENT_SIGNING_PUBKEY_B64="$(cat "$PUBKEY_B64")"
# RELEASES_DIR points at the platypus-server's data volume mounted at
# /output. release-publish.sh writes the manifest + binaries straight
# under /output/releases/, exactly where the server's LocalStore reads
# from when serving /v1/manifest/<channel>.
export RELEASES_DIR="${RELEASES_DIR:-/output/releases}"

# Default matrix mirrors the platforms we've smoke-tested with
# CGO_ENABLED=0 (cmd/platypus-agent has only one platform-split file —
# sysinfo_machine_*.go — with an _other.go fallback). Override via the
# PLATFORMS env var if you want a leaner / different set.
export PLATFORMS="${PLATFORMS:-\
linux/amd64 linux/arm64 linux/arm linux/386 linux/riscv64 \
linux/ppc64le linux/s390x linux/loong64 \
linux/mips linux/mipsle linux/mips64 linux/mips64le \
darwin/amd64 darwin/arm64 \
windows/amd64 windows/arm64 windows/386 \
freebsd/amd64 freebsd/arm64 freebsd/386 \
openbsd/amd64 openbsd/arm64 \
netbsd/amd64 netbsd/arm64}"

mkdir -p "$RELEASES_DIR"

# The publisher runs as root but platypus-server runs as the distroless
# `nonroot` user (uid 65532) and needs to create platypus.db under
# /app/data (== this container's /output). Bind-mount source dirs are
# auto-created by Docker as root:root 0755, so without widening perms
# here the server fails on boot with `unable to open database file (14)`.
chmod 0777 /output "$RELEASES_DIR"

exec bash scripts/release-publish.sh
