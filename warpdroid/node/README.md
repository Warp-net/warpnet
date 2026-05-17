# WarpNet Android Native Client

This directory contains the Go implementation of the libp2p client for WarpNet Android.

## Overview

The WarpNet client is built using Go and libp2p, compiled to a native Android library (.aar) using gomobile.

## Structure

- `client.go` - Core libp2p client implementation
- `mobile.go` - Gomobile-compatible wrapper for JNI binding
- `go.mod` - Go module dependencies

## Building

### Prerequisites

1. Go 1.21 or higher
2. gomobile tool
3. Android SDK and NDK

### Setup gomobile

```bash
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init
```

### Build for Android

```bash
# From this directory
gomobile bind -target=android -o ../../app/libs/warpnet.aar .
```

This generates `warpnet.aar` which can be included in the Android project.

### Build for specific architectures

```bash
# For specific ABIs
gomobile bind -target=android/arm64,android/amd64 -o ../../app/libs/warpnet.aar .
```

## Testing

Run Go tests:

```bash
go test -v
```

## Dependencies

Main dependencies are declared in `go.mod`:

- `github.com/libp2p/go-libp2p` - Core libp2p implementation
- `github.com/libp2p/go-libp2p-pnet` - Private network support
- `github.com/multiformats/go-multiaddr` - Multiaddr support
