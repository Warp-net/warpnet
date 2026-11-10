//go:build darwin && cgo

package keepalive

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <stdlib.h>
#include <IOKit/pwr_mgt/IOPMLib.h>

static IOReturn ka_assertion_create(const char *type, const char *name, IOPMAssertionID *id) {
	CFStringRef t = CFStringCreateWithCString(kCFAllocatorDefault, type, kCFStringEncodingUTF8);
	CFStringRef n = CFStringCreateWithCString(kCFAllocatorDefault, name, kCFStringEncodingUTF8);
	IOReturn r = kIOReturnNoMemory;
	if (t != NULL && n != NULL) {
		r = IOPMAssertionCreateWithName(t, kIOPMAssertionLevelOn, n, id);
	}
	if (t != NULL) CFRelease(t);
	if (n != NULL) CFRelease(n);
	return r;
}
*/
import "C"

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"unsafe"
)

// Assertion types from IOPMLib.h. PreventUserIdleDisplaySleep also blocks
// idle system sleep. PreventSystemSleep is marked unsupported in the header
// but powerd still honours it on AC power (caffeinate -s uses it).
const (
	assertIdleSystem   = "PreventUserIdleSystemSleep"
	assertIdleDisplay  = "PreventUserIdleDisplaySleep"
	assertSystemSleep  = "PreventSystemSleep"
	iopmMechanismName  = "IOPMAssertion"
	iopmMechanismLabel = "IOKit power assertions (IOPMAssertionCreateWithName)"
)

func newPlatform() Inhibitor { return iopmInhibitor{} }

func platformMechanisms() []Mechanism {
	return []Mechanism{{Name: iopmMechanismName, Available: true, Detail: iopmMechanismLabel}}
}

// iopmInhibitor holds IOKit power assertions. powerd releases them when the
// process exits, however it exits.
type iopmInhibitor struct{}

func (iopmInhibitor) Name() string { return iopmMechanismName }

func (iopmInhibitor) Acquire(ctx context.Context, o Options) (Hold, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name := label(o)
	h := &iopmHold{}
	if err := h.create(assertIdleSystem, name); err != nil {
		return nil, &Error{Err: err, Hint: "macOS refused the power assertion; check `pmset -g assertions`"}
	}
	if o.KeepDisplay {
		if err := h.create(assertIdleDisplay, name); err != nil {
			h.Release()
			return nil, &Error{Err: err, Hint: "macOS refused the power assertion; check `pmset -g assertions`"}
		}
	}
	// Best effort: only effective on AC power.
	if err := h.create(assertSystemSleep, name); err != nil {
		slog.Debug("power: PreventSystemSleep not available", "err", err)
	}
	slog.Debug("power: acquired", "mechanism", h.Describe(), "reason", o.Reason)
	return h, nil
}

type iopmHold struct {
	once  sync.Once
	ids   []C.IOPMAssertionID
	types []string
}

func (h *iopmHold) create(typ, name string) error {
	ctyp, cname := C.CString(typ), C.CString(name)
	defer C.free(unsafe.Pointer(ctyp))
	defer C.free(unsafe.Pointer(cname))
	var id C.IOPMAssertionID
	if r := C.ka_assertion_create(ctyp, cname, &id); r != C.kIOReturnSuccess {
		return fmt.Errorf("IOPMAssertionCreateWithName(%s): IOReturn 0x%x", typ, uint32(r))
	}
	h.ids = append(h.ids, id)
	h.types = append(h.types, typ)
	return nil
}

func (h *iopmHold) Release() error {
	var err error
	h.once.Do(func() {
		var failed []string
		for i, id := range h.ids {
			if r := C.IOPMAssertionRelease(id); r != C.kIOReturnSuccess {
				failed = append(failed, fmt.Sprintf("%s: IOReturn 0x%x", h.types[i], uint32(r)))
			}
		}
		if len(failed) > 0 {
			err = fmt.Errorf("IOPMAssertionRelease: %s", strings.Join(failed, ", "))
		}
		slog.Debug("power: released", "mechanism", h.Describe(), "err", err)
	})
	return err
}

func (h *iopmHold) Describe() string {
	return iopmMechanismName + "(" + strings.Join(h.types, ", ") + ")"
}
