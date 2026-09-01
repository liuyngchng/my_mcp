#!/bin/bash
# Cross-compile script for mcp-chat
# Usage: ./build.sh [version]
#
# Output: standalone exe/elf binaries with embedded web + mcp_servers.json.
# cfg.yml is external (placed alongside the binary at runtime).

set -e

VERSION="${1:-$(date +%Y%m%d)}"
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
    echo "  CGO_ENABLED=0 GOOS=${OS} GOARCH=${ARCH} go build \\"
    echo "    -ldflags=\"-s -w -X main.version=${VERSION}\" \\"
    echo "    -o ${OUT}"
    CGO_ENABLED=0 GOOS="${OS}" GOARCH="${ARCH}" go build \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o "${OUT}" .
    echo "  -> $(ls -lh "${OUT}" | awk '{print $5, $NF}')"
    echo ""
}

build_one windows amd64 .exe
build_one windows arm64 .exe
build_one linux   amd64 ""
build_one linux   arm64 ""
build_one darwin  amd64 ""
build_one darwin  arm64 ""

echo "============================================"
echo " Done. Output: $BUILD_DIR/"
echo "============================================"
ls -lh "$BUILD_DIR/"