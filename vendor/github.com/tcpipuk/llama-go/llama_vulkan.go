//go:build vulkan
// +build vulkan

// This file provides Vulkan GPU acceleration support when built with the
// 'vulkan' build tag. It links against the Vulkan API for cross-platform
// GPU-accelerated inference on NVIDIA, AMD, Intel, and ARM GPUs.
//
// Build with: BUILD_TYPE=vulkan make libbinding.a
//
// Requires Vulkan SDK installed with compatible GPU drivers. Vulkan provides
// a unified backend avoiding vendor-specific code whilst supporting modern GPU
// features including cooperative matrices and tensor cores.
//
// CGO flags required:
//
//	-lvulkan -L/usr/lib/x86_64-linux-gnu
package llama
