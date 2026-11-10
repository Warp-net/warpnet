//go:build darwin || linux

package keepalive

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// procStopTimeout bounds how long Release waits for the helper to exit after
// SIGTERM before it sends SIGKILL.
const procStopTimeout = 2 * time.Second

// procHold keeps a helper process (caffeinate, systemd-inhibit) alive for
// the lifetime of the hold. The helper runs in its own process group so a
// terminal Ctrl-C reaches keepalive first and the hold is released in order.
type procHold struct {
	cmd   *exec.Cmd
	desc  string
	stdin io.Closer // optional; closed on release

	exited   chan struct{} // closed once the helper has been reaped
	waitErr  error         // valid after exited is closed
	lost     chan error    // buffered(1)
	mu       sync.Mutex    // orders released against the loss report
	released bool
	once     sync.Once
}

// startProc starts cmd and watches it. The caller sets cmd.SysProcAttr; it
// must include Setpgid.
func startProc(cmd *exec.Cmd, desc string) (*procHold, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	h := &procHold{cmd: cmd, desc: desc, exited: make(chan struct{}), lost: make(chan error, 1)}
	go h.wait()
	return h, nil
}

func (h *procHold) wait() {
	h.waitErr = h.cmd.Wait()
	close(h.exited)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.released {
		return
	}
	err := h.waitErr
	if err == nil {
		err = errors.New("exited")
	}
	slog.Debug("power: helper exited while holding", "mechanism", h.desc, "err", err)
	h.lost <- fmt.Errorf("%s: %w", h.desc, err)
}

func (h *procHold) Lost() <-chan error { return h.lost }

func (h *procHold) Describe() string { return h.desc }

// Release stops the helper (SIGTERM to its group, SIGKILL after
// procStopTimeout) and reaps it.
func (h *procHold) Release() error {
	var err error
	h.once.Do(func() {
		h.mu.Lock()
		h.released = true
		h.mu.Unlock()
		if h.stdin != nil {
			h.stdin.Close()
		}
		err = h.stop()
		slog.Debug("power: released", "mechanism", h.desc, "err", err)
	})
	return err
}

func (h *procHold) stop() error {
	select {
	case <-h.exited:
		return nil
	default:
	}
	pgid := -h.cmd.Process.Pid
	if err := syscall.Kill(pgid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		h.cmd.Process.Kill()
	}
	select {
	case <-h.exited:
		return nil
	case <-time.After(procStopTimeout):
	}
	syscall.Kill(pgid, syscall.SIGKILL)
	select {
	case <-h.exited:
		return nil
	case <-time.After(procStopTimeout):
		return fmt.Errorf("%s (pid %d) did not exit", h.desc, h.cmd.Process.Pid)
	}
}
