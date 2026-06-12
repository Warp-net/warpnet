//go:build sycl
// +build sycl

// This file provides Intel oneAPI SYCL GPU acceleration support when built with
// the 'sycl' build tag. It links against Intel's oneAPI libraries for unified
// GPU programming supporting Intel Arc/Xe GPUs, with optional support for NVIDIA
// and AMD GPUs via SYCL backends.
//
// Build with: BUILD_TYPE=sycl make libbinding.a
//
// Requires Intel oneAPI toolkit installed. The SYCL backend provides a unified
// programming model across multiple GPU vendors, with primary support for Intel
// Arc and Xe GPUs. Set SYCL_TARGET environment variable to INTEL (default),
// NVIDIA, or AMD as needed.
//
// CGO flags required:
//
//	-lsycl -L/opt/intel/oneapi/compiler/latest/linux/lib
package llama
