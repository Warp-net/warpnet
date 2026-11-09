package keepalive

import (
	"context"
	"github.com/Warp-net/keepalive"
	"runtime"
	"time"
)

var (
	inhibitorSingleton keepalive.Inhibitor
	holdSingleton      keepalive.Hold
)

func init() {
	inhibitorSingleton = keepalive.New()
}

func Acquire() (err error) {
	keepaliveCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	holdSingleton, err = inhibitorSingleton.Acquire(keepaliveCtx, keepalive.Options{
		KeepDisplay: runtime.GOOS == "linux",
		Reason:      "serving paired devices",
	})
	if err != nil {
		holdSingleton = holdNoOp{}
	}
	return err
}

func Release() (err error) {
	return holdSingleton.Release()
}

type holdNoOp struct{}

func (h holdNoOp) Release() error {
	return nil
}

func (h holdNoOp) Describe() string {
	return "no-op"
}
