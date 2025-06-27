package moderation

import (
	"errors"
	"fmt"
	llama "github.com/Warp-net/warpnet/core/moderation/binding/go-llama.cpp"
	"strings"
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

	lle := &llamaEngine{llm: llm, opts: opts}
	prompt := generatePrompt("White tea is better than black")
	_, err = lle.llm.Predict(prompt, lle.opts...) // warm up!
	return lle, err
}

type (
	ModerationResult bool
	ModerationReason = string
)

const (
	OK      ModerationResult = true
	FAILURE ModerationResult = false
)

func (e *llamaEngine) Moderate(content string) (ModerationResult, ModerationReason, error) {
	prompt := generatePrompt(content)

	resp, err := e.llm.Predict(prompt, e.opts...)
	if err != nil {
		return true, "", err
	}

	out := strings.ToLower(strings.TrimSpace(resp))

	switch {
	case strings.HasPrefix(out, "no"):
		return OK, "", nil
	case strings.HasPrefix(out, "yes"):
		reason := strings.TrimSpace(strings.TrimPrefix(out, "yes"))
		reason = strings.Trim(reason, ",.:;- \"\n")
		reason = strings.ReplaceAll(reason, "\n", "")
		return FAILURE, reason, nil
	default:
		return OK, "", errors.New("unrecognized LLM output: " + out)
	}
}

func (e *llamaEngine) Close() {
	e.llm.Free()
}
