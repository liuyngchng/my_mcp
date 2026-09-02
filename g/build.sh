#!/bin/bash
# Linux build script for mcp-chat
# Usage: ./build.sh [version] [--windows]
#
#   version   - version tag, defaults to YYYYMMDD
#   --windows - also cross-compile Windows .exe (amd64)
#
# Output: standalone elf/exe binaries with embedded web + mcp_servers.json.
# cfg.yml is external (placed alongside the binary at runtime).

set -e

# garble 路径
GARBLE="$HOME/go/bin/garble"
if [ ! -x "$GARBLE" ]; then
    echo "ERROR: garble not found at $GARBLE"
    echo "Install with: go install mvdan.cc/garble@latest"
    exit 1
fi

CROSS_WINDOWS=false
VERSION=""

for arg in "$@"; do
    case "$arg" in
        --windows) CROSS_WINDOWS=true ;;
        *) VERSION="$arg" ;;
    esac
done

VERSION="${VERSION:-$(date +%Y%m%d)}"
APP_NAME="mcp-chat"
BUILD_DIR="build"

mkdir -p "$BUILD_DIR"

echo "============================================"
echo " Building $APP_NAME v$VERSION"
echo " Host: $(go env GOHOSTOS)/$(go env GOHOSTARCH)"
echo " Go:   $(go version)"
echo "============================================"
echo ""

# build_one <os> <arch> <suffix>
build_one() {
    local OS="$1"
    local ARCH="$2"
    local SUFFIX="$3"
    local OUT="${BUILD_DIR}/${APP_NAME}_${VERSION}_${OS}_${ARCH}${SUFFIX}"

    echo "--- [$OS/$ARCH] ---"
    echo "  CGO_ENABLED=0 GOOS=${OS} GOARCH=${ARCH} garble -literals -tiny build \\"
    echo "    -ldflags=\"-s -w -X main.version=${VERSION}\" \\"
    echo "    -o ${OUT}"
    CGO_ENABLED=0 GOOS="${OS}" GOARCH="${ARCH}" "$GARBLE" -literals -tiny build \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o "${OUT}" .
    echo "  -> $(ls -lh "${OUT}" | awk '{print $5, $NF}')"
    echo ""
}

# Always build Linux native (amd64 only)
build_one linux amd64 ""

# Cross-compile Windows on demand
if $CROSS_WINDOWS; then
    build_one windows amd64 .exe
fi

echo "============================================"
echo " Done. Output: $BUILD_DIR/"
echo "============================================"
ls -lh "$BUILD_DIR/"
if ! $CROSS_WINDOWS; then
    echo ""
    echo "💡 加上 --windows 可交叉编译 Windows .exe:"
    echo "   ./build.sh --windows"
fi