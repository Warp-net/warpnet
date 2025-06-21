package moderation

import (
	"os"
	"runtime"
	"testing"
	"time"
)

const modelPath = "/go/src/github.com/warpnet/llama-2-7b-chat.Q8_0.gguf"

func TestNewLlamaEngineViolationSuccess(t *testing.T) {
	if t.Skipped() {
		return
	}
	if runtime.GOOS != "linux" {
		return
	}

	home := os.Getenv("HOME")
	path := home + modelPath

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	eng, err := NewLlamaEngine(path, runtime.NumCPU())
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
	if t.Skipped() {
		return
	}
	if runtime.GOOS != "linux" {
		return
	}
	home := os.Getenv("HOME")
	path := home + modelPath

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	eng, err := NewLlamaEngine(path, runtime.NumCPU())
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
