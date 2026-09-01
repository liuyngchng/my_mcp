#!/bin/bash
# 交叉编译脚本
# 用法: ./build.sh [版本号]

set -e

VERSION="${1:-$(date +%Y%m%d)}"
APP_NAME="mcp-chat"
BUILD_DIR="build"

mkdir -p "$BUILD_DIR"

echo "=== 编译 $APP_NAME v$VERSION ==="

# Windows
echo "  -> Windows amd64"
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.version=$VERSION" -o "$BUILD_DIR/${APP_NAME}_${VERSION}_win_amd64.exe" .

# Windows arm64
echo "  -> Windows arm64"
GOOS=windows GOARCH=arm64 go build -ldflags="-s -w -X main.version=$VERSION" -o "$BUILD_DIR/${APP_NAME}_${VERSION}_win_arm64.exe" .

# Linux amd64
echo "  -> Linux amd64"
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$VERSION" -o "$BUILD_DIR/${APP_NAME}_${VERSION}_linux_amd64" .

# Linux arm64
echo "  -> Linux arm64"
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X main.version=$VERSION" -o "$BUILD_DIR/${APP_NAME}_${VERSION}_linux_arm64" .

# macOS amd64
echo "  -> macOS amd64"
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w -X main.version=$VERSION" -o "$BUILD_DIR/${APP_NAME}_${VERSION}_darwin_amd64" .

# macOS arm64
echo "  -> macOS arm64"
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w -X main.version=$VERSION" -o "$BUILD_DIR/${APP_NAME}_${VERSION}_darwin_arm64" .

echo ""
echo "=== 编译完成 ==="
echo "输出目录: $BUILD_DIR/"
ls -lh "$BUILD_DIR/"