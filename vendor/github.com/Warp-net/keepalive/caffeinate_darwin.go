package keepalive

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// caffeinateInhibitor runs caffeinate(8). It is the engine of builds without
// cgo, which cannot call IOKit. `-w <our pid>` makes caffeinate exit on its
// own when keepalive dies, so a SIGKILL never leaves it behind.
type caffeinateInhibitor struct{}

func (caffeinateInhibitor) Name() string { return "caffeinate" }

func (caffeinateInhibitor) Acquire(ctx context.Context, o Options) (Hold, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := exec.LookPath("caffeinate")
	if err != nil {
		return nil, &Error{Err: fmt.Errorf("caffeinate not found: %w", err), Hint: "caffeinate ships with macOS in /usr/bin; check PATH"}
	}
	args := caffeinateArgs(o.KeepDisplay, os.Getpid())
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	h, err := startProc(cmd, "caffeinate "+strings.Join(args, " "))
	if err != nil {
		return nil, &Error{Err: fmt.Errorf("start caffeinate: %w", err), Hint: "caffeinate ships with macOS in /usr/bin; check PATH"}
	}
	slog.Debug("power: acquired", "mechanism", h.Describe(), "reason", o.Reason)
	return h, nil
}

// caffeinateArgs: -i idle system sleep, -d display sleep, -s system sleep on
// AC power, -w exit when pid exits.
func caffeinateArgs(keepDisplay bool, pid int) []string {
	args := []string{"-i"}
	if keepDisplay {
		args = append(args, "-d")
	}
	return append(args, "-s", "-w", strconv.Itoa(pid))
}

func caffeinateMechanism() Mechanism {
	path, err := exec.LookPath("caffeinate")
	if err != nil {
		return Mechanism{Name: "caffeinate", Detail: "caffeinate not found in PATH"}
	}
	return Mechanism{Name: "caffeinate", Available: true, Detail: path}
}
