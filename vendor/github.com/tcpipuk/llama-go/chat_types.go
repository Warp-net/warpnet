package llama

// ChatMessage represents a message in a chat conversation.
//
// Common roles include "system", "user", "assistant", "tool", and "function".
// The role is not validated by this library - the model's chat template will
// handle role interpretation and any unknown roles.
//
// Example:
//
//	messages := []llama.ChatMessage{
//	    {Role: "system", Content: "You are a helpful assistant."},
//	    {Role: "user", Content: "What is the capital of France?"},
//	}
type ChatMessage struct {
	Role    string // Message role (e.g., "system", "user", "assistant")
	Content string // Message content
}

// ChatResponse represents the complete response from a chat completion.
//
// For standard models, only Content is populated. For reasoning models
// (like DeepSeek-R1), ReasoningContent may contain extracted thinking/
// reasoning tokens that were separated from the main response.
//
// Example:
//
//	response, err := model.Chat(ctx, messages, opts)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Response:", response.Content)
//	if response.ReasoningContent != "" {
//	    fmt.Println("Reasoning:", response.ReasoningContent)
//	}
type ChatResponse struct {
	Content          string // Regular response content
	ReasoningContent string // Extracted reasoning/thinking (if reasoning model)
	// Future fields: ToolCalls, FinishReason, Usage, etc.
}

// ChatDelta represents a streaming chunk from chat completion.
//
// During streaming, deltas arrive progressively. For standard models,
// only Content is populated with token(s). For reasoning models with
// extraction enabled, tokens may appear in either Content or
// ReasoningContent depending on whether they're inside reasoning tags.
//
// Example:
//
//	deltaCh, errCh := model.ChatStream(ctx, messages, opts)
//	for {
//	    select {
//	    case delta, ok := <-deltaCh:
//	        if !ok {
//	            return
//	        }
//	        if delta.Content != "" {
//	            fmt.Print(delta.Content)
//	        }
//	        if delta.ReasoningContent != "" {
//	            fmt.Print("[thinking: ", delta.ReasoningContent, "]")
//	        }
//	    case err := <-errCh:
//	        if err != nil {
//	            log.Fatal(err)
//	        }
//	    }
//	}
type ChatDelta struct {
	Content          string // Regular content token(s)
	ReasoningContent string // Reasoning token(s)
	// Future fields: ToolCalls, Role, FinishReason, etc.
}
