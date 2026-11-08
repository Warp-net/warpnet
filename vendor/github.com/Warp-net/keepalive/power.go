// Package power keeps the system (and optionally the display) awake. Callers
// acquire a Hold and release it when the session ends. Every implementation
// uses an OS mechanism that the OS drops when the process exits, so a crashed
// or killed keepalive never leaves the machine unable to sleep.
package keepalive

import (
	"context"
	"errors"
)

// Options describes the hold being requested.
type Options struct {
	// KeepDisplay keeps the display on as well; false keeps only the system
	// awake.
	KeepDisplay bool
	// Reason is shown by OS tools that list power assertions.
	Reason string
}

// Hold is an acquired power assertion. Release is idempotent: only the first
// call releases anything, later calls return nil.
type Hold interface {
	Release() error
	// Describe is a short human description of the mechanism in use, e.g.
	// "IOPMAssertion(PreventUserIdleSystemSleep)".
	Describe() string
}

// Watcher is implemented by holds the OS can take away while they are held
// (a D-Bus connection drops, a helper process exits). Lost delivers one
// error when that happens; it never fires after Release.
type Watcher interface {
	Lost() <-chan error
}

// Inhibitor acquires power holds.
type Inhibitor interface {
	Acquire(ctx context.Context, o Options) (Hold, error)
	Name() string
}

// New returns the inhibitor for the current OS.
func New() Inhibitor { return newPlatform() }

// Mechanism is one way this OS can keep the system awake, as reported by
// Mechanisms for `keepalive doctor`.
type Mechanism struct {
	Name      string
	Available bool
	Detail    string
}

// Mechanisms lists the sleep-prevention mechanisms of this OS and whether
// each is usable right now. It takes no power holds.
func Mechanisms() []Mechanism { return platformMechanisms() }

// Error is an acquisition failure with a short hint for the user.
type Error struct {
	Err  error
	Hint string
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Hint returns the hint carried by err, or "".
func Hint(err error) string {
	var pe *Error
	if errors.As(err, &pe) {
		return pe.Hint
	}
	return ""
}

// label is the name shown by OS tools for our holds.
func label(o Options) string {
	if o.Reason == "" {
		return "keepalive: keeping the system awake"
	}
	return "keepalive: " + o.Reason
}
