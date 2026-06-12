#include "wrapper.h"
#include "llama.cpp/include/llama.h"
#include "llama.cpp/ggml/include/ggml.h"
#include "llama.cpp/common/common.h"
#include "llama.cpp/common/sampling.h"
#include "llama.cpp/common/speculative.h"
#include "llama.cpp/common/chat.h"
// Use angle-bracket form so this resolves via -I paths in both modes:
// dev (-I./llama.cpp/vendor → submodule) and consumer
// (-I./cgo_headers/llama.cpp/thirdparty → vendored copy). The vendored copy
// can't live under a directory named "vendor" because Go's module zip excludes
// any nested vendor/ subtree.
#include <nlohmann/json.hpp>

#include <string>
#include <vector>
#include <memory>
#include <cstring>

// CUDA backend header for GPU info
#ifdef GGML_USE_CUDA
#include "llama.cpp/ggml/include/ggml-cuda.h"
#endif

// Global error handling
static std::string g_last_error;

// Global log level control
static ggml_log_level g_min_log_level = GGML_LOG_LEVEL_INFO;

// Log callback that respects LLAMA_LOG environment variable
static void llama_log_callback(ggml_log_level level, const char * text, void * /*user_data*/) {
    if (level >= g_min_log_level) {
        fprintf(stderr, "%s", text);
    }
}

extern "C" {

// Initialise logging based on LLAMA_LOG environment variable
// Supported values: none, debug, info (default), warn, error
void llama_wrapper_init_logging() {
    const char* log_level = std::getenv("LLAMA_LOG");
    if (log_level != nullptr) {
        std::string level_str(log_level);
        if (level_str == "none") {
            g_min_log_level = GGML_LOG_LEVEL_NONE;
        } else if (level_str == "debug") {
            g_min_log_level = GGML_LOG_LEVEL_DEBUG;
        } else if (level_str == "info") {
            g_min_log_level = GGML_LOG_LEVEL_INFO;
        } else if (level_str == "warn") {
            g_min_log_level = GGML_LOG_LEVEL_WARN;
        } else if (level_str == "error") {
            g_min_log_level = GGML_LOG_LEVEL_ERROR;
        }
    }
    llama_log_set(llama_log_callback, nullptr);
}

// Forward declarations of Go callback functions
extern bool goTokenCallback(uintptr_t handle, const char* token);
extern bool goProgressCallback(float progress, void* user_data);

// Separate wrappers for model and context
struct llama_wrapper_model_t {
    llama_model* model;
    int n_gpu_layers;  // Number of GPU layers requested (for stats reporting)
};

struct llama_wrapper_context_t {
    llama_context* ctx;
    llama_model* model;  // Reference to parent model
    std::vector<int> cached_tokens;  // Cache for prefix matching optimisation
};

const char* llama_wrapper_last_error() {
    return g_last_error.c_str();
}

void llama_wrapper_free_result(char* result) {
    if (result) {
        free(result);
    }
}

// Static no-op callback for silent loading
static bool silent_progress_callback(float progress, void* user_data) {
    (void)progress;
    (void)user_data;
    return true;  // Continue loading
}

// Convert our params to llama.cpp model params
static struct llama_model_params convert_model_params(llama_wrapper_model_params params) {
    struct llama_model_params model_params = llama_model_default_params();

    // Only set n_gpu_layers if not -1 (which means "use default/all layers")
    // llama.cpp default is 999 which effectively means all layers
    if (params.n_gpu_layers != -1) {
        model_params.n_gpu_layers = params.n_gpu_layers;
    }

    model_params.main_gpu = params.main_gpu ? atoi(params.main_gpu) : 0;
    model_params.use_mmap = params.mmap;
    model_params.use_mlock = params.mlock;
    model_params.no_host = false;  // Use host buffers (b6709 added field)

    // Configure progress callback
    if (params.disable_progress_callback) {
        model_params.progress_callback = silent_progress_callback;
        model_params.progress_callback_user_data = nullptr;
    } else if (params.progress_callback) {
        model_params.progress_callback = params.progress_callback;
        model_params.progress_callback_user_data = params.progress_callback_user_data;
    }
    // Otherwise NULL → llama.cpp installs default dot printer

    return model_params;
}

// Convert our params to llama.cpp context params
static struct llama_context_params convert_context_params(llama_wrapper_model_params params) {
    struct llama_context_params ctx_params = llama_context_default_params();
    ctx_params.n_ctx = params.n_ctx > 0 ? params.n_ctx : 2048;
    ctx_params.n_batch = params.n_batch > 0 ? params.n_batch : 512;
    // n_ubatch: 0 means match n_batch. Encoder-only models (nomic, BERT) require
    // n_ubatch >= longest sequence length, since the whole input must fit in one
    // forward pass.
    ctx_params.n_ubatch = params.n_ubatch > 0 ? params.n_ubatch : ctx_params.n_batch;
    ctx_params.n_threads = params.n_threads > 0 ? params.n_threads : 4;
    ctx_params.n_threads_batch = params.n_threads_batch > 0 ? params.n_threads_batch : ctx_params.n_threads;
    ctx_params.n_seq_max = params.n_parallel > 0 ? params.n_parallel : 1;
    ctx_params.embeddings = params.embeddings;

    // Set KV cache quantization type
    if (params.kv_cache_type != nullptr) {
        std::string cache_type(params.kv_cache_type);
        if (cache_type == "f16") {
            ctx_params.type_k = GGML_TYPE_F16;
            ctx_params.type_v = GGML_TYPE_F16;
        } else if (cache_type == "q8_0") {
            ctx_params.type_k = GGML_TYPE_Q8_0;
            ctx_params.type_v = GGML_TYPE_Q8_0;
        } else if (cache_type == "q4_0") {
            ctx_params.type_k = GGML_TYPE_Q4_0;
            ctx_params.type_v = GGML_TYPE_Q4_0;
        }
        // If unrecognized, leave as default (f16)
    }

    // Set Flash Attention mode
    if (params.flash_attn != nullptr) {
        std::string fa_mode(params.flash_attn);
        if (fa_mode == "enabled") {
            ctx_params.flash_attn_type = LLAMA_FLASH_ATTN_TYPE_ENABLED;
        } else if (fa_mode == "disabled") {
            ctx_params.flash_attn_type = LLAMA_FLASH_ATTN_TYPE_DISABLED;
        } else if (fa_mode == "auto") {
            ctx_params.flash_attn_type = LLAMA_FLASH_ATTN_TYPE_AUTO;
        }
        // If unrecognized, leave as default (auto)
    }

    return ctx_params;
}

void* llama_wrapper_model_load(const char* model_path, llama_wrapper_model_params params) {
    if (!model_path) {
        g_last_error = "Model path cannot be null";
        return nullptr;
    }

    try {
        // Initialize llama backend
        llama_backend_init();

        // Load model (weights only)
        auto model_params = convert_model_params(params);
        llama_model* model = llama_model_load_from_file(model_path, model_params);
        if (!model) {
            g_last_error = "Failed to load model from: " + std::string(model_path);
            return nullptr;
        }

        // Create wrapper (model only, no context)
        auto wrapper = new llama_wrapper_model_t();
        wrapper->model = model;
        // Store n_gpu_layers for stats reporting
        // If -1 was passed (meaning "use default"), llama.cpp uses 999 layers
        wrapper->n_gpu_layers = (params.n_gpu_layers == -1) ? 999 : params.n_gpu_layers;

        return wrapper;
    } catch (const std::exception& e) {
        g_last_error = "Exception loading model: " + std::string(e.what());
        return nullptr;
    }
}

void llama_wrapper_model_free(void* model) {
    if (!model) return;

    auto wrapper = static_cast<llama_wrapper_model_t*>(model);
    if (wrapper->model) {
        llama_model_free(wrapper->model);
        wrapper->model = nullptr;  // Prevent double-free
    }
    delete wrapper;
}

void* llama_wrapper_context_create(void* model, llama_wrapper_model_params params) {
    if (!model) {
        g_last_error = "Model cannot be null";
        return nullptr;
    }

    try {
        auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);

        // Create context from model
        auto ctx_params = convert_context_params(params);
        llama_context* ctx = llama_init_from_model(model_wrapper->model, ctx_params);
        if (!ctx) {
            g_last_error = "Failed to create context";
            return nullptr;
        }

        // Create context wrapper
        auto ctx_wrapper = new llama_wrapper_context_t();
        ctx_wrapper->ctx = ctx;
        ctx_wrapper->model = model_wrapper->model;  // Keep reference to parent model

        return ctx_wrapper;
    } catch (const std::exception& e) {
        g_last_error = "Exception creating context: " + std::string(e.what());
        return nullptr;
    }
}

void llama_wrapper_context_free(void* ctx) {
    if (!ctx) return;

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);
    if (wrapper->ctx) {
        llama_free(wrapper->ctx);
        wrapper->ctx = nullptr;  // Prevent double-free
    }
    delete wrapper;
}

// Get model's native maximum context length from GGUF metadata
int llama_wrapper_get_model_context_length(void* model) {
    if (!model) {
        return 32768;  // Fallback if model is null
    }

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);

    // Query model's native context length from GGUF metadata
    int n_ctx_train = llama_model_n_ctx_train(model_wrapper->model);

    // Return model's training context, or reasonable fallback
    return (n_ctx_train > 0) ? n_ctx_train : 32768;
}

// Get model's embedding dimension
int llama_wrapper_model_n_embd(void* model) {
    if (!model) {
        return -1;  // Error if model is null
    }

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);
    return llama_model_n_embd(model_wrapper->model);
}

// Helper function to find common prefix length between two token vectors
static int findCommonPrefix(const std::vector<int>& a, const std::vector<int>& b) {
    int commonLen = 0;
    size_t minLen = std::min(a.size(), b.size());
    for (size_t i = 0; i < minLen; i++) {
        if (a[i] != b[i]) {
            break;
        }
        commonLen++;
    }
    return commonLen;
}

char* llama_wrapper_generate_with_tokens(void* ctx, const int* tokens, int n_tokens, int prefix_len, llama_wrapper_generate_params params) {
    if (!ctx || !tokens) {
        g_last_error = "Context and tokens cannot be null";
        return nullptr;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);
    if (!wrapper->ctx) {
        g_last_error = "Context has been freed";
        return nullptr;
    }

    try {
        // Convert C tokens to vector
        std::vector<llama_token> prompt_tokens(tokens, tokens + n_tokens);

        if (prompt_tokens.empty()) {
            g_last_error = "Token array is empty";
            return nullptr;
        }

        // Check context size with safety margin BEFORE manipulating KV cache
        int available_ctx = llama_n_ctx(wrapper->ctx);
        if (available_ctx <= 0) {
            g_last_error = "Invalid context size";
            return nullptr;
        }
        // Check if prompt fits with room for at least a few generated tokens
        int tokens_needed = (int)prompt_tokens.size() + params.max_tokens;
        if (tokens_needed > available_ctx) {
            char err_msg[256];
            snprintf(err_msg, sizeof(err_msg),
                    "Prompt too long for context size: need %d tokens (%d prompt + %d generation) but context is only %d tokens",
                    tokens_needed, (int)prompt_tokens.size(), params.max_tokens > 0 ? params.max_tokens : 128, available_ctx);
            g_last_error = err_msg;
            return nullptr;
        }
        if ((int)prompt_tokens.size() >= available_ctx - 1) {
            g_last_error = "Prompt too long for context size (need at least 1 token for generation)";
            return nullptr;
        }

        // Clear KV cache from divergence point onwards
        // For full cache hits, we'll refresh the last prompt token, so clear from prefix_len - 1
        // For partial matches, clear from prefix_len as usual
        int clear_from = (prefix_len == n_tokens && n_tokens > 0) ? prefix_len - 1 : prefix_len;
        // Only clear if clear_from is valid and within context bounds
        if (clear_from >= 0 && clear_from < available_ctx) {
            llama_memory_seq_rm(llama_get_memory(wrapper->ctx), 0, clear_from, -1);
        }

        // Create sampling parameters - use the struct directly instead of calling a function
        common_params_sampling sampling_params;
        // Basic sampling
        sampling_params.seed = params.seed;
        sampling_params.temp = params.temperature;
        sampling_params.top_k = params.top_k;
        sampling_params.top_p = params.top_p;
        sampling_params.min_p = params.min_p;
        sampling_params.typ_p = params.typ_p;
        sampling_params.top_n_sigma = params.top_n_sigma;
        sampling_params.min_keep = params.min_keep;

        // Repetition penalties
        sampling_params.penalty_last_n = params.penalty_last_n;
        sampling_params.penalty_repeat = params.penalty_repeat;
        sampling_params.penalty_freq = params.penalty_freq;
        sampling_params.penalty_present = params.penalty_present;

        // DRY sampling
        sampling_params.dry_multiplier = params.dry_multiplier;
        sampling_params.dry_base = params.dry_base;
        sampling_params.dry_allowed_length = params.dry_allowed_length;
        sampling_params.dry_penalty_last_n = params.dry_penalty_last_n;
        // Convert dry_sequence_breakers from C array to std::vector
        sampling_params.dry_sequence_breakers.clear();
        for (int i = 0; i < params.dry_sequence_breakers_count; i++) {
            sampling_params.dry_sequence_breakers.push_back(std::string(params.dry_sequence_breakers[i]));
        }

        // Dynamic temperature
        sampling_params.dynatemp_range = params.dynatemp_range;
        sampling_params.dynatemp_exponent = params.dynatemp_exponent;

        // XTC sampling
        sampling_params.xtc_probability = params.xtc_probability;
        sampling_params.xtc_threshold = params.xtc_threshold;

        // Mirostat sampling
        sampling_params.mirostat = params.mirostat;
        sampling_params.mirostat_tau = params.mirostat_tau;
        sampling_params.mirostat_eta = params.mirostat_eta;

        // Other parameters
        sampling_params.n_prev = params.n_prev;
        sampling_params.n_probs = params.n_probs;
        sampling_params.ignore_eos = params.ignore_eos;

        // Initialise sampler
        common_sampler* sampler = common_sampler_init(wrapper->model, sampling_params);
        if (!sampler) {
            g_last_error = "Failed to initialise sampler";
            return nullptr;
        }

        // Validate generation parameters
        // Reject negative max_tokens (0 is allowed and means "use default")
        if (params.max_tokens < 0) {
            common_sampler_free(sampler);
            g_last_error = "Invalid max_tokens value (must be >= 0)";
            return nullptr;
        }
        int n_predict = params.max_tokens > 0 ? params.max_tokens : 128;

        // After clearing cache from prefix_len onwards, cache ends at prefix_len - 1
        // Next position to use is prefix_len
        int n_past = prefix_len;

        // Process prompt tokens from prefix_len onwards using explicit positions
        if (prefix_len < n_tokens) {
            int tokens_to_process = n_tokens - prefix_len;
            int n_batch = llama_n_batch(wrapper->ctx);

            // Process tokens in chunks that respect n_batch limit
            for (int chunk_start = 0; chunk_start < tokens_to_process; chunk_start += n_batch) {
                int chunk_size = std::min(n_batch, tokens_to_process - chunk_start);
                llama_batch batch = llama_batch_init(chunk_size, 0, 1);
                common_batch_clear(batch);

                // Add tokens for this chunk with explicit positions
                for (int i = 0; i < chunk_size; i++) {
                    int token_idx = prefix_len + chunk_start + i;
                    int position = prefix_len + chunk_start + i;
                    // Only the very last token of the entire prompt needs logits
                    bool needs_logits = (chunk_start + i == tokens_to_process - 1);
                    common_batch_add(batch, prompt_tokens[token_idx], position, { 0 }, needs_logits);
                }

                if (llama_decode(wrapper->ctx, batch) != 0) {
                    if (params.debug) {
                        fprintf(stderr, "WARNING: prompt decode failed for chunk starting at %d\n", chunk_start);
                    }
                    llama_batch_free(batch);
                    common_sampler_free(sampler);
                    g_last_error = "Failed to decode prompt";
                    return nullptr;
                }

                llama_batch_free(batch);
            }

            n_past = n_tokens;  // Position now at end of prompt
        } else if (prefix_len == n_tokens && n_tokens > 0) {
            // Full cache hit - refresh last token's logits to ensure determinism
            // This is critical: without this, we sample from stale logits from the previous generation
            // The last prompt token is at position n_tokens - 1 (0-indexed positions)
            llama_batch batch = llama_batch_init(512, 0, 1);
            common_batch_clear(batch);
            common_batch_add(batch, prompt_tokens[n_tokens - 1], n_tokens - 1, { 0 }, true);

            if (llama_decode(wrapper->ctx, batch) != 0) {
                if (params.debug) {
                    fprintf(stderr, "WARNING: logit refresh failed\n");
                }
                llama_batch_free(batch);
                common_sampler_free(sampler);
                g_last_error = "Failed to refresh logits for cached prompt";
                return nullptr;
            }

            llama_batch_free(batch);
            n_past = n_tokens;  // Set position to end of prompt for generation
        }
        // If n_tokens == 0, nothing to decode

        // Generation loop - follows simple.cpp pattern
        std::string result;
        int n_decode = 0;

        if (params.debug) {
            fprintf(stderr, "DEBUG: Starting generation loop, n_predict=%d, n_past=%d\n", n_predict, n_past);
        }

        // Main generation loop - decode first, then sample
        for (int n_gen = 0; n_gen < n_predict; n_gen++) {
            if (params.debug && n_gen == 0) {
                fprintf(stderr, "DEBUG: First iteration, about to sample\n");
            }

            // Sample the next token (using logits from previous decode or prompt)
            llama_token new_token_id = common_sampler_sample(sampler, wrapper->ctx, -1);

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "DEBUG: Sampled token: %d\n", new_token_id);
            }

            // Check for EOS
            if (llama_vocab_is_eog(llama_model_get_vocab(wrapper->model), new_token_id)) {
                if (params.debug) {
                    fprintf(stderr, "INFO: End of generation token encountered\n");
                }
                break;
            }

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "DEBUG: About to convert token to text\n");
            }

            // Convert token to text
            std::string token_str = common_token_to_piece(wrapper->ctx, new_token_id);

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "DEBUG: Token text: '%s'\n", token_str.c_str());
            }

            // Call callback if provided
            if (params.callback_handle != 0) {
                if (!goTokenCallback(params.callback_handle, token_str.c_str())) {
                    if (params.debug) {
                        fprintf(stderr, "INFO: Generation stopped by callback\n");
                    }
                    break;
                }
            }

            result += token_str;

            // Check stop words
            for (int j = 0; j < params.stop_words_count; j++) {
                if (result.find(params.stop_words[j]) != std::string::npos) {
                    if (params.debug) {
                        fprintf(stderr, "INFO: Stop word found, ending generation\n");
                    }
                    goto generation_done;
                }
            }

            if (params.debug && n_gen == 0) {
                // Query actual cache state before decode
                int cache_pos = llama_memory_seq_pos_max(llama_get_memory(wrapper->ctx), 0);
                fprintf(stderr, "DEBUG: About to decode token, n_past=%d, cache_pos_max=%d\n", n_past, cache_pos);
            }

            // Decode the sampled token to get logits for next iteration
            // Allocate enough space for the batch (minimum 512 tokens as per llama.cpp examples)
            llama_batch gen_batch = llama_batch_init(512, 0, 1);
            common_batch_clear(gen_batch);
            common_batch_add(gen_batch, new_token_id, n_past, { 0 }, true);

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "DEBUG: Batch token=%d, pos=%d, n_tokens=%d\n", new_token_id, n_past, gen_batch.n_tokens);
            }

            // Increment position for next iteration
            n_past++;

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "DEBUG: Batch prepared, calling llama_decode\n");
            }

            if (llama_decode(wrapper->ctx, gen_batch) != 0) {
                if (params.debug) {
                    fprintf(stderr, "WARNING: decode failed, stopping generation\n");
                }
                llama_batch_free(gen_batch);
                break;
            }

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "DEBUG: Decode succeeded, freeing batch\n");
            }

            llama_batch_free(gen_batch);
            n_decode += 1;

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "DEBUG: First iteration complete\n");
            }
        }

generation_done:
        common_sampler_free(sampler);

        // Return allocated string (caller must free)
        char* c_result = (char*)malloc(result.length() + 1);
        if (c_result) {
            memcpy(c_result, result.c_str(), result.length());
            c_result[result.length()] = '\0';
        } else {
            g_last_error = "Failed to allocate memory for result";
        }
        return c_result;

    } catch (const std::exception& e) {
        g_last_error = "Exception during generation: " + std::string(e.what());
        return nullptr;
    }
}

// Simple wrapper that tokenises the prompt and handles prefix caching automatically
char* llama_wrapper_generate(void* ctx, llama_wrapper_generate_params params) {
    if (!ctx) {
        g_last_error = "Context cannot be null";
        return nullptr;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);
    if (!wrapper->ctx) {
        g_last_error = "Context has been freed";
        return nullptr;
    }

    try {
        // Tokenise the prompt
        std::vector<llama_token> prompt_tokens = common_tokenize(wrapper->ctx, params.prompt, true, true);

        if (prompt_tokens.empty()) {
            g_last_error = "Failed to tokenize prompt";
            return nullptr;
        }

        // Convert to int vector for comparison
        std::vector<int> tokens_int(prompt_tokens.begin(), prompt_tokens.end());

        // Find common prefix with cached tokens (only if prefix caching enabled)
        int prefix_len = params.enable_prefix_caching
            ? findCommonPrefix(wrapper->cached_tokens, tokens_int)
            : 0;

        // Update cache to new token sequence (only if prefix caching enabled)
        if (params.enable_prefix_caching) {
            wrapper->cached_tokens = tokens_int;
        } else {
            wrapper->cached_tokens.clear();  // Ensure cache is empty when disabled
        }

        // Call token-based generation with prefix caching
        return llama_wrapper_generate_with_tokens(ctx, tokens_int.data(), tokens_int.size(), prefix_len, params);

    } catch (const std::exception& e) {
        g_last_error = "Exception during generation: " + std::string(e.what());
        return nullptr;
    }
}

char* llama_wrapper_generate_draft_with_tokens(void* ctx_target, void* ctx_draft, const int* tokens, int n_tokens, int target_prefix_len, int draft_prefix_len, llama_wrapper_generate_params params) {
    if (!ctx_target || !ctx_draft || !tokens) {
        g_last_error = "Target, draft contexts and tokens cannot be null";
        return nullptr;
    }

    auto wrapper_tgt = static_cast<llama_wrapper_context_t*>(ctx_target);
    auto wrapper_dft = static_cast<llama_wrapper_context_t*>(ctx_draft);
    if (!wrapper_tgt->ctx) {
        g_last_error = "Target context has been freed";
        return nullptr;
    }
    if (!wrapper_dft->ctx) {
        g_last_error = "Draft context has been freed";
        return nullptr;
    }

    try {
        // Clear target KV cache from divergence point
        // Sequence ID 0 is the default sequence for single-sequence inference
        // For speculative generation with full cache hits, we need to refresh the second-to-last token
        // (since we decode all but last token), so clear from that position
        int target_clear_from = (target_prefix_len == n_tokens && n_tokens > 1) ? n_tokens - 2 : target_prefix_len;
        llama_memory_seq_rm(llama_get_memory(wrapper_tgt->ctx), 0, target_clear_from, -1);
        // Note: draft KV cache is managed internally by common_speculative (b8635+)

        // Convert C tokens to vector
        std::vector<llama_token> prompt_tokens(tokens, tokens + n_tokens);

        if (prompt_tokens.empty()) {
            g_last_error = "Token array is empty";
            return nullptr;
        }

        // Set up speculative parameters.
        // b9002+: common_params_speculative uses a nested draft substructure
        // (params.draft.* instead of the flat fields used pre-b9002).
        // common_speculative_init checks !params.draft.mparams.path.empty() to
        // decide whether the draft implementation should be registered, so the
        // path field must be set even though we're passing the model directly.
        common_params_speculative spec_params;
        spec_params.type = COMMON_SPECULATIVE_TYPE_DRAFT;

        // Configure draft model parameters
        spec_params.draft.n_max = params.n_draft > 0 ? params.n_draft : 16;
        spec_params.draft.p_min = 0.75f;
        spec_params.draft.model = wrapper_dft->model;
        spec_params.draft.mparams.path = "draft";

        // Draft context parameters
        spec_params.draft.cparams = llama_context_default_params();
        spec_params.draft.cparams.n_ctx           = llama_n_ctx(wrapper_dft->ctx);
        spec_params.draft.cparams.n_batch         = llama_n_batch(wrapper_dft->ctx);
        spec_params.draft.cparams.n_threads       = llama_n_threads(wrapper_dft->ctx);
        spec_params.draft.cparams.n_threads_batch = llama_n_threads_batch(wrapper_dft->ctx);

        // Initialize speculative sampling
        common_speculative* spec = common_speculative_init(spec_params, wrapper_tgt->ctx);
        if (!spec) {
            g_last_error = "Failed to initialize speculative sampling";
            return nullptr;
        }

        // Create sampling parameters
        common_params_sampling sampling_params;
        // Basic sampling
        sampling_params.seed = params.seed;
        sampling_params.temp = params.temperature;
        sampling_params.top_k = params.top_k;
        sampling_params.top_p = params.top_p;
        sampling_params.min_p = params.min_p;
        sampling_params.typ_p = params.typ_p;
        sampling_params.top_n_sigma = params.top_n_sigma;
        sampling_params.min_keep = params.min_keep;

        // Repetition penalties
        sampling_params.penalty_last_n = params.penalty_last_n;
        sampling_params.penalty_repeat = params.penalty_repeat;
        sampling_params.penalty_freq = params.penalty_freq;
        sampling_params.penalty_present = params.penalty_present;

        // DRY sampling
        sampling_params.dry_multiplier = params.dry_multiplier;
        sampling_params.dry_base = params.dry_base;
        sampling_params.dry_allowed_length = params.dry_allowed_length;
        sampling_params.dry_penalty_last_n = params.dry_penalty_last_n;
        // Convert dry_sequence_breakers from C array to std::vector
        sampling_params.dry_sequence_breakers.clear();
        for (int i = 0; i < params.dry_sequence_breakers_count; i++) {
            sampling_params.dry_sequence_breakers.push_back(std::string(params.dry_sequence_breakers[i]));
        }

        // Dynamic temperature
        sampling_params.dynatemp_range = params.dynatemp_range;
        sampling_params.dynatemp_exponent = params.dynatemp_exponent;

        // XTC sampling
        sampling_params.xtc_probability = params.xtc_probability;
        sampling_params.xtc_threshold = params.xtc_threshold;

        // Mirostat sampling
        sampling_params.mirostat = params.mirostat;
        sampling_params.mirostat_tau = params.mirostat_tau;
        sampling_params.mirostat_eta = params.mirostat_eta;

        // Other parameters
        sampling_params.n_prev = params.n_prev;
        sampling_params.n_probs = params.n_probs;
        sampling_params.ignore_eos = params.ignore_eos;

        // Initialise sampler
        common_sampler* sampler = common_sampler_init(wrapper_tgt->model, sampling_params);
        if (!sampler) {
            common_speculative_free(spec);
            g_last_error = "Failed to initialise sampler";
            return nullptr;
        }

        // Evaluate prompt (all but last token), but only process tokens after the target prefix
        // If target_prefix_len is at or past the last token, we don't need to decode anything
        if (prompt_tokens.size() > 1 && target_prefix_len < (int)prompt_tokens.size() - 1) {
            // Process tokens from target_prefix_len to size - 1
            int tokens_to_process = prompt_tokens.size() - 1 - target_prefix_len;
            int n_batch = llama_n_batch(wrapper_tgt->ctx);

            // Process tokens in chunks that respect n_batch limit
            for (int chunk_start = 0; chunk_start < tokens_to_process; chunk_start += n_batch) {
                int chunk_size = std::min(n_batch, tokens_to_process - chunk_start);
                llama_batch batch = llama_batch_init(chunk_size, 0, 1);
                common_batch_clear(batch);

                // Add tokens for this chunk with explicit positions
                for (int i = 0; i < chunk_size; i++) {
                    int token_idx = target_prefix_len + chunk_start + i;
                    // Only the very last token of the entire prompt needs logits
                    bool needs_logits = (chunk_start + i == tokens_to_process - 1);
                    common_batch_add(batch, prompt_tokens[token_idx], token_idx, { 0 }, needs_logits);
                }

                if (llama_decode(wrapper_tgt->ctx, batch) != 0) {
                    llama_batch_free(batch);
                    common_sampler_free(sampler);
                    common_speculative_free(spec);
                    g_last_error = "Failed to decode prompt";
                    return nullptr;
                }

                llama_batch_free(batch);
            }
        } else if (target_prefix_len == (int)prompt_tokens.size() && prompt_tokens.size() > 1) {
            // Full cache hit - refresh the second-to-last token to ensure determinism
            // This matches the pattern where we decode all but the last token
            llama_batch batch = llama_batch_init(512, 0, 1);
            common_batch_clear(batch);
            common_batch_add(batch, prompt_tokens[prompt_tokens.size() - 2], prompt_tokens.size() - 2, { 0 }, true);

            if (llama_decode(wrapper_tgt->ctx, batch) != 0) {
                if (params.debug) {
                    fprintf(stderr, "WARNING: speculative prompt logit refresh failed\n");
                }
                llama_batch_free(batch);
                common_sampler_free(sampler);
                common_speculative_free(spec);
                g_last_error = "Failed to refresh logits for cached speculative prompt";
                return nullptr;
            }
            llama_batch_free(batch);
        }

        // Generation variables
        std::string result;
        llama_token last_token = prompt_tokens.back();
        llama_tokens prompt_tgt(prompt_tokens.begin(), prompt_tokens.end() - 1);
        int n_past = prompt_tokens.size() - 1;
        int n_predict = params.max_tokens > 0 ? params.max_tokens : 128;

        llama_batch batch_tgt = llama_batch_init(llama_n_batch(wrapper_tgt->ctx), 0, 1);

        // Generation loop
        while (result.length() < (size_t)n_predict) {
            // Generate draft tokens
            llama_tokens draft = common_speculative_draft(spec, spec_params, prompt_tgt, last_token);

            // Prepare batch with last token and draft
            common_batch_clear(batch_tgt);
            common_batch_add(batch_tgt, last_token, n_past, { 0 }, true);

            for (size_t i = 0; i < draft.size(); ++i) {
                common_batch_add(batch_tgt, draft[i], n_past + i + 1, { 0 }, true);
            }

            // Evaluate on target model
            if (llama_decode(wrapper_tgt->ctx, batch_tgt) != 0) {
                if (params.debug) {
                    fprintf(stderr, "WARNING: target decode failed, stopping\n");
                }
                break;
            }

            // Sample and accept tokens
            const auto ids = common_sampler_sample_and_accept_n(sampler, wrapper_tgt->ctx, draft);

            if (ids.empty()) {
                break;
            }

            // Process accepted tokens - track actual count in case of early termination
            size_t tokens_processed = 0;
            bool early_termination = false;

            for (size_t i = 0; i < ids.size(); ++i) {
                const llama_token id = ids[i];

                // Check for EOS
                if (llama_vocab_is_eog(llama_model_get_vocab(wrapper_tgt->model), id)) {
                    early_termination = true;
                    break;
                }

                const std::string token_str = common_token_to_piece(wrapper_tgt->ctx, id);

                // Call callback if provided
                if (params.callback_handle != 0) {
                    if (!goTokenCallback(params.callback_handle, token_str.c_str())) {
                        early_termination = true;
                        break;
                    }
                }

                result += token_str;
                prompt_tgt.push_back(id);
                tokens_processed++;

                // Check stop words
                for (int j = 0; j < params.stop_words_count; j++) {
                    if (result.find(params.stop_words[j]) != std::string::npos) {
                        early_termination = true;
                        goto early_exit;
                    }
                }
            }

early_exit:
            // Update position tracking based on tokens actually processed
            if (early_termination) {
                n_past += tokens_processed;
                if (params.debug) {
                    fprintf(stderr, "DEBUG: Early termination after processing %zu/%zu tokens\n",
                            tokens_processed, ids.size());
                }
            } else {
                n_past += ids.size();
            }

            // Clean up any unaccepted/unprocessed tokens from KV cache
            // This removes everything from position n_past onwards, ensuring the cache
            // only contains tokens we've actually processed and accepted
            llama_memory_seq_rm(llama_get_memory(wrapper_tgt->ctx), 0, n_past, -1);

            // Update last token for next iteration
            if (tokens_processed > 0) {
                // Use the last token we actually processed
                last_token = prompt_tgt[prompt_tgt.size() - 1];
            }

            // Break if early termination
            if (early_termination) {
                break;
            }
        }

        llama_batch_free(batch_tgt);
        common_sampler_free(sampler);
        common_speculative_free(spec);

        // Return allocated string
        char* c_result = (char*)malloc(result.length() + 1);
        if (c_result) {
            memcpy(c_result, result.c_str(), result.length());
            c_result[result.length()] = '\0';
        } else {
            g_last_error = "Failed to allocate memory for result";
        }
        return c_result;

    } catch (const std::exception& e) {
        g_last_error = "Exception during speculative generation: " + std::string(e.what());
        return nullptr;
    }
}

// Simple wrapper that tokenises the prompt and handles prefix caching automatically for both models
char* llama_wrapper_generate_draft(void* ctx_target, void* ctx_draft, llama_wrapper_generate_params params) {
    if (!ctx_target || !ctx_draft) {
        g_last_error = "Target and draft contexts cannot be null";
        return nullptr;
    }

    auto wrapper_tgt = static_cast<llama_wrapper_context_t*>(ctx_target);
    auto wrapper_dft = static_cast<llama_wrapper_context_t*>(ctx_draft);
    if (!wrapper_tgt->ctx) {
        g_last_error = "Target context has been freed";
        return nullptr;
    }
    if (!wrapper_dft->ctx) {
        g_last_error = "Draft context has been freed";
        return nullptr;
    }

    try {
        // Tokenise the prompt
        std::vector<llama_token> prompt_tokens = common_tokenize(wrapper_tgt->ctx, params.prompt, true, true);

        if (prompt_tokens.empty()) {
            g_last_error = "Failed to tokenize prompt";
            return nullptr;
        }

        // Convert to int vector for comparison
        std::vector<int> tokens_int(prompt_tokens.begin(), prompt_tokens.end());

        // Find common prefix for both contexts (only if prefix caching enabled)
        int target_prefix_len = params.enable_prefix_caching
            ? findCommonPrefix(wrapper_tgt->cached_tokens, tokens_int)
            : 0;
        int draft_prefix_len = params.enable_prefix_caching
            ? findCommonPrefix(wrapper_dft->cached_tokens, tokens_int)
            : 0;

        // Update both caches to new token sequence (only if prefix caching enabled)
        if (params.enable_prefix_caching) {
            wrapper_tgt->cached_tokens = tokens_int;
            wrapper_dft->cached_tokens = tokens_int;
        } else {
            wrapper_tgt->cached_tokens.clear();  // Ensure cache is empty when disabled
            wrapper_dft->cached_tokens.clear();
        }

        // Call token-based speculative generation with prefix caching
        return llama_wrapper_generate_draft_with_tokens(ctx_target, ctx_draft, tokens_int.data(), tokens_int.size(), target_prefix_len, draft_prefix_len, params);

    } catch (const std::exception& e) {
        g_last_error = "Exception during speculative generation: " + std::string(e.what());
        return nullptr;
    }
}

int llama_wrapper_tokenize(void* ctx, const char* text, int* tokens, int max_tokens) {
    if (!ctx || !text || !tokens) {
        g_last_error = "Invalid parameters for tokenization";
        return -1;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);

    try {
        std::vector<llama_token> token_vec = common_tokenize(wrapper->ctx, text, true, true);

        int count = std::min((int)token_vec.size(), max_tokens);
        for (int i = 0; i < count; i++) {
            tokens[i] = token_vec[i];
        }

        return count;
    } catch (const std::exception& e) {
        g_last_error = "Exception during tokenization: " + std::string(e.what());
        return -1;
    }
}

// Tokenise with dynamic allocation (C manages memory)
// Caller must free the returned tokens array with llama_wrapper_free_tokens
void llama_wrapper_tokenize_alloc(void* ctx, const char* text, int** tokens, int* count) {
    // Initialise outputs to safe defaults
    if (tokens) *tokens = nullptr;
    if (count) *count = -1;

    if (!ctx || !text || !tokens || !count) {
        g_last_error = "Invalid parameters for tokenization";
        return;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);

    try {
        // Tokenise text (no truncation)
        std::vector<llama_token> token_vec = common_tokenize(wrapper->ctx, text, true, true);

        // Allocate exact size needed
        int n_tokens = token_vec.size();
        int* allocated_tokens = (int*)malloc(n_tokens * sizeof(int));
        if (!allocated_tokens) {
            g_last_error = "Failed to allocate memory for tokens";
            return;
        }

        // Copy tokens from vector to allocated array
        for (int i = 0; i < n_tokens; i++) {
            allocated_tokens[i] = token_vec[i];
        }

        // Return pointer and count
        *tokens = allocated_tokens;
        *count = n_tokens;

    } catch (const std::exception& e) {
        g_last_error = "Exception during tokenization: " + std::string(e.what());
        if (tokens && *tokens) {
            free(*tokens);
            *tokens = nullptr;
        }
        if (count) *count = -1;
    }
}

// Free tokens allocated by llama_wrapper_tokenize_alloc
void llama_wrapper_free_tokens(int* tokens) {
    if (tokens) {
        free(tokens);
    }
}

int llama_wrapper_embeddings(void* ctx, const char* text, float* embeddings, int max_embeddings) {
    if (!ctx || !text || !embeddings) {
        g_last_error = "Invalid parameters for embeddings";
        return -1;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);

    try {
        // Clear KV cache to ensure clean state
        llama_memory_seq_rm(llama_get_memory(wrapper->ctx), 0, -1, -1);

        // Tokenize text
        std::vector<llama_token> tokens = common_tokenize(wrapper->ctx, text, true, true);

        if (tokens.empty()) {
            g_last_error = "Failed to tokenize text for embeddings";
            return -1;
        }

        // Evaluate tokens in chunks that respect n_batch limit
        int n_batch = llama_n_batch(wrapper->ctx);
        int n_tokens = tokens.size();

        for (int i = 0; i < n_tokens; i += n_batch) {
            int chunk_size = std::min(n_batch, n_tokens - i);
            llama_batch batch = llama_batch_init(chunk_size, 0, 1);
            common_batch_clear(batch);

            // Add tokens for this chunk
            for (int j = 0; j < chunk_size; j++) {
                // All tokens need logits for embeddings
                common_batch_add(batch, tokens[i + j], i + j, { 0 }, true);
            }

            if (llama_decode(wrapper->ctx, batch) != 0) {
                llama_batch_free(batch);
                g_last_error = "Failed to decode tokens for embeddings";
                return -1;
            }

            llama_batch_free(batch);
        }

        // Get embeddings from sequence 0 (works for both single and multi-sequence contexts)
        const float* embd = llama_get_embeddings_seq(wrapper->ctx, 0);
        if (!embd) {
            g_last_error = "Failed to get embeddings from context";
            return -1;
        }

        // Copy embeddings
        int n_embd = llama_model_n_embd(wrapper->model);
        int count = std::min(n_embd, max_embeddings);

        memcpy(embeddings, embd, count * sizeof(float));

        return count;
    } catch (const std::exception& e) {
        g_last_error = "Exception during embedding generation: " + std::string(e.what());
        return -1;
    }
}

int llama_wrapper_embeddings_batch(void* ctx, const char** texts, int n_texts, float* embeddings, int n_embd) {
    if (!ctx || !texts || !embeddings || n_texts <= 0 || n_embd <= 0) {
        g_last_error = "Invalid parameters for batch embeddings";
        return -1;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);

    try {
        // Clear KV cache to ensure clean state
        llama_memory_clear(llama_get_memory(wrapper->ctx), true);

        // Tokenize all texts
        std::vector<std::vector<llama_token>> all_tokens;
        all_tokens.reserve(n_texts);

        for (int i = 0; i < n_texts; i++) {
            if (!texts[i]) {
                g_last_error = "Null text in batch at index " + std::to_string(i);
                return -1;
            }
            std::vector<llama_token> tokens = common_tokenize(wrapper->ctx, texts[i], true, true);
            if (tokens.empty()) {
                g_last_error = "Failed to tokenize text at index " + std::to_string(i);
                return -1;
            }
            all_tokens.push_back(std::move(tokens));
        }

        // Get batch size and max sequences
        int n_batch = llama_n_batch(wrapper->ctx);
        int n_seq_max = llama_n_seq_max(wrapper->ctx);

        // Initialize batch
        llama_batch batch = llama_batch_init(n_batch, 0, n_seq_max);

        int embeddings_stored = 0;  // Track how many embeddings we've extracted

        // Process texts in batches
        int s = 0;  // Current sequence ID in batch
        for (int k = 0; k < n_texts; k++) {
            const auto& tokens = all_tokens[k];
            int n_tokens = tokens.size();

            // Check if adding this text would exceed batch size or sequence limit
            if (batch.n_tokens + n_tokens > n_batch || s >= n_seq_max) {
                // Decode current batch
                if (llama_decode(wrapper->ctx, batch) != 0) {
                    llama_batch_free(batch);
                    g_last_error = "Failed to decode batch";
                    return -1;
                }

                // Extract embeddings for all sequences in this batch
                for (int seq = 0; seq < s; seq++) {
                    const float* embd = llama_get_embeddings_seq(wrapper->ctx, seq);
                    if (!embd) {
                        llama_batch_free(batch);
                        g_last_error = "Failed to get embeddings for sequence " + std::to_string(seq);
                        return -1;
                    }
                    // Copy embedding to output buffer
                    memcpy(embeddings + embeddings_stored * n_embd, embd, n_embd * sizeof(float));
                    embeddings_stored++;
                }

                // Clear KV cache for processed sequences before resetting
                for (int seq = 0; seq < s; seq++) {
                    llama_memory_seq_rm(llama_get_memory(wrapper->ctx), seq, -1, -1);
                }

                // Reset for next batch
                s = 0;
                common_batch_clear(batch);
            }

            // Add tokens for this text with unique seq_id
            for (int j = 0; j < n_tokens; j++) {
                // Position is relative to this sequence (starts at 0)
                // All tokens need logits for embeddings
                common_batch_add(batch, tokens[j], j, { s }, true);
            }

            s++;  // Move to next sequence ID
        }

        // Process final batch if there are remaining sequences
        if (s > 0) {
            if (llama_decode(wrapper->ctx, batch) != 0) {
                llama_batch_free(batch);
                g_last_error = "Failed to decode final batch";
                return -1;
            }

            // Extract embeddings for remaining sequences
            for (int seq = 0; seq < s; seq++) {
                const float* embd = llama_get_embeddings_seq(wrapper->ctx, seq);
                if (!embd) {
                    llama_batch_free(batch);
                    g_last_error = "Failed to get embeddings for final sequence " + std::to_string(seq);
                    return -1;
                }
                memcpy(embeddings + embeddings_stored * n_embd, embd, n_embd * sizeof(float));
                embeddings_stored++;
            }
        }

        llama_batch_free(batch);

        // Verify we got all embeddings
        if (embeddings_stored != n_texts) {
            g_last_error = "Embedding count mismatch: expected " + std::to_string(n_texts) +
                          ", got " + std::to_string(embeddings_stored);
            return -1;
        }

        return embeddings_stored;

    } catch (const std::exception& e) {
        g_last_error = "Exception during batch embedding generation: " + std::string(e.what());
        return -1;
    }
}

int llama_wrapper_get_cached_token_count(void* ctx) {
    if (!ctx) {
        g_last_error = "Context cannot be null";
        return -1;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);
    return static_cast<int>(wrapper->cached_tokens.size());
}

// Get the chat template from model metadata
// Returns nullptr if no template is available
const char* llama_wrapper_get_chat_template(void* model) {
    if (!model) {
        return nullptr;
    }

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);

    // Get default chat template (name = nullptr)
    const char* tmpl = llama_model_chat_template(model_wrapper->model, nullptr);

    return tmpl;  // May be nullptr if model has no template
}

// Apply chat template to messages
// Returns allocated string with formatted prompt (caller must free with llama_wrapper_free_result)
// Returns nullptr on error
char* llama_wrapper_apply_chat_template(const char* tmpl, const char** roles, const char** contents, int n_messages, bool add_assistant) {
    if (!tmpl || !roles || !contents || n_messages < 0) {
        g_last_error = "Invalid parameters for chat template application";
        return nullptr;
    }

    try {
        // Build array of llama_chat_message structs
        std::vector<llama_chat_message> messages;
        messages.reserve(n_messages);

        for (int i = 0; i < n_messages; i++) {
            if (!roles[i] || !contents[i]) {
                g_last_error = "Role or content cannot be null";
                return nullptr;
            }
            messages.push_back({roles[i], contents[i]});
        }

        // Start with a reasonable buffer size (8KB)
        std::vector<char> buffer(8192);

        // Try to apply template
        int32_t result_len = llama_chat_apply_template(
            tmpl,
            messages.data(),
            n_messages,
            add_assistant,
            buffer.data(),
            buffer.size()
        );

        // If buffer was too small, resize and retry
        if (result_len > (int32_t)buffer.size()) {
            buffer.resize(result_len);
            result_len = llama_chat_apply_template(
                tmpl,
                messages.data(),
                n_messages,
                add_assistant,
                buffer.data(),
                buffer.size()
            );
        }

        // Check for errors
        if (result_len < 0) {
            g_last_error = "Failed to apply chat template (template detection or application error)";
            return nullptr;
        }

        // Allocate result and copy
        char* c_result = (char*)malloc(result_len + 1);
        if (c_result) {
            memcpy(c_result, buffer.data(), result_len);
            c_result[result_len] = '\0';
        } else {
            g_last_error = "Failed to allocate memory for chat template result";
            return nullptr;
        }

        return c_result;
    } catch (const std::exception& e) {
        g_last_error = "Exception during chat template application: " + std::string(e.what());
        return nullptr;
    }
}

// Parse model output to extract reasoning/thinking content
// Returns NULL on error. Free result with llama_wrapper_free_parsed_message()
llama_wrapper_parsed_message* llama_wrapper_parse_reasoning(
    const char* text,
    bool is_partial,
    llama_wrapper_reasoning_format format,
    int chat_format
) {
    if (!text) {
        g_last_error = "Text cannot be null for reasoning parsing";
        return nullptr;
    }

    try {
        // Configure syntax for parsing
        common_chat_parser_params syntax;
        syntax.format = static_cast<common_chat_format>(chat_format);
        syntax.reasoning_format = static_cast<common_reasoning_format>(format);
        syntax.reasoning_in_content = false;  // Extract to separate field for streaming
        syntax.parse_tool_calls = false;  // Don't need tool parsing for this use case

        // Parse the text
        common_chat_msg msg = common_chat_parse(std::string(text), is_partial, syntax);

        // Allocate result struct
        auto* result = new llama_wrapper_parsed_message;
        result->content = strdup(msg.content.c_str());
        result->reasoning_content = msg.reasoning_content.empty()
            ? nullptr
            : strdup(msg.reasoning_content.c_str());

        return result;
    } catch (const std::exception& e) {
        g_last_error = "Exception during reasoning parsing: " + std::string(e.what());
        return nullptr;
    }
}

void llama_wrapper_free_parsed_message(llama_wrapper_parsed_message* msg) {
    if (!msg) return;

    if (msg->content) {
        free(const_cast<char*>(msg->content));
    }
    if (msg->reasoning_content) {
        free(const_cast<char*>(msg->reasoning_content));
    }
    delete msg;
}

void* llama_wrapper_chat_templates_init(void* model, const char* template_override) {
    if (!model) return nullptr;

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);
    std::string tmpl_override = template_override ? template_override : "";

    auto templates = common_chat_templates_init(model_wrapper->model, tmpl_override);
    return templates.release();  // Transfer ownership
}

void llama_wrapper_chat_templates_free(void* templates) {
    if (!templates) return;
    common_chat_templates_free(static_cast<common_chat_templates*>(templates));
}

int llama_wrapper_chat_templates_get_format(void* templates) {
    if (!templates) return 0;  // COMMON_CHAT_FORMAT_CONTENT_ONLY = 0

    auto tmpl = static_cast<common_chat_templates*>(templates);

    try {
        // Apply with minimal dummy messages just to trigger format detection
        common_chat_templates_inputs inputs;
        inputs.use_jinja = true;
        inputs.add_generation_prompt = true;

        // Create a minimal dummy message to satisfy template application
        common_chat_msg dummy_msg;
        dummy_msg.role = "user";
        dummy_msg.content = "test";  // Non-empty to avoid potential issues
        inputs.messages.push_back(dummy_msg);

        auto params = common_chat_templates_apply(tmpl, inputs);
        return static_cast<int>(params.format);
    } catch (const std::exception& e) {
        // If template application fails, return CONTENT_ONLY as fallback
        g_last_error = "Format detection failed: " + std::string(e.what());
        return 0;  // COMMON_CHAT_FORMAT_CONTENT_ONLY
    }
}

// Get model metadata string value by key
const char* llama_wrapper_model_meta_string(void* model, const char* key) {
    if (!model || !key) return nullptr;

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);

    // Use llama.cpp's metadata API with buffer
    static char buffer[2048];  // Static buffer for metadata strings
    int32_t result = llama_model_meta_val_str(model_wrapper->model, key, buffer, sizeof(buffer));

    if (result < 0) {
        return nullptr;  // Key doesn't exist
    }

    return buffer;
}

// Get count of metadata key-value pairs
int llama_wrapper_model_meta_count(void* model) {
    if (!model) return 0;

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);
    return llama_model_meta_count(model_wrapper->model);
}

// Get number of CUDA devices
int llama_wrapper_get_gpu_count() {
#ifdef GGML_USE_CUDA
    return ggml_backend_cuda_get_device_count();
#else
    return 0;
#endif
}

// Get GPU device information
bool llama_wrapper_get_gpu_info(int device_id, llama_wrapper_gpu_info* info) {
    if (!info) return false;

#ifdef GGML_USE_CUDA
    int count = ggml_backend_cuda_get_device_count();
    if (device_id < 0 || device_id >= count) return false;

    // Get device description
    ggml_backend_cuda_get_device_description(device_id, info->device_name, sizeof(info->device_name));
    info->device_id = device_id;

    // Get memory info
    size_t free_mem, total_mem;
    ggml_backend_cuda_get_device_memory(device_id, &free_mem, &total_mem);
    info->free_memory_mb = free_mem / (1024 * 1024);
    info->total_memory_mb = total_mem / (1024 * 1024);

    return true;
#else
    return false;
#endif
}

// Get runtime information about model and context
void llama_wrapper_get_runtime_info(void* model, void* ctx, const char* kv_cache_type, llama_wrapper_runtime_info* info) {
    if (!model || !info) return;

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);

    // Get layer counts (llama.cpp uses singular "layer" not "layers")
    info->total_layers = llama_model_n_layer(model_wrapper->model);
    // GPU layers loaded is minimum of requested and total layers
    // (can't load more layers than the model has)
    info->gpu_layers = std::min(model_wrapper->n_gpu_layers, info->total_layers);

    if (ctx) {
        auto ctx_wrapper = static_cast<llama_wrapper_context_t*>(ctx);
        info->n_ctx = llama_n_ctx(ctx_wrapper->ctx);
        info->n_batch = llama_n_batch(ctx_wrapper->ctx);

        // Calculate KV cache size properly accounting for GQA/MQA
        // Formula: 2 * n_ctx * (head_dim * n_head_kv) * n_layers * bytes_per_element
        int n_embd = llama_model_n_embd(model_wrapper->model);
        int n_head = llama_model_n_head(model_wrapper->model);
        int n_head_kv = llama_model_n_head_kv(model_wrapper->model);
        int head_dim = n_embd / n_head;

        // Determine element size based on quantization type
        float bytes_per_element = 2.0f;  // Default f16

        if (kv_cache_type) {
            std::string cache_type(kv_cache_type);
            if (cache_type == "f16") {
                bytes_per_element = 2.0f;
            } else if (cache_type == "q8_0") {
                bytes_per_element = 1.125f;  // ~1 byte + overhead
            } else if (cache_type == "q4_0") {
                bytes_per_element = 0.625f;  // ~0.5 bytes + overhead
            }
        }

        // K and V cache: n_ctx * head_dim * n_head_kv * 2 (K+V) * n_layers * element_size
        long long cache_bytes = (long long)info->n_ctx * head_dim * n_head_kv * 2LL * info->total_layers * bytes_per_element;
        info->kv_cache_size_mb = cache_bytes / (1024 * 1024);
    } else {
        // No context - use defaults or zeros
        info->n_ctx = 0;
        info->n_batch = 0;
        info->kv_cache_size_mb = 0;
    }
}

} // extern "C"
