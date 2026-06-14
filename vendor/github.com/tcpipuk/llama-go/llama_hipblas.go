//go:build hipblas
// +build hipblas

// This file provides ROCm/HIP GPU acceleration support when built with the
// 'hipblas' build tag. It links against AMD's ROCm libraries for GPU-accelerated
// inference on AMD GPUs.
//
// Build with: BUILD_TYPE=hipblas make libbinding.a
//
// Requires ROCm toolkit installed with hipBLAS and rocBLAS libraries. The ROCm
// compiler (hipcc) is required for proper linking.
//
// CGO flags required:
//
//	-O3 --hip-link --rtlib=compiler-rt -unwindlib=libgcc -lrocblas -lhipblas
package llama
