package keepalive

import (
	"context"
	"github.com/Warp-net/keepalive"
	"runtime"
	"sync"
	"time"
)

var (
	inhibitorSingleton keepalive.Inhibitor
	holdSingleton      keepalive.Hold
	holdMx             sync.Mutex
)

func init() {
	inhibitorSingleton = keepalive.New()
}

func Acquire() error {
	holdMx.Lock()
	defer holdMx.Unlock()
	if holdSingleton != nil {
		return nil
	}
	keepaliveCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	hold, err := inhibitorSingleton.Acquire(keepaliveCtx, keepalive.Options{
		KeepDisplay: runtime.GOOS == "linux",
		Reason:      "serving paired devices",
	})
	if err != nil {
		return err
	}
	holdSingleton = hold
	return nil
}

func Release() error {
	holdMx.Lock()
	defer holdMx.Unlock()
	if holdSingleton == nil {
		return nil
	}
	err := holdSingleton.Release()
	holdSingleton = nil
	return err
}
