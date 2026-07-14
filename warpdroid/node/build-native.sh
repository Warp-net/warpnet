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

# Pin the toolchain: never let GOTOOLCHAIN=auto silently upgrade to a newer Go than
# the one the build is supposed to use. F-Droid builds Go from the `go@goX.Y.Z`
# srclib with GOTOOLCHAIN=local; this makes the release CI behave identically so both
# use the SAME Go version (a version mismatch would make libgojni.so differ and break
# the reproducible-build comparison). The active `go` must satisfy go.mod's directive;
# if a dependency ever requires a newer Go, bump BOTH this build's Go and the recipe's
# `go@go...` srclib together.
export GOTOOLCHAIN=local

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
# Reproducible build:
#   -s -w                     drop the Go symbol table + DWARF
#   -buildid=                 clear the Go build ID (.note.go.buildid), a hash of
#                             build inputs that otherwise varies per environment
#   -extldflags=-Wl,--build-id=none  stop the NDK linker adding a .note.gnu.build-id
#   -trimpath                 trim dependency/stdlib source paths
#   GOFLAGS=-buildvcs=false   drop VCS stamping from .go.buildinfo
# NOTE: gomobile ignores -trimpath for the MAIN module, so it still embeds the
# checkout path in .go.buildinfo. That is handled by building at the same absolute
# path F-Droid uses (/home/vagrant/build/<appid>) — see release.yaml and
# https://f-droid.org/docs/Reproducible_Builds/ . Together with the app module's
# ndkVersion pin (so AGP strips the .so) this makes libgojni.so byte-identical
# across build environments.
GOFLAGS="-mod=readonly -buildvcs=false" gomobile bind \
    -ldflags="-checklinkname=0 -s -w -buildid= -extldflags=-Wl,--build-id=none" \
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
