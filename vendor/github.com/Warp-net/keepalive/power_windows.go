package keepalive

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Power requests (PowerCreateRequest/PowerSetRequest) are kernel handles:
// they are not tied to a thread, show up in `powercfg /requests`, and the
// kernel drops them when the process exits and its handles close.

const (
	powerRequestContextVersion      = 0   // POWER_REQUEST_CONTEXT_VERSION
	powerRequestContextSimpleString = 0x1 // POWER_REQUEST_CONTEXT_SIMPLE_STRING

	// POWER_REQUEST_TYPE
	powerRequestDisplayRequired = 0
	powerRequestSystemRequired  = 1
)

// reasonContext mirrors REASON_CONTEXT. The union's largest member is
// Detailed {HMODULE; ULONG; ULONG; LPWSTR*}; with the SIMPLE_STRING flag only
// its first word, SimpleReasonString, is read. The padding keeps the size
// identical to the C struct (32 bytes on 64-bit, 24 on 32-bit).
type reasonContext struct {
	Version            uint32
	Flags              uint32
	SimpleReasonString *uint16
	_                  uint32
	_                  uint32
	_                  uintptr
}

var (
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procPowerCreateRequest = kernel32.NewProc("PowerCreateRequest")
	procPowerSetRequest    = kernel32.NewProc("PowerSetRequest")
	procPowerClearRequest  = kernel32.NewProc("PowerClearRequest")
)

func newPlatform() Inhibitor { return powerRequestInhibitor{} }

func platformMechanisms() []Mechanism {
	for _, p := range []*windows.LazyProc{procPowerCreateRequest, procPowerSetRequest, procPowerClearRequest} {
		if err := p.Find(); err != nil {
			return []Mechanism{{Name: "PowerCreateRequest", Detail: err.Error()}}
		}
	}
	return []Mechanism{{Name: "PowerCreateRequest", Available: true, Detail: "kernel32 power requests"}}
}

type powerRequestInhibitor struct{}

func (powerRequestInhibitor) Name() string { return "PowerCreateRequest" }

func (powerRequestInhibitor) Acquire(ctx context.Context, o Options) (Hold, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	const hint = "Windows refused the power request; check `powercfg /requests`"
	reason, err := windows.UTF16PtrFromString(label(o))
	if err != nil {
		return nil, err
	}
	rc := reasonContext{Version: powerRequestContextVersion, Flags: powerRequestContextSimpleString, SimpleReasonString: reason}
	r, _, callErr := procPowerCreateRequest.Call(uintptr(unsafe.Pointer(&rc)))
	runtime.KeepAlive(&rc)
	if windows.Handle(r) == windows.InvalidHandle || r == 0 {
		return nil, &Error{Err: fmt.Errorf("PowerCreateRequest: %w", callErr), Hint: hint}
	}
	h := &powerRequestHold{handle: windows.Handle(r)}
	types := []uintptr{powerRequestSystemRequired}
	if o.KeepDisplay {
		// DisplayRequired alone does not keep the system awake.
		types = append(types, powerRequestDisplayRequired)
	}
	for _, t := range types {
		if ok, _, callErr := procPowerSetRequest.Call(uintptr(h.handle), t); ok == 0 {
			h.Release()
			return nil, &Error{Err: fmt.Errorf("PowerSetRequest(%s): %w", requestName(t), callErr), Hint: hint}
		}
		h.set = append(h.set, t)
	}
	slog.Debug("power: acquired", "mechanism", h.Describe(), "reason", o.Reason)
	return h, nil
}

func requestName(t uintptr) string {
	if t == powerRequestDisplayRequired {
		return "DisplayRequired"
	}
	return "SystemRequired"
}

type powerRequestHold struct {
	once   sync.Once
	handle windows.Handle
	set    []uintptr
}

func (h *powerRequestHold) Release() error {
	var err error
	h.once.Do(func() {
		var errs []error
		for _, t := range h.set {
			if ok, _, callErr := procPowerClearRequest.Call(uintptr(h.handle), t); ok == 0 {
				errs = append(errs, fmt.Errorf("PowerClearRequest(%s): %w", requestName(t), callErr))
			}
		}
		if cerr := windows.CloseHandle(h.handle); cerr != nil {
			errs = append(errs, fmt.Errorf("CloseHandle: %w", cerr))
		}
		err = errors.Join(errs...)
		slog.Debug("power: released", "mechanism", h.Describe(), "err", err)
	})
	return err
}

func (h *powerRequestHold) Describe() string {
	names := make([]string, len(h.set))
	for i, t := range h.set {
		names[i] = requestName(t)
	}
	return "PowerRequest(" + strings.Join(names, ", ") + ")"
}
