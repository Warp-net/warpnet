package llama

// Tool represents a tool/function that can be called by the model.
//
// Note: Tool calling is not yet implemented in the Go API, but these
// types are defined for future compatibility with models that support
// function calling (like GPT-4, Claude, etc.).
//
// When implemented, tools will be passed via ChatOptions and the model
// may return ToolCall objects in ChatResponse/ChatDelta.
//
// Example (future usage):
//
//	tool := llama.Tool{
//	    Type: "function",
//	    Function: llama.ToolFunction{
//	        Name:        "get_weather",
//	        Description: "Get the current weather in a location",
//	        Parameters: map[string]interface{}{
//	            "type": "object",
//	            "properties": map[string]interface{}{
//	                "location": map[string]interface{}{
//	                    "type":        "string",
//	                    "description": "City name",
//	                },
//	            },
//	            "required": []string{"location"},
//	        },
//	    },
//	}
type Tool struct {
	Type     string       `json:"type"`     // "function"
	Function ToolFunction `json:"function"` // Function definition
}

// ToolFunction defines a function that can be called by the model.
//
// The Parameters field should contain a JSON Schema describing the
// function's parameters. This follows the OpenAI function calling format.
type ToolFunction struct {
	Name        string                 `json:"name"`        // Function name (must be valid identifier)
	Description string                 `json:"description"` // Human-readable description
	Parameters  map[string]interface{} `json:"parameters"`  // JSON Schema for parameters
}

// ToolCall represents a function call made by the model.
//
// When a model decides to call a function, it returns a ToolCall with
// the function name and arguments (as a JSON string). The application
// should execute the function and return the result in a subsequent
// message with role "tool".
//
// Example (future usage):
//
//	// Model returns tool call
//	if len(response.ToolCalls) > 0 {
//	    call := response.ToolCalls[0]
//	    result := executeFunction(call.Function.Name, call.Function.Arguments)
//
//	    // Send result back to model
//	    messages = append(messages, llama.ChatMessage{
//	        Role:    "tool",
//	        Content: result,
//	        ToolCallID: call.ID,
//	    })
//	}
type ToolCall struct {
	ID       string `json:"id"`   // Unique identifier for this call
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`      // Function name being called
		Arguments string `json:"arguments"` // JSON string of arguments
	} `json:"function"`
}
