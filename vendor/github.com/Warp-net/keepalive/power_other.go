//go:build !darwin && !linux && !windows

package keepalive

import (
	"context"
	"errors"
	"runtime"
)

func newPlatform() Inhibitor { return unsupportedInhibitor{} }

func platformMechanisms() []Mechanism {
	return []Mechanism{{Name: "none", Detail: "sleep prevention is not supported on " + runtime.GOOS}}
}

type unsupportedInhibitor struct{}

func (unsupportedInhibitor) Name() string { return "unsupported" }

func (unsupportedInhibitor) Acquire(context.Context, Options) (Hold, error) {
	return nil, &Error{Err: errors.New("sleep prevention is not supported on " + runtime.GOOS), Hint: "keepalive supports macOS, Linux and Windows"}
}
