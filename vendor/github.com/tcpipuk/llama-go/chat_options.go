package llama

// ChatOptions configures chat completion behaviour.
//
// This extends the base generation options with chat-specific settings
// like template variables and reasoning parameters. All generation options
// (temperature, top_p, etc.) can be set here, or left nil to use defaults.
//
// Example:
//
//	opts := llama.ChatOptions{
//	    MaxTokens:   llama.Int(100),
//	    Temperature: llama.Float32(0.7),
//	    TopP:        llama.Float32(0.9),
//	}
type ChatOptions struct {
	// Base generation options
	MaxTokens   *int     // Maximum tokens to generate (nil = model default)
	Temperature *float32 // Sampling temperature (nil = model default, typically 0.8)
	TopP        *float32 // Nucleus sampling threshold (nil = model default, typically 0.95)
	TopK        *int     // Top-K sampling (nil = model default, typically 40)
	Seed        *int     // Random seed for reproducible generation (nil = random)
	StopWords   []string // Additional stop sequences beyond model defaults

	// Chat template (Jinja2 template string)
	// If empty, uses model's GGUF template. If model has no template, returns error.
	// Supports 40+ formats: chatml, llama2, llama3, mistral, gemma, phi3, etc.
	// See: https://github.com/ggerganov/llama.cpp/blob/master/common/chat.cpp
	ChatTemplate string

	// Chat template variables (arbitrary JSON-compatible key-value pairs)
	// These are passed to the model's Jinja2 chat template for customisation.
	// Common examples: {"add_generation_prompt": true, "tools": [...]}
	ChatTemplateKwargs map[string]interface{}

	// Reasoning model options (for models like DeepSeek-R1)
	EnableThinking  *bool           // Enable/disable thinking output (nil = model default)
	ReasoningBudget *int            // Token limit for reasoning (-1 = unlimited, 0 = disabled)
	ReasoningFormat ReasoningFormat // How to handle reasoning content

	// Streaming configuration
	StreamBufferSize int // Buffer size for streaming channels (default: 256)
}

// ReasoningFormat specifies how reasoning content is handled for models
// that emit thinking/reasoning tokens (like DeepSeek-R1).
//
// Reasoning models typically emit content within special tags like
// <think>...</think>. These formats control whether that content is
// extracted into separate ReasoningContent fields or left inline.
type ReasoningFormat int

const (
	// ReasoningFormatNone leaves reasoning content inline with regular content.
	// All tokens appear in Content/delta.Content fields.
	ReasoningFormatNone ReasoningFormat = iota

	// ReasoningFormatAuto extracts reasoning to ReasoningContent field.
	// Tokens inside reasoning tags go to ReasoningContent, others to Content.
	// This is the recommended format for reasoning models.
	ReasoningFormatAuto

	// ReasoningFormatDeepSeekLegacy extracts in non-streaming mode only.
	// For streaming: reasoning stays inline. For Chat(): extracted.
	// This matches DeepSeek's original API behaviour.
	ReasoningFormatDeepSeekLegacy

	// ReasoningFormatDeepSeek extracts reasoning in all modes.
	// Always separates reasoning content from regular content.
	ReasoningFormatDeepSeek
)

// String returns the string representation of a ReasoningFormat.
func (r ReasoningFormat) String() string {
	switch r {
	case ReasoningFormatNone:
		return "none"
	case ReasoningFormatAuto:
		return "auto"
	case ReasoningFormatDeepSeekLegacy:
		return "deepseek-legacy"
	case ReasoningFormatDeepSeek:
		return "deepseek"
	default:
		return "unknown"
	}
}
