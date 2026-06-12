//go:build !shared_lib
// +build !shared_lib

// Static-mode CGO link flags.
//
// This is the default build mode for llama-go. The Makefile copies the static
// archives (libbinding.a, libllama-common.a, libllama-common-base.a, libllama.a,
// libggml*.a) into the workspace root and the linker pulls all symbols into
// the consuming binary. No shared libraries are required at runtime, so the
// resulting Go binary is self-contained for the llama.cpp side.
//
// To opt into shared mode instead, build with: go build -tags shared_lib
// (and produce the workspace with: BUILD_LINKAGE=shared make libbinding.a).
//
// -Wl,--start-group/--end-group is used because libllama-common, libllama, and
// the various libggml-* archives have cross-references that the linker can't
// resolve in a single pass.

package llama

/*
#cgo CFLAGS: -I./ -I./llama.cpp -I./llama.cpp/include -I./llama.cpp/ggml/include -I./llama.cpp/common -I./llama.cpp/vendor -I./cgo_headers -I./cgo_headers/llama.cpp -I./cgo_headers/llama.cpp/include -I./cgo_headers/llama.cpp/ggml/include -I./cgo_headers/llama.cpp/common -I./cgo_headers/llama.cpp/thirdparty
#cgo CXXFLAGS: -I./ -I./llama.cpp -I./llama.cpp/include -I./llama.cpp/ggml/include -I./llama.cpp/common -I./llama.cpp/vendor -I./cgo_headers -I./cgo_headers/llama.cpp -I./cgo_headers/llama.cpp/include -I./cgo_headers/llama.cpp/ggml/include -I./cgo_headers/llama.cpp/common -I./cgo_headers/llama.cpp/thirdparty
#cgo LDFLAGS: -L./ -Wl,--start-group -lbinding -lllama-common -lllama-common-base -lllama -lggml-cpu -lggml -lggml-base -Wl,--end-group -lstdc++ -lm -lgomp
*/
import "C"
