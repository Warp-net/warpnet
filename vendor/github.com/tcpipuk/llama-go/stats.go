package llama

/*
#include "wrapper.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"strings"
	"unsafe"
)

// GPUInfo contains information about a CUDA GPU device.
type GPUInfo struct {
	DeviceID      int    // CUDA device ID
	DeviceName    string // GPU model name (e.g., "NVIDIA GeForce RTX 3090")
	FreeMemoryMB  int    // Available VRAM in MB
	TotalMemoryMB int    // Total VRAM in MB
}

// ModelMetadata contains model information from GGUF metadata.
type ModelMetadata struct {
	Architecture string // Model architecture (e.g., "qwen3", "llama")
	Name         string // Full model name
	Basename     string // Base model name
	QuantizedBy  string // Who quantized the model
	SizeLabel    string // Model size (e.g., "8B", "70B")
	RepoURL      string // Hugging Face repo URL
}

// RuntimeInfo contains current runtime configuration and resource usage.
type RuntimeInfo struct {
	ContextSize     int    // Context window size in tokens
	BatchSize       int    // Batch processing size
	KVCacheType     string // KV cache quantization type ("f16", "q8_0", "q4_0")
	KVCacheSizeMB   int    // Estimated KV cache memory usage in MB
	GPULayersLoaded int    // Number of layers offloaded to GPU
	TotalLayers     int    // Total number of layers in model
}

// ModelStats contains comprehensive model statistics and metadata.
//
// This includes GPU information, model metadata from GGUF, and runtime
// configuration. Use Model.Stats() to retrieve these statistics.
type ModelStats struct {
	GPUs     []GPUInfo     // Information about available CUDA GPUs
	Metadata ModelMetadata // Model metadata from GGUF file
	Runtime  RuntimeInfo   // Runtime configuration and resource usage
}

// Stats returns comprehensive statistics about the model and runtime environment.
//
// This includes:
//   - GPU device information (name, VRAM)
//   - Model metadata from GGUF (architecture, name, size, etc.)
//   - Runtime configuration (context size, batch size, KV cache)
//
// The returned information is useful for:
//   - Displaying model details to users
//   - Debugging configuration issues
//   - Monitoring resource usage
//
// Example:
//
//	stats, err := model.Stats()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(stats)
func (m *Model) Stats() (*ModelStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("model is closed")
	}

	stats := &ModelStats{}

	// Get GPU information
	gpuCount := int(C.llama_wrapper_get_gpu_count())
	stats.GPUs = make([]GPUInfo, 0, gpuCount)

	for i := 0; i < gpuCount; i++ {
		var cInfo C.llama_wrapper_gpu_info
		if C.llama_wrapper_get_gpu_info(C.int(i), &cInfo) {
			stats.GPUs = append(stats.GPUs, GPUInfo{
				DeviceID:      int(cInfo.device_id),
				DeviceName:    C.GoString(&cInfo.device_name[0]),
				FreeMemoryMB:  int(cInfo.free_memory_mb),
				TotalMemoryMB: int(cInfo.total_memory_mb),
			})
		}
	}

	// Get model metadata from GGUF
	stats.Metadata = ModelMetadata{
		Architecture: m.getMetaString("general.architecture"),
		Name:         m.getMetaString("general.name"),
		Basename:     m.getMetaString("general.basename"),
		QuantizedBy:  m.getMetaString("general.quantized_by"),
		SizeLabel:    m.getMetaString("general.size_label"),
		RepoURL:      m.getMetaString("general.repo_url"),
	}

	// Note: Runtime information (context size, batch size, KV cache type) is
	// context-specific and should be obtained from Context instances, not Model.
	// The Runtime field in ModelStats will be zero-valued.

	return stats, nil
}

// getMetaString retrieves a string value from model metadata.
func (m *Model) getMetaString(key string) string {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	cValue := C.llama_wrapper_model_meta_string(m.modelPtr, cKey)
	if cValue == nil {
		return ""
	}

	return C.GoString(cValue)
}

// String returns a formatted summary of model statistics.
//
// The output includes GPU information, model details, and runtime configuration
// in a human-readable format suitable for display.
//
// Example output:
//
//	=== Model Statistics ===
//
//	GPU Devices:
//	  GPU 0: NVIDIA GeForce RTX 3090
//	    VRAM: 23733 MB free / 24576 MB total
//
//	Model Details:
//	  Name: DeepSeek-R1-0528-Qwen3-8B
//	  Architecture: qwen3 (8B)
//	  Quantized by: Unsloth
//	  Repository: https://huggingface.co/unsloth
//
//	Runtime Configuration:
//	  Context: 131,072 tokens | Batch: 512 tokens
//	  KV Cache: q8_0 (9,216 MB)
//	  GPU Layers: 28/28
func (s *ModelStats) String() string {
	var b strings.Builder

	b.WriteString("=== Model Statistics ===\n\n")

	// GPU information
	if len(s.GPUs) > 0 {
		b.WriteString("GPU Devices:\n")
		for _, gpu := range s.GPUs {
			fmt.Fprintf(&b, "  GPU %d: %s\n", gpu.DeviceID, gpu.DeviceName)
			fmt.Fprintf(&b, "    VRAM: %d MB free / %d MB total\n", gpu.FreeMemoryMB, gpu.TotalMemoryMB)
		}
		b.WriteString("\n")
	}

	// Model metadata
	b.WriteString("Model Details:\n")
	if s.Metadata.Name != "" {
		fmt.Fprintf(&b, "  Name: %s\n", s.Metadata.Name)
	}
	if s.Metadata.Architecture != "" {
		arch := s.Metadata.Architecture
		if s.Metadata.SizeLabel != "" {
			arch += " (" + s.Metadata.SizeLabel + ")"
		}
		fmt.Fprintf(&b, "  Architecture: %s\n", arch)
	}
	if s.Metadata.QuantizedBy != "" {
		fmt.Fprintf(&b, "  Quantized by: %s\n", s.Metadata.QuantizedBy)
	}
	if s.Metadata.RepoURL != "" {
		fmt.Fprintf(&b, "  Repository: %s\n", s.Metadata.RepoURL)
	}
	b.WriteString("\n")

	// Runtime configuration
	b.WriteString("Runtime Configuration:\n")
	fmt.Fprintf(&b, "  Context: %s tokens | Batch: %d tokens\n",
		formatNumber(s.Runtime.ContextSize), s.Runtime.BatchSize)
	fmt.Fprintf(&b, "  KV Cache: %s (%s MB)\n",
		s.Runtime.KVCacheType, formatNumber(s.Runtime.KVCacheSizeMB))
	fmt.Fprintf(&b, "  GPU Layers: %d/%d\n",
		s.Runtime.GPULayersLoaded, s.Runtime.TotalLayers)

	return b.String()
}

// formatNumber formats an integer with thousand separators for readability.
func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	// Simple thousand separator implementation
	s := fmt.Sprintf("%d", n)
	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}
