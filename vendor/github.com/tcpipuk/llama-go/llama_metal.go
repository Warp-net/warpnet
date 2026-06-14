//go:build metal
// +build metal

// This file provides Metal GPU acceleration support when built with the 'metal'
// build tag. It links against Apple's Metal frameworks for GPU-accelerated
// inference on Apple Silicon (M-series) Macs.
//
// Build with: BUILD_TYPE=metal make libbinding.a
//
// Requires macOS with Metal support. The build process creates a ggml-metal.metal
// shader file that must be distributed alongside the application binary.
//
// CGO flags required:
//
//	-framework Foundation -framework Metal -framework MetalKit
//	-framework MetalPerformanceShaders
package llama
