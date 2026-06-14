//go:build cublas
// +build cublas

// This file provides CUDA/cuBLAS GPU acceleration support when built with the
// 'cublas' build tag. It links against NVIDIA's CUDA libraries for GPU-accelerated
// inference on NVIDIA GPUs.
//
// Build with: go build -tags cublas
//
// Requires CUDA toolkit installed with cuBLAS and CUDA runtime libraries.
//
// CGO LDFLAGS for this backend live in linkage-specific siblings:
//   - llama_cublas_static.go (built when -tags cublas, default linkage)
//   - llama_cublas_shared.go (built when -tags "cublas shared_lib")
package llama

/*
#cgo CPPFLAGS: -DGGML_USE_CUDA
*/
import "C"
