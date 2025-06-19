package moderation

import (
	"errors"
	llama "github.com/Warp-net/warpnet/core/moderation/binding/go-llama.cpp"
	"strings"
)

type Engine interface {
	Moderate(content string) (bool, string, error)
	Close()
}

type LlamaEngine struct {
	llm  *llama.LLama
	opts []llama.PredictOption
}

func NewLlamaEngine(modelPath string, threads int) (*LlamaEngine, error) {
	llm, err := llama.New(modelPath,
		llama.SetContext(512),
		llama.SetMMap(true),
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
	}

	lle := &LlamaEngine{llm: llm, opts: opts}
	_, err = lle.llm.Predict("Hello!", lle.opts...) // warm up!
	return lle, err
}

func (e *LlamaEngine) Moderate(content string) (bool, string, error) {
	prompt := generatePrompt(content)

	resp, err := e.llm.Predict(prompt, e.opts...)
	if err != nil {
		return true, "", err
	}

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

func (e *LlamaEngine) Close() {
	e.llm.Free()
}
