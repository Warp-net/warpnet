#pragma once

#ifdef __cplusplus
extern "C" {
#endif

#include <stdbool.h>
#include <stdint.h>

// Progress callback type (matches llama.cpp signature)
typedef bool (*llama_progress_callback_wrapper)(float progress, void* user_data);

// Model parameters for loading
typedef struct {
    int n_ctx;              // Context size
    int n_batch;            // Batch size (logical)
    int n_ubatch;           // Micro-batch size (physical); 0 = match n_batch
    int n_gpu_layers;       // Number of GPU layers
    int n_threads;          // Number of threads for generation (per token)
    int n_threads_batch;    // Number of threads for batch processing (prompt)
    int n_parallel;         // Number of parallel sequences (for batch embeddings)
    bool f16_memory;        // Use F16 for memory
    bool mlock;            // Memory lock
    bool mmap;             // Memory mapping
    bool embeddings;       // Enable embeddings
    const char* main_gpu;   // Main GPU
    const char* tensor_split; // Tensor split
    const char* kv_cache_type; // KV cache quantization: "f16", "q8_0", "q4_0"
    const char* flash_attn;    // Flash Attention: "auto", "enabled", "disabled"
    bool disable_progress_callback;           // For silent loading
    llama_progress_callback_wrapper progress_callback;  // Custom callback
    void* progress_callback_user_data;        // User data for callback
} llama_wrapper_model_params;

// Generation parameters
typedef struct {
    const char* prompt;
    int max_tokens;
    int seed;
    const char** stop_words;
    int stop_words_count;
    int n_draft;           // For speculative sampling
    bool debug;
    uintptr_t callback_handle; // Handle to Go callback
    bool enable_prefix_caching; // Enable KV cache reuse for matching prefixes

    // Basic sampling parameters
    float temperature;
    int top_k;
    float top_p;
    float min_p;
    float typ_p;
    float top_n_sigma;
    int min_keep;

    // Repetition penalties
    int penalty_last_n;
    float penalty_repeat;
    float penalty_freq;
    float penalty_present;

    // DRY sampling
    float dry_multiplier;
    float dry_base;
    int dry_allowed_length;
    int dry_penalty_last_n;
    const char** dry_sequence_breakers;
    int dry_sequence_breakers_count;

    // Dynamic temperature
    float dynatemp_range;
    float dynatemp_exponent;

    // XTC sampling
    float xtc_probability;
    float xtc_threshold;

    // Mirostat sampling
    int mirostat;
    float mirostat_tau;
    float mirostat_eta;

    // Other parameters
    int n_prev;
    int n_probs;
    bool ignore_eos;
} llama_wrapper_generate_params;

// Callback for streaming tokens
typedef bool (*llama_wrapper_token_callback)(const char* token);

// Logging initialization
void llama_wrapper_init_logging();

// Model management
void* llama_wrapper_model_load(const char* model_path, llama_wrapper_model_params params);
void llama_wrapper_model_free(void* model);

// Context management (kept for API compatibility)
void* llama_wrapper_context_create(void* model, llama_wrapper_model_params params);
void llama_wrapper_context_free(void* ctx);

// Text generation
char* llama_wrapper_generate(void* ctx, llama_wrapper_generate_params params);
char* llama_wrapper_generate_with_tokens(void* ctx, const int* tokens, int n_tokens, int prefix_len, llama_wrapper_generate_params params);

// Speculative generation with draft model
char* llama_wrapper_generate_draft(void* ctx_target, void* ctx_draft, llama_wrapper_generate_params params);
char* llama_wrapper_generate_draft_with_tokens(void* ctx_target, void* ctx_draft, const int* tokens, int n_tokens, int target_prefix_len, int draft_prefix_len, llama_wrapper_generate_params params);

// Tokenization
int llama_wrapper_tokenize(void* ctx, const char* text, int* tokens, int max_tokens);

// Tokenise with dynamic allocation (C manages memory)
// Allocates exact size needed for tokens - caller must free with llama_wrapper_free_tokens
// tokens: output parameter for allocated token array pointer
// count: output parameter for number of tokens (or -1 on error)
void llama_wrapper_tokenize_alloc(void* ctx, const char* text, int** tokens, int* count);

// Free tokens allocated by llama_wrapper_tokenize_alloc
void llama_wrapper_free_tokens(int* tokens);

// Embeddings
int llama_wrapper_embeddings(void* ctx, const char* text, float* embeddings, int max_embeddings);

// Batch embeddings - process multiple texts efficiently
// texts: array of text strings to embed
// n_texts: number of texts in the array
// embeddings: output buffer (must have space for n_texts * n_embd floats)
// n_embd: embedding dimension from model (llama_model_n_embd)
// Returns number of embeddings generated (should equal n_texts), or -1 on error
int llama_wrapper_embeddings_batch(void* ctx, const char** texts, int n_texts, float* embeddings, int n_embd);

// Utility functions
void llama_wrapper_free_result(char* result);
const char* llama_wrapper_last_error();
int llama_wrapper_get_cached_token_count(void* ctx);

// Get model's native maximum context length
int llama_wrapper_get_model_context_length(void* model);

// Get model's embedding dimension
int llama_wrapper_model_n_embd(void* model);

// Chat template support
const char* llama_wrapper_get_chat_template(void* model);
char* llama_wrapper_apply_chat_template(const char* tmpl, const char** roles, const char** contents, int n_messages, bool add_assistant);

// Reasoning content parsing
typedef enum {
    REASONING_FORMAT_NONE = 0,
    REASONING_FORMAT_AUTO = 1,
    REASONING_FORMAT_DEEPSEEK_LEGACY = 2,
    REASONING_FORMAT_DEEPSEEK = 3
} llama_wrapper_reasoning_format;

typedef struct {
    const char* content;
    const char* reasoning_content;  // NULL if empty
} llama_wrapper_parsed_message;

// Parse model output to extract reasoning/thinking content
// For streaming: call with is_partial=true, reasoning_format=DEEPSEEK or AUTO
// Returns NULL on error. Free result with llama_wrapper_free_parsed_message()
llama_wrapper_parsed_message* llama_wrapper_parse_reasoning(
    const char* text,
    bool is_partial,
    llama_wrapper_reasoning_format format,
    int chat_format
);

void llama_wrapper_free_parsed_message(llama_wrapper_parsed_message* msg);

// Chat format auto-detection from model metadata
void* llama_wrapper_chat_templates_init(void* model, const char* template_override);
void llama_wrapper_chat_templates_free(void* templates);
int llama_wrapper_chat_templates_get_format(void* templates);

// Chat format constants (values match common_chat_format enum in llama.cpp/common/chat.h)
#define LLAMA_CHAT_FORMAT_CONTENT_ONLY 0

// Model metadata access
const char* llama_wrapper_model_meta_string(void* model, const char* key);
int llama_wrapper_model_meta_count(void* model);

// GPU information
typedef struct {
    int device_id;
    char device_name[256];
    int free_memory_mb;
    int total_memory_mb;
} llama_wrapper_gpu_info;

int llama_wrapper_get_gpu_count();
bool llama_wrapper_get_gpu_info(int device_id, llama_wrapper_gpu_info* info);

// Model runtime information
typedef struct {
    int n_ctx;           // Context size
    int n_batch;         // Batch size
    int kv_cache_size_mb; // Estimated KV cache memory usage
    int gpu_layers;      // GPU layers loaded
    int total_layers;    // Total layers in model
} llama_wrapper_runtime_info;

void llama_wrapper_get_runtime_info(void* model, void* ctx, const char* kv_cache_type, llama_wrapper_runtime_info* info);

#ifdef __cplusplus
}
#endif
