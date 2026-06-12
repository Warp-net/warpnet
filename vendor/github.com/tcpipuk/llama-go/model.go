package llama

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

/*
#cgo CFLAGS: -I./llama.cpp -I./ -I./llama.cpp/ggml/include -I./llama.cpp/include -I./llama.cpp/common -I./llama.cpp/vendor
#cgo CPPFLAGS: -I./llama.cpp -I./ -I./llama.cpp/ggml/include -I./llama.cpp/include -I./llama.cpp/common -I./llama.cpp/vendor
#include "wrapper.h"
#include <stdlib.h>

// Helper function to get the address of the Go progress callback
extern bool goProgressCallback(float progress, void* user_data);

static inline llama_progress_callback_wrapper get_go_progress_callback() {
	return (llama_progress_callback_wrapper)goProgressCallback;
}
*/
import "C"

func init() {
	// Initialise llama.cpp logging based on LLAMA_LOG environment variable
	C.llama_wrapper_init_logging()
}

// Progress callback registry for Go callbacks
var (
	progressCallbackRegistry sync.Map
	progressCallbackCounter  uintptr
	progressCallbackMutex    sync.Mutex
)

// InitLogging (re)initializes llama.cpp logging system based on LLAMA_LOG environment variable.
//
// This function is called automatically when the package loads, but can be called again
// to reconfigure logging after changing the LLAMA_LOG environment variable.
//
// Supported LLAMA_LOG values:
//   - "none" - No logging
//   - "error" - Only errors
//   - "warn" - Warnings and errors (recommended for production)
//   - "info" - Informational messages (default)
//   - "debug" - Verbose debug output
//
// Example:
//
//	os.Setenv("LLAMA_LOG", "warn")  // Quiet mode
//	llama.InitLogging()             // Apply the change
func InitLogging() {
	C.llama_wrapper_init_logging()
}

// Model represents loaded model weights.
//
// Model instances are thread-safe and can be used to create multiple execution
// contexts with different configurations. The model owns the weights in memory
// but doesn't perform inference directly - use NewContext() to create execution
// contexts.
//
// Resources are automatically freed via finaliser, but explicit Close() is
// recommended for deterministic cleanup:
//
//	model, _ := llama.LoadModel("model.gguf")
//	defer model.Close()
//
// Note: Calling methods after Close() returns an error.
type Model struct {
	modelPtr           unsafe.Pointer // llama_wrapper_model_t* (weights only)
	mu                 sync.RWMutex
	closed             bool
	chatTemplates      unsafe.Pointer // cached common_chat_templates*
	ProgressCallbackID uintptr        // Internal ID for progress callback cleanup (for testing)
}

// Config types are defined in types.go

// LoadModel loads a GGUF model from the specified path.
//
// The path must point to a valid GGUF format model file. Legacy GGML formats
// are not supported. The function applies the provided options using the
// functional options pattern, with sensible defaults if none are specified.
//
// Resources are managed automatically via finaliser, but explicit cleanup with
// Close() is recommended for deterministic resource management:
//
//	model, err := llama.LoadModel("model.gguf")
//	if err != nil {
//	    return err
//	}
//	defer model.Close()
//
// Returns an error if the file doesn't exist, is not a valid GGUF model, or
// if model loading fails.
//
// Examples:
//
//	// Load with defaults
//	model, err := llama.LoadModel("model.gguf")
//
//	// Load with custom GPU configuration
//	model, err := llama.LoadModel("model.gguf",
//	    llama.WithGPULayers(35),
//	)
func LoadModel(path string, opts ...ModelOption) (*Model, error) {
	if path == "" {
		return nil, fmt.Errorf("Model path cannot be null")
	}

	// Start with defaults
	config := defaultModelConfig

	// Apply all options
	for _, opt := range opts {
		opt(&config)
	}

	// Convert Go config to C struct for model loading
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	var cMainGPU *C.char
	if config.mainGPU != "" {
		cMainGPU = C.CString(config.mainGPU)
		defer C.free(unsafe.Pointer(cMainGPU))
	}

	var cTensorSplit *C.char
	if config.tensorSplit != "" {
		cTensorSplit = C.CString(config.tensorSplit)
		defer C.free(unsafe.Pointer(cTensorSplit))
	}

	params := C.llama_wrapper_model_params{
		n_ctx:           0, // Not used for model loading
		n_batch:         0, // Not used for model loading
		n_ubatch:        0, // Not used for model loading
		n_gpu_layers:    C.int(config.gpuLayers),
		n_threads:       0, // Not used for model loading
		n_threads_batch: 0, // Not used for model loading
		n_parallel:      0, // Not used for model loading
		f16_memory:      false,
		mlock:           C.bool(config.mlock),
		mmap:            C.bool(config.mmap),
		embeddings:      false,
		main_gpu:        cMainGPU,
		tensor_split:    cTensorSplit,
		kv_cache_type:   nil,
		flash_attn:      nil,
	}

	// Configure progress callback if requested
	var callbackID uintptr
	if config.progressCallback != nil {
		progressCallbackMutex.Lock()
		progressCallbackCounter++
		callbackID = progressCallbackCounter
		progressCallbackMutex.Unlock()

		progressCallbackRegistry.Store(callbackID, config.progressCallback)

		// Set C callback (using helper function to get the function pointer)
		params.progress_callback = C.get_go_progress_callback()
		params.progress_callback_user_data = unsafe.Pointer(callbackID)
	} else if config.disableProgressCallback {
		params.disable_progress_callback = C.bool(true)
	}

	// Load model (weights only)
	modelPtr := C.llama_wrapper_model_load(cPath, params)
	if modelPtr == nil {
		// Clean up callback registry on failure
		if callbackID != 0 {
			progressCallbackRegistry.Delete(callbackID)
		}
		return nil, fmt.Errorf("failed to load model: %s", C.GoString(C.llama_wrapper_last_error()))
	}

	model := &Model{
		modelPtr:           modelPtr,
		ProgressCallbackID: callbackID,
	}

	// Set finaliser to ensure cleanup
	runtime.SetFinalizer(model, (*Model).Close)

	return model, nil
}

// NewContext creates a new execution context from this model.
//
// This method creates an execution context with the specified configuration.
// Multiple contexts can be created from the same model to handle different
// use cases (e.g., small context for tokenization, large context for generation).
//
// Each context maintains its own KV cache and state. For concurrent inference,
// create multiple contexts from the same model - this is VRAM efficient since
// contexts share the model weights (e.g., 7GB model + 100MB per context).
//
// Thread safety: Model is thread-safe, but each Context is not. Use one context
// per goroutine for concurrent inference.
//
// See also: Context.Generate, Context.Chat for inference operations.
//
// Example:
//
//	// Load model once
//	model, _ := llama.LoadModel("model.gguf", llama.WithGPULayers(-1))
//	defer model.Close()
//
//	// Create context for tokenization
//	tokCtx, _ := model.NewContext(
//	    llama.WithContext(512),
//	    llama.WithKVCacheType("f16"),
//	)
//	defer tokCtx.Close()
//
//	// Create context for generation
//	genCtx, _ := model.NewContext(
//	    llama.WithContext(8192),
//	    llama.WithKVCacheType("q8_0"),
//	)
//	defer genCtx.Close()
func (m *Model) NewContext(opts ...ContextOption) (*Context, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, fmt.Errorf("model is closed")
	}
	modelPtr := m.modelPtr
	m.mu.RUnlock()

	// Start with default context config
	config := defaultContextConfig

	// Apply all options
	for _, opt := range opts {
		opt(&config)
	}

	// Auto-set nParallel for embeddings if not explicitly configured
	if config.embeddings && config.nParallel == 1 {
		config.nParallel = 8
	}

	// Query model's native context if user didn't specify
	if config.contextSize == 0 {
		nativeContext := int(C.llama_wrapper_get_model_context_length(modelPtr))
		config.contextSize = nativeContext
	}

	// Optimisation: clamp batch size to context size
	if config.batchSize > config.contextSize {
		config.batchSize = config.contextSize
	}

	// Convert Go config to C struct for context creation
	var cKVCacheType *C.char
	if config.kvCacheType != "" {
		cKVCacheType = C.CString(config.kvCacheType)
		defer C.free(unsafe.Pointer(cKVCacheType))
	}

	var cFlashAttn *C.char
	if config.flashAttn != "" {
		cFlashAttn = C.CString(config.flashAttn)
		defer C.free(unsafe.Pointer(cFlashAttn))
	}

	params := C.llama_wrapper_model_params{
		n_ctx:           C.int(config.contextSize),
		n_batch:         C.int(config.batchSize),
		n_ubatch:        C.int(config.uBatchSize),
		n_gpu_layers:    0, // Not used for context creation (model already loaded)
		n_threads:       C.int(config.threads),
		n_threads_batch: C.int(config.threadsBatch),
		n_parallel:      C.int(config.nParallel),
		f16_memory:      C.bool(config.f16Memory),
		mlock:           false, // Not used for context creation
		mmap:            false, // Not used for context creation
		embeddings:      C.bool(config.embeddings),
		main_gpu:        nil, // Not used for context creation
		tensor_split:    nil, // Not used for context creation
		kv_cache_type:   cKVCacheType,
		flash_attn:      cFlashAttn,
	}

	// Create context
	ctxPtr := C.llama_wrapper_context_create(modelPtr, params)
	if ctxPtr == nil {
		return nil, fmt.Errorf("failed to create context: %s", C.GoString(C.llama_wrapper_last_error()))
	}

	ctx := &Context{
		contextPtr: ctxPtr,
		model:      m,
		config:     config,
	}

	// Set finaliser to ensure cleanup
	runtime.SetFinalizer(ctx, (*Context).Close)

	return ctx, nil
}

// Close frees the model and its associated resources.
//
// This method is idempotent - multiple calls are safe and subsequent calls
// return immediately without error.
//
// After Close() is called, all other methods return an error. The method uses
// a write lock to prevent concurrent operations during cleanup.
//
// Example:
//
//	model, _ := llama.LoadModel("model.gguf")
//	defer model.Close()
func (m *Model) Close() error {
	m.mu.Lock() // Write lock to block all operations
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	// Remove finaliser FIRST to prevent race with GC
	runtime.SetFinalizer(m, nil)

	// Clean up progress callback registry
	if m.ProgressCallbackID != 0 {
		progressCallbackRegistry.Delete(m.ProgressCallbackID)
		m.ProgressCallbackID = 0
	}

	// Free chat templates if cached
	if m.chatTemplates != nil {
		C.llama_wrapper_chat_templates_free(m.chatTemplates)
		m.chatTemplates = nil
	}

	// Free model
	if m.modelPtr != nil {
		C.llama_wrapper_model_free(m.modelPtr)
		m.modelPtr = nil
	}

	m.closed = true
	return nil
}

// ChatTemplate returns the chat template from the model's GGUF metadata.
//
// Returns an empty string if the model has no embedded chat template.
// Most modern instruction-tuned models include a template in their GGUF metadata
// that specifies how to format messages for that specific model.
//
// Example:
//
//	template := model.ChatTemplate()
//	if template == "" {
//	    // Model has no template - user must provide one
//	}
func (m *Model) ChatTemplate() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return ""
	}

	// Call C function to get template from model metadata
	cTemplate := C.llama_wrapper_get_chat_template(m.modelPtr)
	if cTemplate == nil {
		return ""
	}

	return C.GoString(cTemplate)
}

// FormatChatPrompt formats chat messages using the model's chat template.
//
// This method applies the chat template to the provided messages and returns
// the resulting prompt string without performing generation. Useful for:
//   - Debugging what will be sent to the model
//   - Pre-computing prompts for caching
//   - Understanding how the template formats conversations
//
// The template priority is: opts.ChatTemplate > model's GGUF template > error.
//
// See also: Context.Chat for performing chat completion with generation.
//
// Example:
//
//	messages := []llama.ChatMessage{
//	    {Role: "system", Content: "You are helpful."},
//	    {Role: "user", Content: "Hello"},
//	}
//	prompt, err := model.FormatChatPrompt(messages, llama.ChatOptions{})
//	fmt.Println("Formatted prompt:", prompt)
func (m *Model) FormatChatPrompt(messages []ChatMessage, opts ChatOptions) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return "", fmt.Errorf("model is closed")
	}

	// Use the same template resolution logic as Chat/ChatStream
	template := opts.ChatTemplate
	if template == "" {
		template = m.ChatTemplate()
	}
	if template == "" {
		return "", fmt.Errorf("no chat template available: provide ChatOptions.ChatTemplate or use a model with embedded template")
	}

	// Apply template with addAssistant=true (same as generation)
	return applyChatTemplate(template, messages, true)
}

// getChatFormat gets the auto-detected chat format for reasoning parsing.
// This is cached on the model to avoid repeated detection.
func (m *Model) getChatFormat() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize templates if not cached
	if m.chatTemplates == nil {
		m.chatTemplates = C.llama_wrapper_chat_templates_init(m.modelPtr, nil)
		if m.chatTemplates == nil {
			// Fallback to CONTENT_ONLY if init fails
			return int(C.LLAMA_CHAT_FORMAT_CONTENT_ONLY)
		}
	}

	return int(C.llama_wrapper_chat_templates_get_format(m.chatTemplates))
}

// applyChatTemplate applies a Jinja2 chat template to messages.
//
// This is an internal helper that wraps llama.cpp's native chat template system.
// The template can be from GGUF metadata or a custom Jinja2 template string.
//
// Returns the formatted prompt string ready for generation, or an error if
// template application fails.
func applyChatTemplate(template string, messages []ChatMessage, addAssistant bool) (string, error) {
	if template == "" {
		return "", fmt.Errorf("template cannot be empty")
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("messages cannot be empty")
	}

	// Convert template to C string
	cTemplate := C.CString(template)
	defer C.free(unsafe.Pointer(cTemplate))

	// Build C arrays for roles and contents
	cRoles := make([]*C.char, len(messages))
	cContents := make([]*C.char, len(messages))

	// Allocate C strings and set up defer cleanup
	for i, msg := range messages {
		cRoles[i] = C.CString(msg.Role)
		cContents[i] = C.CString(msg.Content)
	}

	// Defer cleanup of all C strings
	defer func() {
		for i := range messages {
			C.free(unsafe.Pointer(cRoles[i]))
			C.free(unsafe.Pointer(cContents[i]))
		}
	}()

	// Call C function to apply template
	cResult := C.llama_wrapper_apply_chat_template(
		cTemplate,
		(**C.char)(unsafe.Pointer(&cRoles[0])),
		(**C.char)(unsafe.Pointer(&cContents[0])),
		C.int(len(messages)),
		C.bool(addAssistant),
	)

	if cResult == nil {
		return "", fmt.Errorf("failed to apply chat template: %s", C.GoString(C.llama_wrapper_last_error()))
	}

	// Convert result and free
	result := C.GoString(cResult)
	C.llama_wrapper_free_result(cResult)

	return result, nil
}
