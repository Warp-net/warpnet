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

// Acquire holds the machine awake; it does nothing while a hold is already
// taken, so pairing a second device does not leak the first hold.
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

// Release drops the hold, if one was taken since the node started.
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
