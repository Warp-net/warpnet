//go:build cublas && !shared_lib
// +build cublas,!shared_lib

// CUDA backend, static linkage.
//
// libggml-cuda.a is shipped in the workspace root and contains the CUDA-specific
// ggml kernels. Because it's a static archive, the linker also needs the CUDA
// runtime/driver libraries it depends on explicitly listed here — shared mode
// would get these transitively via libggml-cuda.so's NEEDED entries, but static
// archives don't carry that information.
//
// We link against the shared CUDA runtime/driver/NCCL (libcudart.so / libcublas.so /
// libcuda.so / libnccl.so), which the user must have installed on the system
// regardless of llama-go's link mode. The CUDA Driver API (libcuda.so, cu* prefix
// symbols) is part of the NVIDIA driver, separate from the toolkit's libcudart.

package llama

/*
// /usr/local/cuda/lib64/stubs lets -lcuda resolve at link time when the build
// runs without GPU access (e.g. inside `docker build` without --gpus). The
// real libcuda.so arrives at runtime via nvidia-container-toolkit; the stub
// just satisfies the linker.
#cgo LDFLAGS: -lggml-cuda -L/usr/local/cuda/lib64 -L/usr/local/cuda/lib64/stubs -lcudart -lcublas -lcuda -lnccl
*/
import "C"
