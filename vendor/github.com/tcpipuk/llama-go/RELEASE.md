# Release process

This document outlines how to create new releases of llama-go that maintain compatibility with
upstream llama.cpp development.

## Overview

Rather than distributing pre-built binaries, this fork tracks specific llama.cpp versions so Go
developers can build exactly what they need. Each release corresponds to a tested, compatible
upstream version - no guesswork about which llama.cpp features are available.

## Release workflow

### 1. Monitor upstream releases

Check [llama.cpp releases](https://github.com/ggml-org/llama.cpp/releases) for new stable releases.
Focus on releases that introduce API changes, performance improvements, or new model support.

### 2. Clean existing build artifacts

Before updating, ensure a clean build environment:

```bash
# Check existing artifacts
ls -la *.a *.so 2>/dev/null

# Clean all build artifacts using Docker for consistent environment
docker run --rm -v $(pwd):/workspace -w /workspace git.tomfos.tr/tom/llama-go:build-cuda \
  "make clean"

# Verify cleanup - should return no results
ls -la *.a *.so 2>/dev/null
ls -la llama.cpp/*.o 2>/dev/null
```

**Verification**: Ensure no `.a`, `.so`, or `.o` files remain before proceeding.

### 3. Update the submodule

Update the llama.cpp submodule to the target release:

```bash
# Check submodule status first
cd llama.cpp
git status  # Should be clean before proceeding

# Fetch and checkout target release
git fetch origin
git checkout <target-tag>  # e.g. b6615

# Verify correct tag
git describe --tags  # Should show exact tag

# Ensure clean state (no modifications)
git status  # Should show "HEAD detached at <tag>" with no changes

cd ..

# Check diff before staging to prevent "-dirty" commits
git diff llama.cpp  # Should show clean pointer update
git submodule status  # Should show no + prefix (clean)

# Stage the submodule update
git add llama.cpp

# Verify staged change
git diff --cached  # Should show clean submodule pointer update
```

**Verification**: The submodule must be at the exact tag with no local modifications.

### 4. Test compatibility

Build and test the updated bindings:

```bash
# Verify no existing build artifacts
ls -la libbinding.a 2>/dev/null  # Should return nothing

# Build with Docker for consistent environment (expect 20-25 minutes with FA_ALL_QUANTS enabled).
# Default linkage is static (single-binary friendly); no LD_LIBRARY_PATH gymnastics needed.
docker run --rm -v $(pwd):/workspace -w /workspace git.tomfos.tr/tom/llama-go:build-cuda \
  "BUILD_TYPE=cublas CUDA_ARCHITECTURES=86 make libbinding.a"

# Verify build created all artifacts (static mode -> .a files in workspace root)
ls -la libbinding.a libllama-common.a libllama-common-base.a libllama.a libggml*.a
ls -lat *.a | head -10  # Check timestamps confirm fresh build

# Download test models if not present
ls -la Qwen3-0.6B-Q8_0.gguf 2>/dev/null || \
  wget -q https://huggingface.co/Qwen/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf

# Download embedding test model for embedding functionality
ls -la Qwen3-Embedding-0.6B-Q8_0.gguf 2>/dev/null || \
  wget -q https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF/resolve/main/Qwen3-Embedding-0.6B-Q8_0.gguf

# Test inference pipeline. -tags cublas pulls in the CUDA backend's CGO LDFLAGS.
docker run --rm --gpus all -v $(pwd):/workspace -w /workspace git.tomfos.tr/tom/llama-go:build-cuda \
  "go run -tags cublas ./examples/simple -m Qwen3-0.6B-Q8_0.gguf -p 'Hello world' -n 50"

# Test embedding functionality
docker run --rm --gpus all -v $(pwd):/workspace -w /workspace git.tomfos.tr/tom/llama-go:build-cuda \
  "go run -tags cublas ./examples/embedding -m Qwen3-Embedding-0.6B-Q8_0.gguf -t 'Hello world'"

# Run test suite with test models
# TEST_MODEL is used by speculative decoding tests
# TEST_CHAT_MODEL is used by generation, streaming, chat, and stop word tests
# TEST_EMBEDDING_MODEL is used by embedding tests
# Limit to 8 CPUs to avoid monopolising shared server resources
docker run --rm --gpus all --cpus=8 -v $(pwd):/workspace -w /workspace \
  -e TEST_MODEL=Qwen3-0.6B-Q8_0.gguf \
  -e TEST_CHAT_MODEL=Qwen3-0.6B-Q8_0.gguf \
  -e TEST_EMBEDDING_MODEL=Qwen3-Embedding-0.6B-Q8_0.gguf \
  -e LLAMA_LOG=error \
  git.tomfos.tr/tom/llama-go:build-cuda \
  "go run github.com/onsi/ginkgo/v2/ginkgo -tags cublas --flake-attempts 3 -v ./..."
```

#### Validating shared linkage (release-time only)

The default static path is exercised every PR via CI. The opt-in shared path
should be verified before tagging a release, since it isn't on the per-PR loop.

```bash
# Clean rebuild in shared mode (full 20-25 min — cmake reconfigure required)
docker run --rm -v $(pwd):/workspace -w /workspace git.tomfos.tr/tom/llama-go:build-cuda \
  "make clean && BUILD_TYPE=cublas CUDA_ARCHITECTURES=86 BUILD_LINKAGE=shared make libbinding.a"

# Should now have .so files instead of .a files in the workspace
ls -la *.so

# Run the suite with the shared_lib build tag
docker run --rm --gpus all --cpus=8 -v $(pwd):/workspace -w /workspace \
  -e TEST_MODEL=Qwen3-0.6B-Q8_0.gguf \
  -e TEST_CHAT_MODEL=Qwen3-0.6B-Q8_0.gguf \
  -e TEST_EMBEDDING_MODEL=Qwen3-Embedding-0.6B-Q8_0.gguf \
  -e LLAMA_LOG=error \
  git.tomfos.tr/tom/llama-go:build-cuda \
  "go run github.com/onsi/ginkgo/v2/ginkgo -tags 'cublas shared_lib' --flake-attempts 3 -v ./..."
```

**Verification**: All commands must complete successfully with expected output before proceeding.

**Note**: The test suite includes a GPU test that validates GPU fallback behaviour (graceful
degradation to CPU when no GPU is available). For actual GPU acceleration testing, see the GPU
testing section below.

### GPU Acceleration Testing (Optional)

For projects requiring GPU acceleration, test CUDA support:

#### Building with CUDA support

1. Build using the CUDA Docker container:

   ```bash
   docker build -f Dockerfile.cuda -t go-llama-cuda .

   # Build for specific GPU architecture (e.g. RTX 3090)
   docker run --rm --gpus all -v $(pwd):/workspace go-llama-cuda \
     bash -c "CUDA_ARCHITECTURES=86 BUILD_TYPE=cublas make libbinding.a"
   ```

2. Verify CUDA libraries are built:

   ```bash
   ls -la *.so | grep ggml
   strings libggml.so | grep -i cuda | head -5  # Should show cuda symbols
   ```

3. Test GPU acceleration with Go 1.26+:

   ```bash
   docker run --rm --gpus all --cpus=8 -v $(pwd):/workspace go-llama-cuda \
     bash -c "wget -q https://go.dev/dl/go1.26.3.linux-amd64.tar.gz && \
              tar -C /usr/local -xzf go1.26.3.linux-amd64.tar.gz && \
              export PATH=/usr/local/go/bin:\$PATH && \
              cd /workspace && \
              LIBRARY_PATH=/workspace C_INCLUDE_PATH=/workspace \
              TEST_MODEL=Qwen3-0.6B-Q8_0.gguf \
              go run github.com/onsi/ginkgo/v2/ginkgo -tags cublas --label-filter='gpu' -v ./..."
   ```

**Expected output**:

- "found 1 CUDA devices: Device 0: NVIDIA GeForce RTX 3090"
- "offloaded X/Y layers to GPU"
- "CUDA0 model buffer size = XXX MiB"

#### Common GPU architectures

| GPU Series        | Architecture | Code |
| ----------------- | ------------ | ---- |
| GTX 1000 (Pascal) | 6.1          | 61   |
| RTX 2000 (Turing) | 7.5          | 75   |
| RTX 3000 (Ampere) | 8.6          | 86   |
| RTX 4000 (Ada)    | 8.9          | 89   |
| A100 (Ampere)     | 8.0          | 80   |

Build for your specific GPU for faster compilation and smaller binaries:

```bash
CUDA_ARCHITECTURES=86 BUILD_TYPE=cublas make libbinding.a  # RTX 3090
```

### 5. When tests fail

Tests should be deterministic, but the environment isn't. Before diving into code fixes:

**Check for resource pressure first**: Tests defaulting to model native context sizes (often 40K+)
can cause cumulative VRAM exhaustion. Look for OOM errors or crashes late in the test run. Fix by
ensuring tests use explicit `WithContext(2048)` unless specifically testing large contexts. GPU
driver issues or CUDA version mismatches can also cause failures - check `nvidia-smi` for errors.

**Study llama.cpp changes systematically**: When tests fail due to actual API changes, comparing the
git diff between tags (`git diff <old-tag>..<new-tag>`) is the most reliable approach. It's heavy
systematic work, especially with many commits, but llama.cpp has contributors spanning mobile
devices to mainframes - occasional API adjustments are natural. Check `include/llama.h` and
`common/common.h` for struct changes, function signatures, and new fields. Compare upstream examples
to see correct patterns for new features.

Common issues: uninitialised struct fields (ABI breaks), stricter validation, changed behaviour, or
new cleanup requirements (e.g. KV cache management). Fix `wrapper.cpp` or `wrapper.h` as needed,
rebuild, and retest.

### 6. Commit and tag the release

Once compatibility is confirmed, commit with a clear 2-3 paragraph description explaining what
changed and why, then create an annotated tag.

**Commit message example**:

```bash
git commit -m "$(cat <<'EOF'
feat(deps): update llama.cpp to b6709 with compatibility fixes

Update llama.cpp submodule from b6615 to b6709 (94 commits). Fix critical ABI
issue by initialising new no_host field in llama_model_params. Implement proper
KV cache cleanup in speculative decoding using llama_memory_seq_rm to remove
unaccepted tokens after each batch.

Update BOS token test to handle models without add_bos_token metadata. All
speculative decoding tests pass.

Upstream release: https://github.com/ggml-org/llama.cpp/releases/tag/b6709
EOF
)"
```

**Tag message**: Check recent tags first for style (`git tag -n99 llama.cpp-b6724`), then
highlight YOUR features in "Major features since..." (3-5 bullets) followed by brief upstream
summary (1 line). First release uses "First tagged release with..." instead.

**Subsequent releases** (most common):

```bash
git tag -a llama.cpp-<tag> -m "$(cat <<'EOF'
Update to llama.cpp b6780

Upstream release: https://github.com/ggml-org/llama.cpp/releases/tag/b6780

Major features since llama.cpp-b6724:
- YOUR new feature with brief description
- YOUR improvements or fixes
- Configuration/test changes

This release includes llama.cpp improvements: [brief 1-line summary].
EOF
)"
```

**First release**:

```bash
git tag -a llama.cpp-b6709 -m "$(cat <<'EOF'
Update to llama.cpp b6709

Upstream release: https://github.com/ggml-org/llama.cpp/releases/tag/b6709

First tagged release with production-ready features:
- Major capability 1
- Major capability 2
- Major capability 3
EOF
)"
```

```bash
git push origin main
git push origin llama.cpp-<tag>
```

## Why no pre-built binaries

Unlike traditional Go libraries, this project doesn't distribute pre-built static libraries. The
reason is simple: there are too many build variants to support effectively.

Different users need different configurations - CPU-only for development, CUDA for NVIDIA cards,
ROCm for AMD, Metal for Apple Silicon, or OpenBLAS for optimised CPU inference. Each acceleration
backend requires different compiler flags, hardware-specific SDKs, and runtime dependencies that
vary by system.

Instead, applications using this library build their own variants as part of their deployment
pipeline. This lets them choose appropriate acceleration for their hardware, include the right
runtime dependencies, and cache builds for their specific configuration.

## Versioning convention

Tags follow the format `llama.cpp-{upstream-tag}` to clearly indicate compatibility:

- `llama.cpp-b6603` - Compatible with llama.cpp tag b6603
- `llama.cpp-b6580` - Compatible with llama.cpp tag b6580

This makes it easy for consumers to choose compatible versions, understand which llama.cpp features
are available, and track upstream development without confusion.

## Testing checklist

Before tagging a release, verify:

- [ ] All existing build artifacts removed via `make clean`
- [ ] No `.a`, `.so`, or `.o` files present before build
- [ ] Submodule updated to target llama.cpp release
- [ ] Submodule has no local modifications (clean checkout)
- [ ] Library builds successfully with Docker containers (default static linkage)
- [ ] All expected artifacts created with fresh timestamps
- [ ] Example program loads test model without errors
- [ ] Text generation produces reasonable output
- [ ] Static-mode test suite passes without failures (`-tags cublas`)
- [ ] **Shared-mode test suite passes** (`BUILD_LINKAGE=shared` build,
      `-tags 'cublas shared_lib'`) — opt-in path verified before tag
- [ ] No obvious API compatibility warnings or errors
- [ ] Commit shows clean submodule pointer update (no "-dirty" suffix)
- [ ] Commit includes clear description of changes made
