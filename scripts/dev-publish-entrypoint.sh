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
# caches mount at /cache/go-mod and /cache/go-build (GOMODCACHE +
# GOCACHE in the publisher image) to keep iteration fast (cold publish
# ~90 s on this matrix; warm publish ~10 s).
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

bash scripts/release-publish.sh

# Plugin marketplace bundle for dev mode.
#
# After agent binaries are signed + published, build the example wasm
# plugins, sign them with a per-volume dev publisher key, stage them
# under /output/plugin-marketplace/, and write an index.json the
# server can refresh from. The server picks up
#   <data-dir>/plugin-marketplace/index.json
# automatically when PLATYPUS_DEV=1 + PLATYPUS_PLUGIN_INDEX is unset
# (see cmd/platypus-server/main.go), so a fresh `docker compose up`
# lands a working marketplace with no extra setup.
#
# Skipped when SKIP_PLUGIN_MARKETPLACE=1 — useful when iterating on
# the agent layer and the marketplace is slow to rebuild.
if [[ "${SKIP_PLUGIN_MARKETPLACE:-0}" == "1" ]]; then
    echo "→ skipping plugin marketplace bundle (SKIP_PLUGIN_MARKETPLACE=1)"
    exit 0
fi

PLUGIN_KEY_SECRET="$KEYS_DIR/plugin-publisher.platypus"
PLUGIN_KEY_PUBLIC="$KEYS_DIR/plugin-publisher.pub"
MARKETPLACE_OUT="/output/plugin-marketplace"

# Build the platypus-cli we need for keygen + sign. The agent publish
# step above already cached most of the Go build; this is a couple of
# seconds.
echo "→ building platypus-cli"
go build -trimpath -ldflags="-s -w" -o /tmp/platypus-cli ./cmd/platypus-cli

if [[ ! -f "$PLUGIN_KEY_SECRET" || ! -f "$PLUGIN_KEY_PUBLIC" ]]; then
    echo "→ generating dev plugin signing keypair at $KEYS_DIR/"
    /tmp/platypus-cli plugin keygen \
        --out-secret "$PLUGIN_KEY_SECRET" \
        --out-public "$PLUGIN_KEY_PUBLIC" \
        --force
fi

echo "→ building + signing example plugins"
PLATYPUS_PUBLISHER_KEY="$PLUGIN_KEY_SECRET" \
    BUILD_DIR=/tmp \
    make example-plugins

# Stage the signed bundles. Mirror is one dir per (id, version) so
# index.json's wasm_url / signature_url / manifest_url can be a
# straight file:// URL pointing at the same files.
echo "→ staging plugin bundle under $MARKETPLACE_OUT"
rm -rf "$MARKETPLACE_OUT"
mkdir -p "$MARKETPLACE_OUT/plugins"
for d in example/plugins/*/; do
    d="${d%/}"
    if ! ls "$d"/target/wasm32-unknown-unknown/release/*.wasm >/dev/null 2>&1; then
        continue
    fi
    pid=$(awk '/^id:/ {print $2; exit}' "$d/plugin.yaml")
    ver=$(awk '/^version:/ {print $2; exit}' "$d/plugin.yaml")
    out="$MARKETPLACE_OUT/plugins/$pid/$ver"
    mkdir -p "$out"
    cp "$d/plugin.yaml" "$out/"
    cp "$d"/target/wasm32-unknown-unknown/release/*.wasm "$out/"
    cp "$d"/target/wasm32-unknown-unknown/release/*.wasm.minisig "$out/"
done

# Generate index.json. The server reads /app/data/plugin-marketplace/...
# (its <data-dir> is mapped to /app/data on the runtime side); the
# publisher writes /output/plugin-marketplace/... (its mount of the same
# volume). file:// URLs need to use the SERVER's path so we hard-code
# /app/data here. PLATYPUS_DEV=1 + the auto-detect branch in main.go
# turns the absent env var into a file:// URL pointing at this index.
echo "→ generating index.json"
SERVER_BUNDLE_PATH=/app/data/plugin-marketplace \
PUBKEY_FILE="$PLUGIN_KEY_PUBLIC" \
BUNDLE_ROOT="$MARKETPLACE_OUT" \
    python3 /workspace/scripts/dev-publish-marketplace-index.py \
    > "$MARKETPLACE_OUT/index.json"

# Drop the publisher .pub alongside so an admin curling the volume
# can spot-check + so a future "dev marketplace publisher" rotation
# tool has a known location.
cp "$PLUGIN_KEY_PUBLIC" "$MARKETPLACE_OUT/dev-publisher.pub"

echo "→ marketplace bundle ready ($(python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1]))["plugins"]))' "$MARKETPLACE_OUT/index.json") plugins)"
