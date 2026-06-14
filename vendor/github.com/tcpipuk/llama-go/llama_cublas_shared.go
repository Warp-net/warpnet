//go:build cublas && shared_lib
// +build cublas,shared_lib

// CUDA backend, shared linkage.
//
// libggml-cuda.so is shipped in the workspace root and links transitively to
// libcudart, libcublas, libcuda and libnccl via its NEEDED entries.
//
// We still list the system CUDA libraries explicitly, mirroring the static-mode
// flags, because our wrapper bakes a small ggml-cuda.o into libbinding.a (see
// EXTRA_TARGETS in the Makefile) and that object references cudart, cublas,
// driver-API and NCCL symbols directly. Modern ld fails with "DSO missing from
// command line" if those references aren't satisfied by an explicitly-listed
// library, regardless of what libggml-cuda.so's NEEDED entries provide.

package llama

/*
// /usr/local/cuda/lib64/stubs lets -lcuda resolve at link time when the build
// runs without GPU access (e.g. inside `docker build` without --gpus). The
// real libcuda.so arrives at runtime via nvidia-container-toolkit; the stub
// just satisfies the linker.
#cgo LDFLAGS: -lggml-cuda -L/usr/local/cuda/lib64 -L/usr/local/cuda/lib64/stubs -lcudart -lcublas -lcuda -lnccl
*/
import "C"
