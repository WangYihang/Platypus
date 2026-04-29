#!/usr/bin/env bash
#
# release-publish.sh — build platypus-agent for the release matrix,
# assemble a signed manifest, and lay the result out on disk under
# RELEASES_DIR in the layout the server's LocalStore expects:
#
#   $RELEASES_DIR/manifest/<channel>.json
#   $RELEASES_DIR/manifest/<channel>.json.sig
#   $RELEASES_DIR/artifacts/<version>/<os>/<arch>/platypus-agent[.exe]
#
# Operators rsync the resulting tree onto the server's data volume
# (or mount the same directory directly in dev / single-host setups);
# the server picks it up on the next manifest fetch — no restart
# required.
#
# Required environment variables:
#
#   VERSION                    release version (e.g. 1.6.0)
#   CHANNEL                    release channel (stable | beta | canary)
#   RELEASES_DIR               output root; layout above is created beneath it
#   AGENT_SIGNING_PUBKEY_B64   base64 Ed25519 public key to bake into the agent
#   AGENT_SIGNING_PRIVKEY_PEM  path to the Ed25519 private key (PEM) used to sign the manifest
#
# Optional:
#
#   PLATFORMS                  space-separated list of GOOS/GOARCH pairs
#                              (defaults to: linux/amd64 linux/arm64 windows/amd64 windows/arm64)

set -euo pipefail

: "${VERSION:?VERSION is required}"
: "${CHANNEL:?CHANNEL is required}"
: "${RELEASES_DIR:?RELEASES_DIR is required}"
: "${AGENT_SIGNING_PUBKEY_B64:?AGENT_SIGNING_PUBKEY_B64 is required}"
: "${AGENT_SIGNING_PRIVKEY_PEM:?AGENT_SIGNING_PRIVKEY_PEM is required}"

PLATFORMS="${PLATFORMS:-linux/amd64 linux/arm64 windows/amd64 windows/arm64}"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

ARTIFACTS_JSON=""
for pair in $PLATFORMS; do
  GOOS="${pair%/*}"
  GOARCH="${pair#*/}"
  out="$WORKDIR/${GOOS}/${GOARCH}/platypus-agent"
  [[ "$GOOS" == "windows" ]] && out+=".exe"
  mkdir -p "$(dirname "$out")"

  echo "→ building $GOOS/$GOARCH"
  # -buildvcs=false: the dev compose sidecar bind-mounts the source
  # tree as root inside the container, but the host .git is owned by
  # uid 1000, which makes git's safe.directory check refuse and Go
  # then surfaces it as "error obtaining VCS status: exit status 128".
  # The release artifact's identity already comes from VERSION + the
  # signed manifest; embedded git metadata is redundant.
  GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED=0 \
    go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X github.com/WangYihang/Platypus/internal/agent.SigningPublicKey=${AGENT_SIGNING_PUBKEY_B64}" \
    -o "$out" ./cmd/platypus-agent

  size=$(wc -c <"$out" | tr -d ' ')
  sha=$(sha256sum "$out" | awk '{print $1}')
  key="artifacts/${VERSION}/${GOOS}/${GOARCH}/$(basename "$out")"

  entry=$(printf '{"os":"%s","arch":"%s","key":"%s","size":%s,"sha256":"%s"}' \
    "$GOOS" "$GOARCH" "$key" "$size" "$sha")
  if [[ -z "$ARTIFACTS_JSON" ]]; then
    ARTIFACTS_JSON="$entry"
  else
    ARTIFACTS_JSON="${ARTIFACTS_JSON},${entry}"
  fi
done

RELEASED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
MANIFEST="$WORKDIR/manifest.json"
printf '{"version":"%s","channel":"%s","released_at":"%s","artifacts":[%s]}' \
  "$VERSION" "$CHANNEL" "$RELEASED_AT" "$ARTIFACTS_JSON" >"$MANIFEST"

echo "→ signing manifest"
openssl pkeyutl -sign \
  -inkey "$AGENT_SIGNING_PRIVKEY_PEM" \
  -rawin -in "$MANIFEST" \
  -out "$WORKDIR/manifest.sig"

echo "→ laying out release tree under $RELEASES_DIR"

# Place artifacts before the manifest so an agent racing the
# release never reads a manifest that points at files that don't
# exist yet. Same invariant the old S3 path enforced via upload
# order.
for pair in $PLATFORMS; do
  GOOS="${pair%/*}"
  GOARCH="${pair#*/}"
  src="$WORKDIR/${GOOS}/${GOARCH}/platypus-agent"
  [[ "$GOOS" == "windows" ]] && src+=".exe"
  dst="$RELEASES_DIR/artifacts/${VERSION}/${GOOS}/${GOARCH}/$(basename "$src")"
  mkdir -p "$(dirname "$dst")"
  cp "$src" "$dst"
done

mkdir -p "$RELEASES_DIR/manifest"
cp "$MANIFEST" "$RELEASES_DIR/manifest/${CHANNEL}.json"
cp "$WORKDIR/manifest.sig" "$RELEASES_DIR/manifest/${CHANNEL}.json.sig"

echo "✓ release $VERSION published to channel $CHANNEL under $RELEASES_DIR"
