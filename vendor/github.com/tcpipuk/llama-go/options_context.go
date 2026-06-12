package llama

import (
	"runtime"
)

// Context-level options
// (ContextOption type is defined in types.go)

// WithContext sets the context window size in tokens.
//
// The context size determines how many tokens (prompt + generation) the context
// can process. By default, the library uses the model's native maximum context
// length (e.g. 32768 for Qwen3, 128000 for Gemma 3 models >4B).
//
// Override this if you need to limit memory usage or have specific requirements.
//
// IMPORTANT: Very small context sizes (< 64 tokens) may cause llama.cpp to
// crash internally. The library provides defensive checks but cannot prevent
// all edge cases with absurdly small contexts.
//
// Default: 0 (uses model's native maximum from GGUF metadata)
//
// Examples:
//
//	// Use model's full capability (default)
//	ctx, err := model.NewContext()
//
//	// Limit to 8K for memory savings
//	ctx, err := model.NewContext(llama.WithContext(8192))
func WithContext(size int) ContextOption {
	return func(c *contextConfig) {
		c.contextSize = size
	}
}

// WithBatch sets the batch size for prompt processing.
//
// Larger batch sizes improve throughput for long prompts but increase memory
// usage. The batch size determines how many tokens are processed in parallel
// during the prompt evaluation phase.
//
// Default: 512
//
// Example:
//
//	// Process 1024 tokens at once for faster prompt handling
//	ctx, err := model.NewContext(llama.WithBatch(1024))
func WithBatch(size int) ContextOption {
	return func(c *contextConfig) {
		c.batchSize = size
	}
}

// WithUBatch sets the physical (micro) batch size used for a single forward pass.
//
// llama.cpp distinguishes between the logical batch (n_batch — how many tokens may
// queue before a flush) and the physical batch (n_ubatch — how many tokens are
// actually processed in one forward pass through the GPU). For most decoder models
// the default ubatch (which matches batch when unset) is fine.
//
// Encoder-only models (e.g. nomic-embed-text, BERT-family embedding models) require
// n_ubatch >= n_tokens for any single sequence — the entire input must fit in one
// forward pass. If you embed sequences longer than the default 512 tokens with such
// a model, set WithUBatch to at least your longest expected sequence length, or
// llama.cpp will abort with "encoder requires n_ubatch >= n_tokens".
//
// Default: 0 (matches batch size)
//
// Example:
//
//	// Embed long documents with a nomic encoder model.
//	ctx, err := model.NewContext(
//	    llama.WithEmbeddings(),
//	    llama.WithContext(8192),
//	    llama.WithBatch(8192),
//	    llama.WithUBatch(8192),
//	)
func WithUBatch(size int) ContextOption {
	return func(c *contextConfig) {
		c.uBatchSize = size
	}
}

// WithThreads sets the number of threads for token generation.
// If not specified, defaults to runtime.NumCPU().
// This also sets threadsBatch to the same value unless WithThreadsBatch is used.
func WithThreads(n int) ContextOption {
	return func(c *contextConfig) {
		c.threads = n
	}
}

// WithThreadsBatch sets the number of threads for batch/prompt processing.
// If not specified, defaults to the same value as threads.
// For most use cases, leaving this unset is recommended.
func WithThreadsBatch(n int) ContextOption {
	return func(c *contextConfig) {
		c.threadsBatch = n
	}
}

// WithF16Memory enables 16-bit floating point memory mode.
//
// When enabled, the context uses FP16 precision for KV cache storage, reducing
// memory usage at the cost of slight accuracy loss. Most useful when working
// with very long contexts or memory-constrained environments.
//
// Default: false (uses FP32 for KV cache)
//
// Example:
//
//	ctx, err := model.NewContext(llama.WithF16Memory())
func WithF16Memory() ContextOption {
	return func(c *contextConfig) {
		c.f16Memory = true
	}
}

// WithEmbeddings enables embedding extraction mode.
//
// When enabled, the context can compute text embeddings via GetEmbeddings().
// This mode is required for semantic search, clustering, or similarity tasks.
// Note that not all models support embeddings - check model documentation.
//
// Default: false (text generation mode)
//
// Example:
//
//	ctx, err := model.NewContext(llama.WithEmbeddings())
//	embeddings, err := ctx.GetEmbeddings("Hello world")
func WithEmbeddings() ContextOption {
	return func(c *contextConfig) {
		c.embeddings = true
	}
}

// WithKVCacheType sets the quantization type for KV cache storage.
//
// The KV (key-value) cache stores attention states during generation and grows
// with context length. Quantizing this cache dramatically reduces VRAM usage
// with minimal quality impact:
//
//   - "q8_0" (default): 50% VRAM savings, ~0.1% quality loss (imperceptible)
//   - "f16": Full precision, no savings, maximum quality
//   - "q4_0": 75% VRAM savings, noticeable quality loss (models become forgetful)
//
// Memory scaling example for 131K context (DeepSeek-R1 trained capacity):
//   - f16:  18 GB
//   - q8_0:  9 GB (recommended)
//   - q4_0:  4.5 GB (use only for extreme VRAM constraints)
//
// Default: "q8_0" (best balance of memory and quality)
//
// Examples:
//
//	// Use default Q8 quantization (recommended)
//	ctx, err := model.NewContext()
//
//	// Maximum quality for VRAM-rich systems
//	ctx, err := model.NewContext(llama.WithKVCacheType("f16"))
//
//	// Extreme memory savings (accept quality loss)
//	ctx, err := model.NewContext(llama.WithKVCacheType("q4_0"))
func WithKVCacheType(cacheType string) ContextOption {
	return func(c *contextConfig) {
		// Validate cache type
		switch cacheType {
		case "f16", "q8_0", "q4_0":
			c.kvCacheType = cacheType
		default:
			// Silently ignore invalid types and keep default
			// This prevents hard failures from typos while maintaining sensible behaviour
		}
	}
}

// WithFlashAttn controls Flash Attention kernel usage for attention computation.
//
// Flash Attention is a GPU-optimized attention implementation that significantly
// reduces VRAM usage and improves performance, especially for longer contexts.
// It's required when using quantized KV cache types (q8_0, q4_0).
//
// Available modes:
//   - "auto" (default): llama.cpp decides based on hardware and model config
//   - "enabled": Force Flash Attention on (fails if hardware doesn't support it)
//   - "disabled": Use traditional attention (incompatible with quantized KV cache)
//
// Technical details:
//   - Requires CUDA compute capability 7.0+ (Volta/Turing or newer)
//   - With GGML_CUDA_FA_ALL_QUANTS: Supports all KV cache quantization types
//   - Without flag: Only supports f16, q4_0, and q8_0 (matching K/V types)
//   - AUTO mode detects if backend scheduler supports the Flash Attention ops
//
// Default: "auto" (llama.cpp chooses optimal path)
//
// Examples:
//
//	// Use default auto-detection (recommended)
//	ctx, err := model.NewContext(llama.WithKVCacheType("q8_0"))
//
//	// Force Flash Attention on (errors if unsupported)
//	ctx, err := model.NewContext(llama.WithFlashAttn("enabled"))
//
//	// Disable Flash Attention (requires f16 KV cache)
//	ctx, err := model.NewContext(
//	    llama.WithKVCacheType("f16"),
//	    llama.WithFlashAttn("disabled"),
//	)
func WithFlashAttn(mode string) ContextOption {
	return func(c *contextConfig) {
		// Validate flash attention mode
		switch mode {
		case "auto", "enabled", "disabled":
			c.flashAttn = mode
		default:
			// Silently ignore invalid modes and keep default
			// This prevents hard failures from typos while maintaining sensible behaviour
		}
	}
}

// WithParallel sets the number of parallel sequences for batch processing.
//
// This option controls how many independent sequences can be processed
// simultaneously in a single batch. Higher values enable larger batch sizes
// for operations like GetEmbeddingsBatch() but consume more VRAM.
//
// For embedding contexts, the library defaults to n_parallel=8 if not explicitly
// set. This option allows tuning this value for your specific VRAM constraints
// and batch sizes.
//
// VRAM usage scales approximately as:
//
//	base_model_size + (n_parallel × context_size × kv_cache_bytes)
//
// For example, a 4B Q8 embedding model with 8192 context and q8_0 cache:
//   - n_parallel=8: ~12 GB VRAM
//   - n_parallel=4: ~8 GB VRAM
//   - n_parallel=2: ~6 GB VRAM
//   - n_parallel=1: ~5 GB VRAM (disables batch processing)
//
// Trade-offs:
//   - Lower values: Less VRAM usage, slower batch processing, smaller max batch size
//   - Higher values: More VRAM usage, faster batch processing, larger max batch size
//
// Default: 1 for generation contexts, 8 for embedding contexts (auto-set)
//
// Examples:
//
//	// Use default (8 for embeddings, 1 for generation)
//	ctx, err := model.NewContext(llama.WithEmbeddings())
//
//	// Tune down for large embedding model with limited VRAM
//	ctx, err := model.NewContext(
//	    llama.WithEmbeddings(),
//	    llama.WithParallel(4),
//	)
//
//	// Single sequence (minimal VRAM, no batching)
//	ctx, err := model.NewContext(
//	    llama.WithEmbeddings(),
//	    llama.WithParallel(1),
//	)
func WithParallel(n int) ContextOption {
	return func(c *contextConfig) {
		if n < 1 {
			n = 1
		}
		c.nParallel = n
	}
}

// WithPrefixCaching enables or disables KV cache prefix reuse.
//
// When enabled (default), the context automatically reuses cached KV entries
// for matching prompt prefixes, significantly improving performance for
// conversation-style usage where prompts share common beginnings.
//
// Default: true (enabled)
//
// Example:
//
//	// Disable prefix caching (not recommended for most use cases)
//	ctx, err := model.NewContext(llama.WithPrefixCaching(false))
func WithPrefixCaching(enabled bool) ContextOption {
	return func(c *contextConfig) {
		c.prefixCaching = enabled
	}
}

// Default values set in defaultContextConfig:
// - contextSize: 0 (use model's native max)
// - batchSize: 512
// - uBatchSize: 0 (matches batchSize)
// - threads: runtime.NumCPU()
// - threadsBatch: 0 (same as threads)
// - nParallel: 1 (8 for embeddings)
// - f16Memory: false
// - embeddings: false
// - prefixCaching: true
// - kvCacheType: "q8_0"
// - flashAttn: "auto"
func init() {
	// Ensure defaultContextConfig is initialized with correct defaults
	defaultContextConfig.threads = runtime.NumCPU()
}
