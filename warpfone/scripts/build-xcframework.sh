#!/bin/bash
# Builds warpfone/Warpnet.xcframework (iOS, iOS Simulator, Mac Catalyst) from
# warpfone/node. Needs macOS with Xcode and the Go version of the root go.mod.

set -euo pipefail

export GOTOOLCHAIN=local

if ! command -v go >/dev/null 2>&1; then
    echo "Error: Go is not installed. See https://go.dev/dl/"
    exit 1
fi
if ! command -v xcrun >/dev/null 2>&1; then
    echo "Error: Xcode command line tools are not installed."
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WARPFONE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_DIR="$(cd "${WARPFONE_DIR}/.." && pwd)"

TOOLBIN="$(mktemp -d)"
trap 'rm -rf "${TOOLBIN}"' EXIT

cd "${REPO_DIR}"

echo "Building pinned gomobile/gobind..."
GOFLAGS="-mod=mod" go build -o "${TOOLBIN}/gomobile" golang.org/x/mobile/cmd/gomobile
GOFLAGS="-mod=mod" go build -o "${TOOLBIN}/gobind" golang.org/x/mobile/cmd/gobind
export PATH="${TOOLBIN}:${PATH}"

echo "Building Warpnet.xcframework..."
GOFLAGS="-mod=readonly -buildvcs=false" gomobile bind \
    -ldflags="-checklinkname=0 -s -w" \
    -trimpath \
    -tags mobile \
    -iosversion=17.0 \
    -target=ios,iossimulator,maccatalyst \
    -o "${WARPFONE_DIR}/Warpnet.xcframework" \
    ./warpfone/node

echo "Build complete: ${WARPFONE_DIR}/Warpnet.xcframework"
