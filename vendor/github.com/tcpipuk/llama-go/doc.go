// Package llama provides Go bindings for llama.cpp, enabling efficient LLM
// inference with GPU acceleration and advanced features like prefix caching
// and speculative decoding.
//
// This package wraps llama.cpp's C++ API whilst maintaining Go idioms and
// safety. Heavy computation stays in optimised C++ code, whilst the Go API
// provides clean concurrency primitives and resource management.
//
// # Quick Start
//
// Load a GGUF model and generate text:
//
//	model, err := llama.LoadModel("model.gguf")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer model.Close()
//
//	result, err := model.Generate("Once upon a time")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(result)
//
// # GPU Acceleration
//
// GPU offloading is enabled by default, automatically using CUDA, ROCm, or
// Metal depending on your build configuration. The library falls back to CPU
// if GPU resources are unavailable:
//
//	// Uses GPU by default (all layers offloaded)
//	model, err := llama.LoadModel("model.gguf")
//
//	// Limit GPU usage (useful for large models)
//	model, err := llama.LoadModel("model.gguf",
//	    llama.WithGPULayers(20),
//	)
//
//	// Force CPU-only inference
//	model, err := llama.LoadModel("model.gguf",
//	    llama.WithGPULayers(0),
//	)
//
// # Context Management
//
// The library automatically uses each model's native maximum context length
// from GGUF metadata, giving you full model capabilities without artificial
// limits:
//
//	// Uses model's native context (e.g. 40960 for Qwen3, 128000 for Gemma 3)
//	model, err := llama.LoadModel("model.gguf")
//
//	// Override for memory savings
//	model, err := llama.LoadModel("model.gguf",
//	    llama.WithContext(8192),
//	)
//
// # Concurrent Inference
//
// Models are thread-safe and support concurrent generation requests through
// an internal context pool:
//
//	var wg sync.WaitGroup
//	for i := 0; i < 10; i++ {
//	    wg.Add(1)
//	    go func(prompt string) {
//	        defer wg.Done()
//	        result, _ := model.Generate(prompt)
//	        fmt.Println(result)
//	    }(fmt.Sprintf("Question %d:", i))
//	}
//	wg.Wait()
//
// The pool automatically scales between minimum and maximum contexts based on
// demand, reusing contexts when possible and cleaning up idle ones.
//
// # Streaming Generation
//
// Stream tokens as they're generated using a callback:
//
//	err := model.GenerateStream("Tell me a story",
//	    func(token string) bool {
//	        fmt.Print(token)
//	        return true  // Continue generation
//	    },
//	)
//
// Return false from the callback to stop generation early.
//
// # Prefix Caching
//
// The library automatically reuses KV cache entries for matching prompt
// prefixes, significantly improving performance for conversation-style usage:
//
//	// First call processes full prompt
//	model.Generate("You are a helpful assistant.\n\nUser: Hello")
//
//	// Second call reuses cached system prompt
//	model.Generate("You are a helpful assistant.\n\nUser: How are you?")
//
// Prefix caching is enabled by default and includes a last-token refresh
// optimisation to maintain deterministic generation with minimal overhead
// (~0.1-0.5ms per call).
//
// # Speculative Decoding
//
// Accelerate generation using a smaller draft model:
//
//	target, _ := llama.LoadModel("large-model.gguf")
//	draft, _ := llama.LoadModel("small-model.gguf")
//	defer target.Close()
//	defer draft.Close()
//
//	result, err := target.GenerateWithDraft(
//	    "Once upon a time",
//	    draft,
//	    llama.WithDraftTokens(5),
//	)
//
// The draft model generates candidate tokens that the target model verifies
// in parallel, reducing overall latency whilst maintaining quality.
//
// # Advanced Configuration
//
// Fine-tune generation behaviour with sampling parameters:
//
//	result, err := model.Generate("Explain quantum computing",
//	    llama.WithMaxTokens(500),
//	    llama.WithTemperature(0.7),
//	    llama.WithTopP(0.9),
//	    llama.WithTopK(40),
//	    llama.WithSeed(42),
//	    llama.WithStopWords("</answer>", "\n\n"),
//	)
//
// # Thread Safety
//
// All public methods are thread-safe. The Model type uses an internal RWMutex
// to protect shared state and coordinates access to the context pool. Multiple
// goroutines can safely call Generate() concurrently.
//
// # Resource Cleanup
//
// Always call Close() when finished with a model to free GPU memory and other
// resources:
//
//	model, err := llama.LoadModel("model.gguf")
//	if err != nil {
//	    return err
//	}
//	defer model.Close()
//
// Close() is safe to call multiple times and will block until all active
// generation requests complete.
//
// # Build Requirements
//
// This package requires CGO and a C++ compiler. Pre-built llama.cpp libraries
// are included in the repository for convenience. See the project README for
// detailed build instructions and GPU acceleration setup.
package llama
