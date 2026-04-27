#!/usr/bin/env bash
#
# release-publish.sh — build platypus-agent for the release matrix,
# assemble a signed manifest, and upload everything to the configured
# S3-compatible object store.
#
# Required environment variables:
#
#   VERSION                    release version (e.g. 1.6.0)
#   CHANNEL                    release channel (stable | beta | canary)
#   AGENT_SIGNING_PUBKEY_B64   base64 Ed25519 public key to bake into the agent
#   AGENT_SIGNING_PRIVKEY_PEM  path to the Ed25519 private key (PEM) used to sign the manifest
#   S3_ENDPOINT                S3 endpoint host (no scheme, e.g. s3.example.com)
#   S3_BUCKET                  target bucket
#   S3_PREFIX                  key prefix inside the bucket (e.g. "agent/")
#   S3_ACCESS_KEY              access key id
#   S3_SECRET_KEY              secret access key
#
# Optional:
#
#   S3_REGION                  defaults to us-east-1
#   S3_SCHEME                  "https" or "http"; defaults to "https"
#   PLATFORMS                  space-separated list of GOOS/GOARCH pairs
#                              (defaults to: linux/amd64 linux/arm64 windows/amd64 windows/arm64)

set -euo pipefail

: "${VERSION:?VERSION is required}"
: "${CHANNEL:?CHANNEL is required}"
: "${AGENT_SIGNING_PUBKEY_B64:?AGENT_SIGNING_PUBKEY_B64 is required}"
: "${AGENT_SIGNING_PRIVKEY_PEM:?AGENT_SIGNING_PRIVKEY_PEM is required}"
: "${S3_ENDPOINT:?S3_ENDPOINT is required}"
: "${S3_BUCKET:?S3_BUCKET is required}"
: "${S3_PREFIX:?S3_PREFIX is required}"
: "${S3_ACCESS_KEY:?S3_ACCESS_KEY is required}"
: "${S3_SECRET_KEY:?S3_SECRET_KEY is required}"

S3_REGION="${S3_REGION:-us-east-1}"
S3_SCHEME="${S3_SCHEME:-https}"
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

echo "→ uploading to s3://$S3_BUCKET/$S3_PREFIX"
mc alias set platypus-release "$S3_SCHEME://$S3_ENDPOINT" "$S3_ACCESS_KEY" "$S3_SECRET_KEY" --api s3v4 >/dev/null

# Upload artifacts first, manifest last — if the manifest lands before
# all artifacts, an agent racing the release would see a manifest that
# points at objects that do not exist yet.
for pair in $PLATFORMS; do
  GOOS="${pair%/*}"
  GOARCH="${pair#*/}"
  src="$WORKDIR/${GOOS}/${GOARCH}/platypus-agent"
  [[ "$GOOS" == "windows" ]] && src+=".exe"
  dst="platypus-release/$S3_BUCKET/${S3_PREFIX}artifacts/${VERSION}/${GOOS}/${GOARCH}/$(basename "$src")"
  mc cp "$src" "$dst"
done

mc cp "$MANIFEST"           "platypus-release/$S3_BUCKET/${S3_PREFIX}manifest/${CHANNEL}.json"
mc cp "$WORKDIR/manifest.sig" "platypus-release/$S3_BUCKET/${S3_PREFIX}manifest/${CHANNEL}.json.sig"

echo "✓ release $VERSION published to channel $CHANNEL"
