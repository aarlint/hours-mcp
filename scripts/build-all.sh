#!/bin/bash

# Build script for Hours MCP - builds for all supported platforms
set -e

VERSION=${1:-"dev"}
OUTPUT_DIR="dist"

echo "🚀 Building Hours MCP v${VERSION} for all platforms..."

# Clean and create output directory
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

# Build targets
declare -a targets=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
)

for target in "${targets[@]}"; do
    IFS='/' read -r GOOS GOARCH <<< "$target"

    output_name="hours-mcp-${GOOS}-${GOARCH}"
    if [ "$GOOS" = "windows" ]; then
        output_name="${output_name}.exe"
    fi

    echo "📦 Building ${GOOS}/${GOARCH}..."

    env GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=1 \
        go build -ldflags="-s -w -X main.version=${VERSION}" \
        -o "${OUTPUT_DIR}/${output_name}" .

    echo "✅ Built ${output_name}"
done

# Create checksums
echo "🔐 Creating checksums..."
cd "${OUTPUT_DIR}"
sha256sum * > checksums.txt
cd ..

echo ""
echo "🎉 Build complete! Binaries available in ${OUTPUT_DIR}/"
echo ""
echo "📦 Built files:"
ls -la "${OUTPUT_DIR}/"

echo ""
echo "🔐 Checksums:"
cat "${OUTPUT_DIR}/checksums.txt"