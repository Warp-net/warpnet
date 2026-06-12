//go:build shared_lib
// +build shared_lib

// Shared-mode CGO link flags.
//
// Selected by building with: go build -tags shared_lib (and producing the
// workspace with: BUILD_LINKAGE=shared make libbinding.a).
//
// In this mode the Makefile copies .so files into the workspace root and the
// resulting binary depends on them at runtime. The -Wl,-rpath,$ORIGIN flag
// bakes a relative-to-binary search path into the binary so the .so files only
// need to sit alongside it (no LD_LIBRARY_PATH required).

package llama

/*
#cgo CFLAGS: -I./ -I./llama.cpp -I./llama.cpp/include -I./llama.cpp/ggml/include -I./llama.cpp/common -I./llama.cpp/vendor -I./cgo_headers -I./cgo_headers/llama.cpp -I./cgo_headers/llama.cpp/include -I./cgo_headers/llama.cpp/ggml/include -I./cgo_headers/llama.cpp/common -I./cgo_headers/llama.cpp/thirdparty
#cgo CXXFLAGS: -I./ -I./llama.cpp -I./llama.cpp/include -I./llama.cpp/ggml/include -I./llama.cpp/common -I./llama.cpp/vendor -I./cgo_headers -I./cgo_headers/llama.cpp -I./cgo_headers/llama.cpp/include -I./cgo_headers/llama.cpp/ggml/include -I./cgo_headers/llama.cpp/common -I./cgo_headers/llama.cpp/thirdparty
#cgo LDFLAGS: -L./ -Wl,-rpath,$ORIGIN -lbinding -lllama-common -lllama -lggml -lggml-cpu -lggml-base -lstdc++ -lm -lgomp
*/
import "C"
