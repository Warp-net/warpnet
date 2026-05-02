#!/bin/bash
# Build script for WarpNet Android native library

set -e

echo "Building WarpNet native library for Android..."
if ! command -v go >/dev/null 2>&1; then
    echo "Error: Go is not installed."
    echo "Install Go from https://go.dev/dl/"
    exit 1
fi

echo "Installing gomobile..."
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest

echo "Initializing gomobile..."
gomobile init

echo "Building Android library..."
GOFLAGS=-mod=mod gomobile bind -ldflags="-checklinkname=0 -s -w" -trimpath -v -androidapi 21 -target=android/arm64 -o warpnet.aar .
rm -rf ../warpnet-transport/libs/warpnet
rm -f ../warpnet-transport/libs/warpnet.aar
mv warpnet.aar ../warpnet-transport/libs/warpnet.aar
mv warpnet-sources.jar ../warpnet-transport/libs/warpnet-sources.jar

echo "Build complete! Library created at warpnet-transport/libs/warpnet.aar"
