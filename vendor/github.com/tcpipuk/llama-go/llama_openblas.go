//go:build openblas
// +build openblas

// This file provides OpenBLAS CPU acceleration support when built with the
// 'openblas' build tag. It links against the OpenBLAS library for optimised
// CPU-based matrix operations, significantly improving inference performance
// on CPU-only systems.
//
// Build with: go build -tags openblas
//
// Requires OpenBLAS library installed on the system.
//
// Unlike CUDA, OpenBLAS is always linked against the system-shipped shared
// library (libopenblas.so) regardless of llama-go's BUILD_LINKAGE mode — there's
// no llama.cpp-built OpenBLAS archive to swap in. So no static/shared split here.
package llama

/*
#cgo LDFLAGS: -lopenblas
*/
import "C"
