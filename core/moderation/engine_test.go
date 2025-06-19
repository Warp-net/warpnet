package moderation

import (
	"runtime"
	"testing"
	"time"
)

const modelPath = "/home/vadim/go/src/github.com/warpnet/llama-2-7b-chat.Q8_0.gguf"

func TestNewLlamaEngine_Success(t *testing.T) {
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

//func TestNewLlamaEngine_Fail(t *testing.T) {
//	eng, err := NewLlamaEngine(modelPath, runtime.NumCPU())
//	if err != nil {
//		t.Fatal(err)
//	}
//	defer eng.Close()
//
//	now := time.Now()
//	ok, reason, err := eng.Moderate("I'm selling automatic transmission and moccasin.")
//	if err != nil {
//		t.Fatal(err)
//	}
//	t.Logf("elapsed: %s", time.Since(now))
//
//	t.Logf("result: ok: %t, reason: %s", ok, reason)
//
//	if !ok {
//		t.Fatal("moderation must succeed:", reason)
//	}
//}
