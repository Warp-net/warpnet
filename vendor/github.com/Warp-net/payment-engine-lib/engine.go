package paymentengine

import (
	"errors"
	"runtime"
)

var ErrUnsupported = errors.New("payment engine: no binary embedded for " + runtime.GOOS + "/" + runtime.GOARCH)
