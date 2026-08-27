#!/usr/bin/env bash
#
# package-desktop.sh — turn what `wails build` left in
# desktop/build/bin into the release artifact for one platform.
#
# This used to live as three inline `run:` blocks in
# .github/workflows/desktop.yaml. Two problems with that:
#
#   1. The product name was written out six times across those blocks
#      and once more in desktop/wails.json. Renaming the binary meant
#      finding all seven, and getting it wrong shows up only when a
#      tag build fails at the packaging step — after the compile has
#      already run on three OSes.
#
#   2. None of it could be run outside CI. A workflow `run:` block is
#      only exercised by pushing a tag, so a typo in the Windows
#      branch waits for a release to announce itself.
#
# The name is read from wails.json here, so that file is the single
# source. And every branch runs anywhere: the linux and windows paths
# need only tar and mv, and the macOS path produces its .zip on any
# host — only the .dmg genuinely needs create-dmg, which the script
# skips (loudly) when it is missing. scripts/package-desktop.test.sh
# exercises all three on whatever machine you have.
#
# Usage:
#   scripts/package-desktop.sh --version v1.6.0 --os linux
#   scripts/package-desktop.sh --version v1.6.0 --os windows --bin-dir /tmp/fake
#
#   --version   version string embedded in the artifact name (required)
#   --os        linux | windows | darwin (default: the host)
#   --arch      arch string in the artifact name (default: amd64;
#               darwin ignores it and uses "universal")
#   --bin-dir   where the build output is (default: desktop/build/bin)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WAILS_JSON="${REPO_ROOT}/desktop/wails.json"

VERSION=""
TARGET_OS=""
TARGET_ARCH="amd64"
BIN_DIR=""

while [ $# -gt 0 ]; do
    case "$1" in
        --version) VERSION="${2:-}"; shift 2 ;;
        --os)      TARGET_OS="${2:-}"; shift 2 ;;
        --arch)    TARGET_ARCH="${2:-}"; shift 2 ;;
        --bin-dir) BIN_DIR="${2:-}"; shift 2 ;;
        -h|--help) sed -n '2,40p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) echo "package-desktop: unknown argument: $1" >&2; exit 2 ;;
    esac
done

if [ -z "$VERSION" ]; then
    echo "package-desktop: --version is required" >&2
    exit 2
fi

if [ -z "$TARGET_OS" ]; then
    case "$(uname -s)" in
        Linux)   TARGET_OS="linux" ;;
        Darwin)  TARGET_OS="darwin" ;;
        MINGW*|MSYS*|CYGWIN*) TARGET_OS="windows" ;;
        *) echo "package-desktop: cannot infer --os from $(uname -s)" >&2; exit 2 ;;
    esac
fi

[ -n "$BIN_DIR" ] || BIN_DIR="${REPO_ROOT}/desktop/build/bin"

# --- the single source for both names --------------------------------
# `outputfilename` is what wails writes; `name` is what the project
# calls itself and what the artifacts are named after. Read rather than
# repeated, so a rename in wails.json carries here on its own.
if [ ! -f "$WAILS_JSON" ]; then
    echo "package-desktop: $WAILS_JSON not found" >&2
    exit 1
fi
read_wails_key() {
    # jq when it is available; a sed fallback so this does not depend
    # on it (git-bash on the Windows runner is the case that bites).
    if command -v jq >/dev/null 2>&1; then
        jq -r --arg k "$1" '.[$k] // empty' "$WAILS_JSON"
    else
        sed -n "s/^[[:space:]]*\"$1\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$WAILS_JSON" | head -1
    fi
}
BINARY_NAME="$(read_wails_key outputfilename)"
PRODUCT_NAME="$(read_wails_key name)"

if [ -z "$BINARY_NAME" ] || [ -z "$PRODUCT_NAME" ]; then
    echo "package-desktop: could not read outputfilename/name from $WAILS_JSON" >&2
    exit 1
fi

if [ ! -d "$BIN_DIR" ]; then
    echo "package-desktop: build output directory $BIN_DIR does not exist" >&2
    exit 1
fi

cd "$BIN_DIR"

case "$TARGET_OS" in
    linux)
        SRC="$BINARY_NAME"
        OUT="${PRODUCT_NAME}-${VERSION}-linux-${TARGET_ARCH}.tar.gz"
        [ -f "$SRC" ] || { echo "package-desktop: $BIN_DIR/$SRC not found — did wails build run?" >&2; exit 1; }
        tar -czf "$OUT" "$SRC"
        echo "$BIN_DIR/$OUT"
        ;;

    windows)
        SRC="${BINARY_NAME}.exe"
        OUT="${PRODUCT_NAME}-${VERSION}-windows-${TARGET_ARCH}.exe"
        [ -f "$SRC" ] || { echo "package-desktop: $BIN_DIR/$SRC not found — did wails build run?" >&2; exit 1; }
        mv "$SRC" "$OUT"
        echo "$BIN_DIR/$OUT"

        # The NSIS installer is optional: `wails build -nsis` produces
        # it, a plain build does not.
        installer="$(find . -maxdepth 1 -name '*installer.exe' ! -name "$OUT" -print -quit)"
        if [ -n "$installer" ]; then
            INSTALLER_OUT="${PRODUCT_NAME}-${VERSION}-windows-${TARGET_ARCH}-installer.exe"
            mv "$installer" "$INSTALLER_OUT"
            echo "$BIN_DIR/$INSTALLER_OUT"
        else
            echo "package-desktop: no NSIS installer found, skipping" >&2
        fi
        ;;

    darwin)
        APP="$(find . -maxdepth 1 -name '*.app' -print -quit)"
        [ -n "$APP" ] || { echo "package-desktop: no .app bundle in $BIN_DIR — did wails build run?" >&2; exit 1; }
        APP="${APP#./}"

        # The zip is the fallback artifact and needs nothing but zip,
        # so it is produced first and on any host.
        ZIP_OUT="${PRODUCT_NAME}-${VERSION}-macos-universal.zip"
        zip -qr "$ZIP_OUT" "$APP"
        echo "$BIN_DIR/$ZIP_OUT"

        DMG_OUT="${PRODUCT_NAME}-${VERSION}-macos-universal.dmg"
        if command -v create-dmg >/dev/null 2>&1; then
            create-dmg \
                --volname "Platypus Desktop" \
                --window-pos 200 120 \
                --window-size 600 300 \
                --icon-size 100 \
                --icon "$APP" 150 150 \
                --hide-extension "$APP" \
                --app-drop-link 450 150 \
                "$DMG_OUT" \
                "$APP"
            echo "$BIN_DIR/$DMG_OUT"
        else
            # Not fatal: this is the one step that genuinely needs a
            # Mac, and saying so beats failing a local rehearsal.
            echo "package-desktop: create-dmg not installed, skipping $DMG_OUT" >&2
        fi
        ;;

    *)
        echo "package-desktop: unsupported --os: $TARGET_OS" >&2
        exit 2
        ;;
esac
