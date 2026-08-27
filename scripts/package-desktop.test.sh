#!/usr/bin/env bash
#
# package-desktop.test.sh — exercise every branch of
# package-desktop.sh against a fake build directory.
#
# The point is that this runs anywhere. The packaging logic used to be
# three inline blocks in a workflow, which meant the only way to find
# out whether the Windows branch worked was to push a tag and watch a
# release build fail after the compile. Faking the build output makes
# all three runnable on whatever machine is in front of you.
#
# The one thing not covered is the .dmg, which genuinely needs
# create-dmg on a Mac; the script skips it with a message and the
# macOS case here asserts the .zip that stands in for it.

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${REPO_ROOT}/scripts/package-desktop.sh"
VERSION="v9.9.9-test"

pass=0
fail=0
check() {
    local what="$1" cond="$2"
    if [ "$cond" = "yes" ]; then
        printf '  ok    %s\n' "$what"; pass=$((pass + 1))
    else
        printf '  FAIL  %s\n' "$what"; fail=$((fail + 1))
    fi
}
exists() { [ -e "$1" ] && echo yes || echo no; }

# Names come from wails.json — the test reads them the same way the
# script does, so a rename there does not need editing here either.
BINARY_NAME="$(jq -r .outputfilename "${REPO_ROOT}/desktop/wails.json")"
PRODUCT_NAME="$(jq -r .name "${REPO_ROOT}/desktop/wails.json")"
echo "using outputfilename=${BINARY_NAME} name=${PRODUCT_NAME} from wails.json"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# --- linux ------------------------------------------------------------
echo "linux:"
d="$TMP/linux"; mkdir -p "$d"
printf 'fake elf\n' > "$d/$BINARY_NAME"
out="$("$SCRIPT" --version "$VERSION" --os linux --bin-dir "$d" 2>/dev/null)"
tgz="$d/${PRODUCT_NAME}-${VERSION}-linux-amd64.tar.gz"
check "produces $(basename "$tgz")" "$(exists "$tgz")"
check "prints the artifact path" "$([ "$out" = "$tgz" ] && echo yes || echo no)"
check "tarball contains the binary under its wails.json name" \
    "$(tar -tzf "$tgz" 2>/dev/null | grep -qx "$BINARY_NAME" && echo yes || echo no)"

# --- windows ----------------------------------------------------------
echo "windows:"
d="$TMP/win"; mkdir -p "$d"
printf 'fake pe\n' > "$d/${BINARY_NAME}.exe"
printf 'fake nsis\n' > "$d/platypus-installer.exe"
out="$("$SCRIPT" --version "$VERSION" --os windows --bin-dir "$d" 2>/dev/null)"
exe="$d/${PRODUCT_NAME}-${VERSION}-windows-amd64.exe"
inst="$d/${PRODUCT_NAME}-${VERSION}-windows-amd64-installer.exe"
check "renames the binary" "$(exists "$exe")"
check "renames the NSIS installer" "$(exists "$inst")"
check "does not leave the original name behind" \
    "$([ ! -e "$d/${BINARY_NAME}.exe" ] && echo yes || echo no)"
check "prints both artifact paths" \
    "$([ "$(printf '%s\n' "$out" | wc -l)" -eq 2 ] && echo yes || echo no)"

# A plain (non-NSIS) build has no installer — that is not an error.
echo "windows, no installer:"
d="$TMP/win2"; mkdir -p "$d"
printf 'fake pe\n' > "$d/${BINARY_NAME}.exe"
"$SCRIPT" --version "$VERSION" --os windows --bin-dir "$d" >/dev/null 2>&1
check "still succeeds" "$([ $? -eq 0 ] && echo yes || echo no)"
check "renames the binary" "$(exists "$d/${PRODUCT_NAME}-${VERSION}-windows-amd64.exe")"

# The renamed binary must not be mistaken for the installer — both end
# in .exe and a naive glob picks up whichever sorts first.
check "does not rename the binary a second time as the installer" \
    "$([ ! -e "$d/${PRODUCT_NAME}-${VERSION}-windows-amd64-installer.exe" ] && echo yes || echo no)"

# --- darwin -----------------------------------------------------------
echo "darwin:"
d="$TMP/mac"; mkdir -p "$d/${PRODUCT_NAME}.app/Contents/MacOS"
printf 'fake macho\n' > "$d/${PRODUCT_NAME}.app/Contents/MacOS/${BINARY_NAME}"
out="$("$SCRIPT" --version "$VERSION" --os darwin --bin-dir "$d" 2>/dev/null)"
zipf="$d/${PRODUCT_NAME}-${VERSION}-macos-universal.zip"
check "produces the zip fallback" "$(exists "$zipf")"
check "zip contains the .app bundle" \
    "$(unzip -l "$zipf" 2>/dev/null | grep -q "${PRODUCT_NAME}.app/" && echo yes || echo no)"
check "prints the zip path" "$([ "$out" = "$zipf" ] && echo yes || echo no)"

# --- argument handling -----------------------------------------------
echo "arguments:"
"$SCRIPT" --os linux --bin-dir "$TMP/linux" >/dev/null 2>&1
check "--version is required" "$([ $? -ne 0 ] && echo yes || echo no)"
"$SCRIPT" --version "$VERSION" --os plan9 --bin-dir "$TMP/linux" >/dev/null 2>&1
check "rejects an unknown --os" "$([ $? -ne 0 ] && echo yes || echo no)"
"$SCRIPT" --version "$VERSION" --os linux --bin-dir "$TMP/does-not-exist" >/dev/null 2>&1
check "fails on a missing build directory" "$([ $? -ne 0 ] && echo yes || echo no)"
d="$TMP/empty"; mkdir -p "$d"
"$SCRIPT" --version "$VERSION" --os linux --bin-dir "$d" >/dev/null 2>&1
check "fails when the binary is absent" "$([ $? -ne 0 ] && echo yes || echo no)"

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
