package keepalive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// systemdInhibitReadyTimeout bounds how long the fallback waits for
// systemd-inhibit to take its lock.
const systemdInhibitReadyTimeout = 10 * time.Second

func newPlatform() Inhibitor {
	return &dbusInhibitor{
		system:        connectSystemBus,
		session:       connectSessionBus,
		fallback:      systemdInhibit,
		fallbackCheck: systemdInhibitMechanism,
	}
}

func platformMechanisms() []Mechanism {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return (&dbusInhibitor{system: connectSystemBus, session: connectSessionBus, fallbackCheck: systemdInhibitMechanism}).mechanisms(ctx)
}

// ---- godbus adapter ----

// godbusConn is a private bus connection owned by one hold. It is never the
// shared godbus connection, so closing it closes nobody else's.
type godbusConn struct{ c *dbus.Conn }

func connectSystemBus(context.Context) (dbusConn, error) {
	c, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	return godbusConn{c}, nil
}

// connectSessionBus connects to the session bus without godbus's dbus-launch
// autolaunch, which would start a bus of our own over SSH.
func connectSessionBus(context.Context) (dbusConn, error) {
	addr := sessionBusAddress()
	if addr == "" {
		return nil, errors.New("no session bus (DBUS_SESSION_BUS_ADDRESS is unset and $XDG_RUNTIME_DIR/bus does not exist)")
	}
	c, err := dbus.Connect(addr)
	if err != nil {
		return nil, err
	}
	return godbusConn{c}, nil
}

func sessionBusAddress() string {
	if a := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" && a != "autolaunch:" {
		return a
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	sock := filepath.Join(dir, "bus")
	if fi, err := os.Stat(sock); err == nil && fi.Mode()&os.ModeSocket != 0 {
		return "unix:path=" + dbus.EscapeBusAddressValue(sock)
	}
	return ""
}

func (g godbusConn) HasOwner(ctx context.Context, name string) (bool, error) {
	var ok bool
	err := g.c.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, name).Store(&ok)
	return ok, convertDBusError(err)
}

func (g godbusConn) Call(ctx context.Context, dest, path, method string, args ...any) ([]any, error) {
	call := g.c.Object(dest, dbus.ObjectPath(path)).CallWithContext(ctx, method, 0, args...)
	if call.Err != nil {
		return nil, convertDBusError(call.Err)
	}
	body := make([]any, len(call.Body))
	for i, v := range call.Body {
		if fd, ok := v.(dbus.UnixFD); ok {
			body[i] = os.NewFile(uintptr(fd), "dbus-fd")
			continue
		}
		body[i] = v
	}
	return body, nil
}

func (g godbusConn) Done() <-chan struct{} { return g.c.Context().Done() }

func (g godbusConn) Close() error { return g.c.Close() }

func convertDBusError(err error) error {
	var de dbus.Error
	if errors.As(err, &de) {
		msg, _ := firstString(de.Body)
		return &dbusError{Name: de.Name, Message: msg}
	}
	var dep *dbus.Error
	if errors.As(err, &dep) && dep != nil {
		msg, _ := firstString(dep.Body)
		return &dbusError{Name: dep.Name, Message: msg}
	}
	return err
}

func firstString(body []any) (string, bool) {
	if len(body) == 0 {
		return "", false
	}
	s, ok := body[0].(string)
	return s, ok
}

// ---- systemd-inhibit fallback ----

// systemdInhibit takes the logind lock through systemd-inhibit when the
// system bus route failed. Its child is `cat` reading a pipe we hold: cat
// echoes one byte once the lock is taken (our readiness signal) and exits on
// EOF when we close the pipe or die, which ends systemd-inhibit and releases
// the lock. Pdeathsig and a process group of its own back that up. Never
// inhibits shutdown.
func systemdInhibit(ctx context.Context, what, reason string) (Hold, error) {
	path, err := exec.LookPath("systemd-inhibit")
	if err != nil {
		return nil, errors.New("systemd-inhibit: not found")
	}
	inR, inW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		inR.Close()
		inW.Close()
		return nil, err
	}
	defer outR.Close()
	var stderr bytes.Buffer
	cmd := exec.Command(path, "--what="+what, "--who="+appName, "--why="+reason, "--mode=block", "cat")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inR, outW, &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	h, err := startProc(cmd, "systemd-inhibit("+what+")")
	inR.Close()
	outW.Close()
	if err != nil {
		inW.Close()
		return nil, fmt.Errorf("systemd-inhibit: %w", err)
	}
	h.stdin = inW

	ready := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(outR, make([]byte, 1))
		ready <- err
	}()
	inW.Write([]byte{'\n'})
	timeout := time.NewTimer(systemdInhibitReadyTimeout)
	defer timeout.Stop()
	select {
	case err := <-ready:
		if err == nil {
			return h, nil
		}
		h.Release()
		msg := ""
		select {
		case <-h.exited: // stderr is complete and no longer written
			msg = strings.TrimSpace(stderr.String())
		default:
		}
		if msg == "" {
			msg = "exited before taking the lock"
		}
		ferr := fmt.Errorf("systemd-inhibit: %s", msg)
		if strings.Contains(strings.ToLower(msg), "access denied") || strings.Contains(strings.ToLower(msg), "not authorized") {
			return nil, &Error{Err: ferr, Hint: polkitHint}
		}
		return nil, ferr
	case <-ctx.Done():
		h.Release()
		return nil, ctx.Err()
	case <-timeout.C:
		h.Release()
		return nil, errors.New("systemd-inhibit: timed out waiting for the lock")
	}
}

func systemdInhibitMechanism() Mechanism {
	path, err := exec.LookPath("systemd-inhibit")
	if err != nil {
		return Mechanism{Name: "systemd-inhibit", Detail: "not found in PATH"}
	}
	return Mechanism{Name: "systemd-inhibit", Available: true, Detail: path + " (fallback)"}
}
