//go:build llama
// +build llama

package moderation

import (
	"errors"
	"fmt"
	llama "github.com/Warp-net/warpnet/core/moderation/binding/go-llama.cpp"
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

type Engine interface {
	Moderate(content string) (bool, string, error)
	Close()
}

type llamaEngine struct {
	llm  *llama.LLama
	opts []llama.PredictOption
}

func NewLlamaEngine(modelPath string, threads int) (_ *llamaEngine, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
		}
	}()
	if modelPath == "" {
		return nil, errors.New("model path is required")
	}
	llm, err := llama.New(
		modelPath,
		llama.SetContext(512),
		llama.SetMMap(true),
		llama.EnabelLowVRAM,
	)
	if err != nil {
		return nil, err
	}

	opts := []llama.PredictOption{
		llama.SetThreads(threads),
		llama.SetTokens(32),
		llama.SetTopP(0.9),
		llama.SetTemperature(0.0),
		llama.SetSeed(42),
		llama.SetMlock(false),
	}

	lle := &llamaEngine{llm: llm, opts: opts}
	return lle, nil
}

func (e *llamaEngine) Moderate(content string) (bool, string, error) {
	prompt := generatePrompt(content)

	now := time.Now()
	resp, err := e.llm.Predict(prompt, e.opts...)
	if err != nil {
		return true, "", err
	}
	elapsed := time.Since(now)
	log.Infof("moderation: elapsed %s", elapsed.String())

	out := strings.ToLower(strings.TrimSpace(resp))

	switch {
	case strings.HasPrefix(out, "no"):
		return true, "", nil
	case strings.HasPrefix(out, "yes"):
		reason := strings.TrimSpace(strings.TrimPrefix(out, "yes"))
		reason = strings.Trim(reason, ",.:;- \"\n")
		reason = strings.ReplaceAll(reason, "\n", "")
		return false, reason, nil
	default:
		return true, "", errors.New("unrecognized LLM output: " + out)
	}
}

func (e *llamaEngine) Close() {
	e.llm.Free()
}
