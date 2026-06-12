//go:build opencl
// +build opencl

// This file provides OpenCL GPU acceleration support when built with the
// 'opencl' build tag. It links against OpenCL libraries for cross-platform
// GPU-accelerated inference on NVIDIA, AMD, Intel, ARM Mali, and Adreno GPUs.
//
// Build with: BUILD_TYPE=opencl make libbinding.a
//
// Requires OpenCL runtime and drivers installed. OpenCL provides broad GPU
// compatibility including older hardware and mobile devices, with support for
// FlashAttention and optimisations for Qualcomm Adreno GPUs.
//
// CGO flags required:
//
//	-lOpenCL
//	On macOS: -framework OpenCL
package llama
