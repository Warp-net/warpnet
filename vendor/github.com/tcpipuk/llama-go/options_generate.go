package llama

// Generation options

// WithMaxTokens sets the maximum number of tokens to generate.
//
// Generation stops after producing this many tokens, even if the model hasn't
// emitted an end-of-sequence token. This prevents runaway generation and
// controls response length.
//
// Default: 128
//
// Example:
//
//	// Generate up to 512 tokens
//	text, err := model.Generate("Write a story",
//	    llama.WithMaxTokens(512),
//	)
func WithMaxTokens(n int) GenerateOption {
	return func(c *generateConfig) {
		c.maxTokens = n
	}
}

// WithTemperature controls randomness in token selection.
//
// Higher values (e.g. 1.2) increase creativity and diversity but may reduce
// coherence. Lower values (e.g. 0.3) make output more deterministic and
// focused. Use 0.0 for fully deterministic greedy sampling (always pick the
// most likely token).
//
// Default: 0.8
//
// Examples:
//
//	// Creative writing
//	text, err := model.Generate("Write a poem",
//	    llama.WithTemperature(1.1),
//	)
//
//	// Precise factual responses
//	text, err := model.Generate("What is 2+2?",
//	    llama.WithTemperature(0.1),
//	)
func WithTemperature(t float32) GenerateOption {
	return func(c *generateConfig) {
		c.temperature = t
	}
}

// WithTopP enables nucleus sampling with the specified cumulative probability.
//
// Top-p sampling (nucleus sampling) considers only the smallest set of tokens
// whose cumulative probability exceeds p. This balances diversity and quality
// better than top-k for many tasks. Use 1.0 to disable (consider all tokens).
//
// Default: 0.95
//
// Example:
//
//	// More focused sampling
//	text, err := model.Generate("Complete this",
//	    llama.WithTopP(0.85),
//	)
func WithTopP(p float32) GenerateOption {
	return func(c *generateConfig) {
		c.topP = p
	}
}

// WithTopK limits token selection to the k most likely candidates.
//
// Top-k sampling considers only the k highest probability tokens at each step.
// Lower values increase focus and determinism, higher values increase diversity.
// Use 0 to disable (consider all tokens).
//
// Default: 40
//
// Example:
//
//	// Very focused generation
//	text, err := model.Generate("Complete this",
//	    llama.WithTopK(10),
//	)
func WithTopK(k int) GenerateOption {
	return func(c *generateConfig) {
		c.topK = k
	}
}

// WithSeed sets the random seed for reproducible generation.
//
// Using the same seed with identical settings produces deterministic output.
// Use -1 for random seed (different output each time). Useful for testing,
// debugging, or when reproducibility is required.
//
// Default: -1 (random)
//
// Example:
//
//	// Reproducible generation
//	text, err := model.Generate("Write a story",
//	    llama.WithSeed(42),
//	    llama.WithTemperature(0.8),
//	)
func WithSeed(seed int) GenerateOption {
	return func(c *generateConfig) {
		c.seed = seed
	}
}

// WithStopWords specifies sequences that terminate generation when encountered.
//
// Generation stops immediately when any stop word is produced. Useful for
// controlling response format (e.g. stopping at newlines) or implementing
// chat patterns. The stop words themselves are not included in the output.
//
// Default: none
//
// Examples:
//
//	// Stop at double newline
//	text, err := model.Generate("Q: What is AI?",
//	    llama.WithStopWords("\n\n"),
//	)
//
//	// Multiple stop sequences
//	text, err := model.Generate("User:",
//	    llama.WithStopWords("User:", "Assistant:", "\n\n"),
//	)
func WithStopWords(words ...string) GenerateOption {
	return func(c *generateConfig) {
		c.stopWords = words
	}
}

// WithDraftTokens sets the number of speculative tokens for draft model usage.
//
// When using GenerateWithDraft, the draft model speculatively generates this
// many tokens per iteration. Higher values increase potential speedup but
// waste more work if predictions are rejected. Typical range: 4-32 tokens.
//
// Default: 16
//
// Example:
//
//	target, _ := llama.LoadModel("large-model.gguf")
//	draft, _ := llama.LoadModel("small-model.gguf")
//	text, err := target.GenerateWithDraft("Write a story", draft,
//	    llama.WithDraftTokens(8),
//	)
func WithDraftTokens(n int) GenerateOption {
	return func(c *generateConfig) {
		c.draftTokens = n
	}
}

// WithDebug enables verbose logging for generation internals.
//
// When enabled, prints detailed information about token sampling, timing,
// and internal state to stderr. Useful for debugging generation issues or
// understanding model behaviour. Not recommended for production use.
//
// Default: false
//
// Example:
//
//	text, err := model.Generate("Test prompt",
//	    llama.WithDebug(),
//	)
func WithDebug() GenerateOption {
	return func(c *generateConfig) {
		c.debug = true
	}
}

// Basic sampling parameters

// WithMinP enables minimum probability threshold sampling.
//
// Min-P sampling filters out tokens with probability below p * max_probability.
// This is a modern alternative to top-p that adapts dynamically to the
// confidence of predictions. More effective than top-p for maintaining quality
// whilst allowing appropriate diversity.
//
// Default: 0.05
//
// Example:
//
//	// Stricter filtering for focused output
//	text, err := model.Generate("Explain quantum physics",
//	    llama.WithMinP(0.1),
//	)
func WithMinP(p float32) GenerateOption {
	return func(c *generateConfig) {
		c.minP = p
	}
}

// WithTypicalP enables locally typical sampling.
//
// Typical-p sampling (typ-p) filters tokens based on information content,
// keeping those with typical entropy. Use 1.0 to disable. This helps avoid
// both highly predictable and highly surprising tokens, producing more
// "typical" text that feels natural.
//
// Default: 1.0 (disabled)
//
// Example:
//
//	// Enable typical sampling
//	text, err := model.Generate("Write naturally",
//	    llama.WithTypicalP(0.95),
//	)
func WithTypicalP(p float32) GenerateOption {
	return func(c *generateConfig) {
		c.typP = p
	}
}

// WithTopNSigma enables top-n-sigma statistical filtering.
//
// Filters tokens beyond n standard deviations from the mean log probability.
// Use -1.0 to disable. This statistical approach removes unlikely outliers
// whilst preserving the natural probability distribution shape.
//
// Default: -1.0 (disabled)
//
// Example:
//
//	// Filter statistical outliers
//	text, err := model.Generate("Generate text",
//	    llama.WithTopNSigma(2.0),
//	)
func WithTopNSigma(sigma float32) GenerateOption {
	return func(c *generateConfig) {
		c.topNSigma = sigma
	}
}

// WithMinKeep sets minimum tokens to keep regardless of other filters.
//
// Ensures at least this many tokens remain available after sampling filters
// (top-k, top-p, min-p, etc.) are applied. Prevents over-aggressive filtering
// from leaving no valid tokens. Use 0 for no minimum.
//
// Default: 0
//
// Example:
//
//	// Ensure at least 5 token choices remain
//	text, err := model.Generate("Generate text",
//	    llama.WithTopK(10),
//	    llama.WithMinKeep(5),
//	)
func WithMinKeep(n int) GenerateOption {
	return func(c *generateConfig) {
		c.minKeep = n
	}
}

// Repetition penalty parameters

// WithRepeatPenalty sets the repetition penalty multiplier.
//
// Applies penalty to recently used tokens to reduce repetition. Values > 1.0
// penalise repeated tokens (1.1 = mild, 1.5 = strong). Use 1.0 to disable.
// Applied to last penalty_last_n tokens. This is the classic repetition
// penalty used in most LLM implementations.
//
// Default: 1.0 (disabled)
//
// Example:
//
//	// Reduce repetition in creative writing
//	text, err := model.Generate("Write a story",
//	    llama.WithRepeatPenalty(1.1),
//	    llama.WithPenaltyLastN(256),
//	)
func WithRepeatPenalty(penalty float32) GenerateOption {
	return func(c *generateConfig) {
		c.penaltyRepeat = penalty
	}
}

// WithFrequencyPenalty sets the frequency-based repetition penalty.
//
// Penalises tokens proportionally to how often they've appeared. Positive
// values (e.g. 0.5) discourage repetition, negative values encourage it.
// Use 0.0 to disable. Unlike repeat penalty, this considers cumulative
// frequency rather than just presence/absence.
//
// Default: 0.0 (disabled)
//
// Example:
//
//	// Discourage frequently used words
//	text, err := model.Generate("Write varied prose",
//	    llama.WithFrequencyPenalty(0.5),
//	)
func WithFrequencyPenalty(penalty float32) GenerateOption {
	return func(c *generateConfig) {
		c.penaltyFreq = penalty
	}
}

// WithPresencePenalty sets the presence-based repetition penalty.
//
// Penalises tokens that have appeared at all, regardless of frequency.
// Positive values (e.g. 0.6) encourage new topics and vocabulary. Use 0.0
// to disable. This is effective for maintaining topic diversity and
// preventing the model from fixating on specific words.
//
// Default: 0.0 (disabled)
//
// Example:
//
//	// Encourage diverse vocabulary
//	text, err := model.Generate("Write creatively",
//	    llama.WithPresencePenalty(0.6),
//	)
func WithPresencePenalty(penalty float32) GenerateOption {
	return func(c *generateConfig) {
		c.penaltyPresent = penalty
	}
}

// WithPenaltyLastN sets how many recent tokens to consider for penalties.
//
// Repetition penalties (repeat, frequency, presence) only apply to the last
// n tokens. Use 0 to disable all repetition penalties, -1 to use full context
// size. Larger values catch longer-range repetition but may over-penalise.
//
// Default: 64
//
// Example:
//
//	// Consider last 256 tokens for repetition
//	text, err := model.Generate("Write text",
//	    llama.WithRepeatPenalty(1.1),
//	    llama.WithPenaltyLastN(256),
//	)
func WithPenaltyLastN(n int) GenerateOption {
	return func(c *generateConfig) {
		c.penaltyLastN = n
	}
}

// DRY (Don't Repeat Yourself) sampling parameters

// WithDRYMultiplier enables DRY repetition penalty.
//
// DRY sampling uses sophisticated sequence matching to penalise repetitive
// patterns. The multiplier controls penalty strength (0.0 = disabled, 0.8 =
// moderate, higher = stronger). More effective than basic repetition penalties
// for catching phrase-level and structural repetition.
//
// Default: 0.0 (disabled)
//
// Example:
//
//	// Prevent repetitive patterns
//	text, err := model.Generate("Write varied text",
//	    llama.WithDRYMultiplier(0.8),
//	    llama.WithDRYBase(1.75),
//	)
func WithDRYMultiplier(mult float32) GenerateOption {
	return func(c *generateConfig) {
		c.dryMultiplier = mult
	}
}

// WithDRYBase sets the base for DRY penalty exponentiation.
//
// Controls how rapidly penalty grows for longer repeated sequences. Higher
// values penalise longer repetitions more aggressively. Only affects behaviour
// when DRY multiplier is enabled (> 0.0).
//
// Default: 1.75
//
// Example:
//
//	// Stronger penalty for long repeated sequences
//	text, err := model.Generate("Write text",
//	    llama.WithDRYMultiplier(0.8),
//	    llama.WithDRYBase(2.0),
//	)
func WithDRYBase(base float32) GenerateOption {
	return func(c *generateConfig) {
		c.dryBase = base
	}
}

// WithDRYAllowedLength sets minimum repeat length before DRY penalty applies.
//
// Repetitions shorter than this many tokens are ignored by DRY sampling.
// Prevents penalising common short phrases and natural language patterns.
// Only relevant when DRY multiplier is enabled.
//
// Default: 2
//
// Example:
//
//	// Only penalise repetitions of 4+ tokens
//	text, err := model.Generate("Write text",
//	    llama.WithDRYMultiplier(0.8),
//	    llama.WithDRYAllowedLength(4),
//	)
func WithDRYAllowedLength(length int) GenerateOption {
	return func(c *generateConfig) {
		c.dryAllowedLength = length
	}
}

// WithDRYPenaltyLastN sets how many recent tokens DRY sampling considers.
//
// DRY looks back this many tokens when detecting repetitive patterns.
// Use -1 for full context size, or specify a smaller window for efficiency.
// Only affects behaviour when DRY multiplier is enabled.
//
// Default: -1 (context size)
//
// Example:
//
//	// Check last 512 tokens for repetition
//	text, err := model.Generate("Write text",
//	    llama.WithDRYMultiplier(0.8),
//	    llama.WithDRYPenaltyLastN(512),
//	)
func WithDRYPenaltyLastN(n int) GenerateOption {
	return func(c *generateConfig) {
		c.dryPenaltyLastN = n
	}
}

// WithDRYSequenceBreakers sets sequences that break DRY repetition matching.
//
// When these sequences appear, DRY stops considering earlier tokens as part
// of a repeated pattern. Default breakers (newline, colon, quote, asterisk)
// work well for natural text structure. Only affects behaviour when DRY
// multiplier is enabled.
//
// Default: []string{"\n", ":", "\"", "*"}
//
// Example:
//
//	// Custom breakers for code generation
//	text, err := model.Generate("Write code",
//	    llama.WithDRYMultiplier(0.8),
//	    llama.WithDRYSequenceBreakers("\n", ";", "{", "}"),
//	)
func WithDRYSequenceBreakers(breakers ...string) GenerateOption {
	return func(c *generateConfig) {
		c.drySequenceBreakers = breakers
	}
}

// Dynamic temperature parameters

// WithDynamicTemperature enables entropy-based temperature adjustment.
//
// Dynamic temperature adjusts sampling temperature based on prediction entropy
// (uncertainty). The range parameter controls the adjustment span
// (0.0 = disabled, higher = more dynamic). The exponent controls how entropy
// maps to temperature. This adapts creativity to context: more focused when
// confident, more exploratory when uncertain.
//
// Default: range 0.0 (disabled), exponent 1.0
//
// Example:
//
//	// Enable dynamic temperature with range 0.5
//	text, err := model.Generate("Write adaptively",
//	    llama.WithDynamicTemperature(0.5, 1.0),
//	)
func WithDynamicTemperature(tempRange, exponent float32) GenerateOption {
	return func(c *generateConfig) {
		c.dynatempRange = tempRange
		c.dynatempExponent = exponent
	}
}

// XTC (eXclude Top Choices) sampling parameters

// WithXTC enables experimental XTC sampling for diversity.
//
// XTC probabilistically excludes the most likely token to encourage diversity.
// The probability parameter controls how often exclusion occurs (0.0 = disabled,
// 0.1 = 10% of the time). The threshold parameter limits when XTC applies
// (> 0.5 effectively disables). This is an experimental technique for reducing
// predictability.
//
// Default: probability 0.0 (disabled), threshold 0.1
//
// Example:
//
//	// Enable XTC for more surprising outputs
//	text, err := model.Generate("Write creatively",
//	    llama.WithXTC(0.1, 0.1),
//	)
func WithXTC(probability, threshold float32) GenerateOption {
	return func(c *generateConfig) {
		c.xtcProbability = probability
		c.xtcThreshold = threshold
	}
}

// Mirostat sampling parameters

// WithMirostat enables Mirostat adaptive sampling.
//
// Mirostat dynamically adjusts sampling to maintain consistent perplexity
// (surprise level). Version 0 = disabled, 1 = Mirostat v1, 2 = Mirostat v2
// (recommended). Use WithMirostatTau and WithMirostatEta to control target
// perplexity and learning rate. Mirostat replaces temperature/top-k/top-p
// with adaptive control for more consistent quality.
//
// Default: 0 (disabled)
//
// Example:
//
//	// Enable Mirostat v2 for consistent quality
//	text, err := model.Generate("Write text",
//	    llama.WithMirostat(2),
//	    llama.WithMirostatTau(5.0),
//	    llama.WithMirostatEta(0.1),
//	)
func WithMirostat(version int) GenerateOption {
	return func(c *generateConfig) {
		c.mirostat = version
	}
}

// WithMirostatTau sets target perplexity for Mirostat sampling.
//
// Tau controls the target cross-entropy (surprise level) that Mirostat tries
// to maintain. Higher values allow more surprise/diversity, lower values
// produce more focused output. Typical range: 3.0-8.0. Only affects behaviour
// when Mirostat is enabled (version 1 or 2).
//
// Default: 5.0
//
// Example:
//
//	// Lower perplexity for more focused output
//	text, err := model.Generate("Write precisely",
//	    llama.WithMirostat(2),
//	    llama.WithMirostatTau(3.0),
//	)
func WithMirostatTau(tau float32) GenerateOption {
	return func(c *generateConfig) {
		c.mirostatTau = tau
	}
}

// WithMirostatEta sets learning rate for Mirostat adaptation.
//
// Eta controls how quickly Mirostat adjusts to maintain target perplexity.
// Higher values adapt faster but may oscillate, lower values adapt smoothly
// but slowly. Typical range: 0.05-0.2. Only affects behaviour when Mirostat
// is enabled (version 1 or 2).
//
// Default: 0.1
//
// Example:
//
//	// Faster adaptation
//	text, err := model.Generate("Write text",
//	    llama.WithMirostat(2),
//	    llama.WithMirostatEta(0.15),
//	)
func WithMirostatEta(eta float32) GenerateOption {
	return func(c *generateConfig) {
		c.mirostatEta = eta
	}
}

// Other sampling parameters

// WithNPrev sets number of previous tokens to remember for sampling.
//
// Controls internal buffer size for token history used by various sampling
// methods. Rarely needs adjustment from the default. Larger values may
// improve long-range coherence at the cost of memory.
//
// Default: 64
//
// Example:
//
//	// Larger history buffer
//	text, err := model.Generate("Write text",
//	    llama.WithNPrev(128),
//	)
func WithNPrev(n int) GenerateOption {
	return func(c *generateConfig) {
		c.nPrev = n
	}
}

// WithNProbs enables probability output for top tokens.
//
// When set to n > 0, outputs probabilities for the top n most likely tokens
// at each step. Use 0 to disable (no probability output). Useful for
// analysis, debugging, or implementing custom sampling strategies. Note that
// enabling this may affect performance.
//
// Default: 0 (disabled)
//
// Example:
//
//	// Output top 5 token probabilities
//	text, err := model.Generate("Write text",
//	    llama.WithNProbs(5),
//	)
func WithNProbs(n int) GenerateOption {
	return func(c *generateConfig) {
		c.nProbs = n
	}
}

// WithIgnoreEOS continues generation past end-of-sequence tokens.
//
// When enabled, generation continues even after the model produces an EOS
// token, up to max_tokens limit. Useful for forcing longer outputs or
// exploring model behaviour beyond natural stopping points. Most applications
// should leave this disabled.
//
// Default: false
//
// Example:
//
//	// Force generation to continue past EOS
//	text, err := model.Generate("Complete this",
//	    llama.WithIgnoreEOS(true),
//	    llama.WithMaxTokens(512),
//	)
func WithIgnoreEOS(ignore bool) GenerateOption {
	return func(c *generateConfig) {
		c.ignoreEOS = ignore
	}
}
