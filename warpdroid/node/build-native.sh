#!/bin/bash
# Build script for the WarpNet Android native library (gomobile).
#
# Produces warpnet.aar (+ warpnet-sources.jar) from the in-repo Go sources and
# installs them into ../warpnet-transport/libs/. F-Droid builds the native
# library from source with this script; the .aar is never committed.
#
# Requirements (provided by F-Droid's buildserver / CI):
#   - Go (see the monorepo go.mod `go` directive)
#   - Android NDK, located via $ANDROID_NDK_HOME (or $ANDROID_HOME/ndk/<ver>)
# gomobile/gobind are pinned by the monorepo go.mod (`tool` directives) and built
# with `-mod=mod` from the Go module cache/proxy (NOT from vendor/), so F-Droid
# does not need the vendored tree and there is no `go install @latest`.
#
# The `gomobile bind` step itself runs in module mode (`-mod=readonly`), NOT
# vendor mode: gomobile internally runs `go list -m all` / `go mod tidy`, which
# cannot operate against a vendor/ directory. readonly keeps the build pinned to
# go.sum (reproducible) and pulls the module sources from the Go module cache
# (populate it with `go mod download` when building without network).

set -euo pipefail

if ! command -v go >/dev/null 2>&1; then
    echo "Error: Go is not installed. See https://go.dev/dl/"
    exit 1
fi

# Resolve paths relative to this script so it works from any working directory.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIBS_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)/warpnet-transport/libs"

# Build the pinned gomobile + gobind (from the module cache/proxy, not vendor/)
# and put them on PATH so `gomobile bind` can locate gobind.
TOOLBIN="$(mktemp -d)"
trap 'rm -rf "${TOOLBIN}"' EXIT
echo "Building pinned gomobile/gobind..."
GOFLAGS="-mod=mod" go build -o "${TOOLBIN}/gomobile" golang.org/x/mobile/cmd/gomobile
GOFLAGS="-mod=mod" go build -o "${TOOLBIN}/gobind" golang.org/x/mobile/cmd/gobind
export PATH="${TOOLBIN}:${PATH}"

cd "${SCRIPT_DIR}"

# The app ships a single arm64-v8a APK, so build only the arm64 library
# (android/arm64 -> arm64-v8a).
echo "Building Android library (arm64-v8a)..."
GOFLAGS="-mod=readonly" gomobile bind \
    -ldflags="-checklinkname=0 -s -w" \
    -trimpath \
    -tags mobile \
    -v \
    -androidapi 21 \
    -target=android/arm64 \
    -o warpnet.aar .

mkdir -p "${LIBS_DIR}"
rm -rf "${LIBS_DIR}/warpnet"
rm -f "${LIBS_DIR}/warpnet.aar" "${LIBS_DIR}/warpnet-sources.jar"
mv warpnet.aar "${LIBS_DIR}/warpnet.aar"
mv warpnet-sources.jar "${LIBS_DIR}/warpnet-sources.jar"

echo "Build complete: ${LIBS_DIR}/warpnet.aar"
