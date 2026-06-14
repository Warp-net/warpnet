package llama

import (
	"runtime"
)

// contextConfig holds configuration for context creation
type contextConfig struct {
	contextSize   int
	batchSize     int
	uBatchSize    int // Physical micro-batch size; 0 means match batchSize
	threads       int
	threadsBatch  int
	nParallel     int // Number of parallel sequences (for batch embeddings)
	f16Memory     bool
	embeddings    bool
	prefixCaching bool   // Enable KV cache prefix reuse (default: true)
	kvCacheType   string // KV cache quantization type: "f16", "q8_0", "q4_0" (default: "q8_0")
	flashAttn     string // Flash Attention mode: "auto", "enabled", "disabled" (default: "auto")
}

// generateConfig holds configuration for text generation
type generateConfig struct {
	// Basic generation
	maxTokens   int
	temperature float32
	seed        int
	stopWords   []string
	draftTokens int
	debug       bool

	// Basic sampling parameters
	topK      int
	topP      float32
	minP      float32
	typP      float32
	topNSigma float32
	minKeep   int

	// Repetition penalties
	penaltyLastN   int
	penaltyRepeat  float32
	penaltyFreq    float32
	penaltyPresent float32

	// DRY (Don't Repeat Yourself) sampling
	dryMultiplier       float32
	dryBase             float32
	dryAllowedLength    int
	dryPenaltyLastN     int
	drySequenceBreakers []string

	// Dynamic temperature
	dynatempRange    float32
	dynatempExponent float32

	// XTC (eXclude Top Choices) sampling
	xtcProbability float32
	xtcThreshold   float32

	// Mirostat sampling
	mirostat    int
	mirostatTau float32
	mirostatEta float32

	// Other parameters
	nPrev     int
	nProbs    int
	ignoreEOS bool
}

// Default context configuration
var defaultContextConfig = contextConfig{
	contextSize:   0, // 0 = use model's native maximum (queried after load)
	batchSize:     512,
	uBatchSize:    0, // 0 = match batchSize (set by wrapper)
	threads:       runtime.NumCPU(),
	threadsBatch:  0, // 0 means use same as threads (set in wrapper)
	nParallel:     1, // 1 for generation, auto-set higher for embeddings
	f16Memory:     false,
	embeddings:    false,
	prefixCaching: true,   // Enable by default for performance
	kvCacheType:   "q8_0", // 50% VRAM savings with ~0.1% quality loss
	flashAttn:     "auto", // Let llama.cpp choose optimal path
}

var defaultGenerateConfig = generateConfig{
	// Basic generation
	maxTokens:   128,
	temperature: 0.8,
	seed:        -1,
	draftTokens: 16,
	debug:       false,

	// Basic sampling parameters
	topK:      40,
	topP:      0.95,
	minP:      0.05,
	typP:      1.0,  // 1.0 = disabled
	topNSigma: -1.0, // -1.0 = disabled
	minKeep:   0,

	// Repetition penalties
	penaltyLastN:   64,
	penaltyRepeat:  1.0, // 1.0 = disabled
	penaltyFreq:    0.0, // 0.0 = disabled
	penaltyPresent: 0.0, // 0.0 = disabled

	// DRY sampling
	dryMultiplier:       0.0, // 0.0 = disabled
	dryBase:             1.75,
	dryAllowedLength:    2,
	dryPenaltyLastN:     -1, // -1 = context size
	drySequenceBreakers: []string{"\n", ":", "\"", "*"},

	// Dynamic temperature
	dynatempRange:    0.0, // 0.0 = disabled
	dynatempExponent: 1.0,

	// XTC sampling
	xtcProbability: 0.0, // 0.0 = disabled
	xtcThreshold:   0.1,

	// Mirostat sampling
	mirostat:    0, // 0 = disabled
	mirostatTau: 5.0,
	mirostatEta: 0.1,

	// Other parameters
	nPrev:     64,
	nProbs:    0, // 0 = disabled
	ignoreEOS: false,
}

// modelConfig holds configuration for model loading (model-level only)
type modelConfig struct {
	gpuLayers               int
	mlock                   bool
	mmap                    bool
	mainGPU                 string
	tensorSplit             string
	disableProgressCallback bool
	progressCallback        ProgressCallback
}

// Default model configuration
var defaultModelConfig = modelConfig{
	gpuLayers: -1, // Offload all layers to GPU by default (falls back to CPU if unavailable)
	mlock:     false,
	mmap:      true,
}

// ModelOption configures model loading behaviour (model-level settings).
type ModelOption func(*modelConfig)

// ContextOption configures context creation (context-level settings).
type ContextOption func(*contextConfig)

// GenerateOption configures text generation behaviour.
type GenerateOption func(*generateConfig)
