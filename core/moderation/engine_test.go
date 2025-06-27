package moderation

import (
	"os"
	"runtime"
	"testing"
	"time"
)

const modelPath = "/tmp/llama-2-7b-chat.Q8_0.gguf"

func TestNewLlamaEngineViolationSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip()
		return
	}
	if runtime.GOOS != "linux" {
		t.Skip()
		return
	}

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skip()
		return
	}

	eng, err := NewLlamaEngine(modelPath, runtime.NumCPU())
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	now := time.Now()
	ok, reason, err := eng.Moderate("I'm selling AK-47 and cocaine")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("elapsed: %s", time.Since(now))

	t.Logf("result: ok: %t, reason: %s", ok, reason)

	if ok {
		t.Fatal("moderation must fail", reason)
	}
}

func TestNewLlamaEngineNoViolationSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip()
		return
	}
	if runtime.GOOS != "linux" {
		t.Skip()
		return
	}
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skip()
		return
	}

	eng, err := NewLlamaEngine(modelPath, runtime.NumCPU())
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	now := time.Now()
	ok, reason, err := eng.Moderate("I'm selling automatic transmission and moccasin.")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("elapsed: %s", time.Since(now))

	t.Logf("result: ok: %t, reason: %s", ok, reason)

	if !ok {
		t.Fatal("moderation must succeed:", reason)
	}
}
