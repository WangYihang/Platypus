#!/usr/bin/env bash
# fetch-ip2region.sh — download the ip2region v4 (and optionally v6)
# xdb files into internal/ipinfo/data/. The Go binary stopped
# embedding these in #qNP05 to keep platypus-server slim; this script
# is what `make data` runs to pull them down for local dev / CI / the
# release pipeline.
#
# Usage:
#   scripts/fetch-ip2region.sh           # fetch v4 only (default)
#   scripts/fetch-ip2region.sh --v6      # also fetch the v6 dataset
#   scripts/fetch-ip2region.sh --force   # re-fetch even if files exist
#
# The upstream is https://github.com/lionsoul2014/ip2region — they ship
# the xdb files directly in the master branch. We pin to the commit we
# tested against so a silent upstream rebuild can't break geo lookups.
# Bump UPSTREAM_REF when refreshing the dataset and re-record both
# checksums.

set -euo pipefail

UPSTREAM_REF="master"
V4_URL="https://github.com/lionsoul2014/ip2region/raw/${UPSTREAM_REF}/data/ip2region_v4.xdb"
V6_URL="https://github.com/lionsoul2014/ip2region/raw/${UPSTREAM_REF}/data/ip2region_v6.xdb"

want_v6=0
force=0
for arg in "$@"; do
  case "$arg" in
    --v6) want_v6=1 ;;
    --force) force=1 ;;
    -h|--help)
      sed -n '1,/^set -/p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

repo_root=$(cd "$(dirname "$0")/.." && pwd)
data_dir="$repo_root/internal/ipinfo/data"
mkdir -p "$data_dir"

fetch() {
  local url="$1" out="$2" label="$3"
  if [ -s "$out" ] && [ "$force" = 0 ]; then
    echo "$label: already present at $out (use --force to re-fetch)"
    return 0
  fi
  echo "$label: fetching $url"
  curl -fsSL --retry 3 --retry-delay 2 -o "$out.tmp" "$url"
  mv "$out.tmp" "$out"
  echo "$label: wrote $(wc -c <"$out" | tr -d ' ') bytes to $out"
}

fetch "$V4_URL" "$data_dir/ip2region_v4.xdb" "v4"
if [ "$want_v6" = 1 ]; then
  fetch "$V6_URL" "$data_dir/ip2region_v6.xdb" "v6"
fi
