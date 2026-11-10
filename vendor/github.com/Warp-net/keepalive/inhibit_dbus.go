package keepalive

// The Linux inhibitor, written against a small D-Bus interface so it can be
// tested on any OS with a fake bus. The godbus adapter and the
// systemd-inhibit fallback live in power_linux.go.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	appName = "keepalive"

	logindName    = "org.freedesktop.login1"
	logindPath    = "/org/freedesktop/login1"
	logindInhibit = "org.freedesktop.login1.Manager.Inhibit"

	// gnome-session inhibit flags.
	gnomeInhibitSuspend = 4
	gnomeInhibitIdle    = 8

	// dbusReleaseTimeout bounds each best-effort Uninhibit call.
	dbusReleaseTimeout = 2 * time.Second

	polkitHint  = "logind refused the inhibitor lock (polkit); run from your desktop session, not over SSH"
	noBusesHint = "keepalive needs systemd-logind on the system bus or a desktop session (GNOME, KDE, XFCE) on the session bus"
)

// dbusConn is the part of a D-Bus connection the inhibitor uses.
type dbusConn interface {
	// HasOwner reports whether a well-known bus name currently has an owner.
	HasOwner(ctx context.Context, name string) (bool, error)
	// Call invokes an interface-qualified method and returns the reply
	// body. A Unix fd in the reply arrives as an io.Closer that owns it; an
	// error reply arrives as *dbusError.
	Call(ctx context.Context, dest, path, method string, args ...any) ([]any, error)
	// Done is closed when the connection is lost or closed.
	Done() <-chan struct{}
	Close() error
}

// dbusError is a D-Bus error reply.
type dbusError struct {
	Name    string
	Message string
}

func (e *dbusError) Error() string {
	if e.Message == "" {
		return e.Name
	}
	return e.Name + ": " + e.Message
}

// isPolkitDenial reports whether err is an authorization refusal.
func isPolkitDenial(err error) bool {
	var de *dbusError
	if !errors.As(err, &de) {
		return false
	}
	switch de.Name {
	case "org.freedesktop.DBus.Error.AccessDenied",
		"org.freedesktop.DBus.Error.InteractiveAuthorizationRequired",
		"org.freedesktop.PolicyKit1.Error.NotAuthorized":
		return true
	}
	return false
}

// desktopService is a session-bus inhibitor that is dropped when our
// connection closes.
type desktopService struct {
	name      string   // bus name, also the Describe label
	paths     []string // object paths, tried in order
	iface     string
	uninhibit string // method name of the release call
}

var (
	screenSaverService = desktopService{
		name:      "org.freedesktop.ScreenSaver",
		paths:     []string{"/org/freedesktop/ScreenSaver", "/ScreenSaver"}, // KDE also exports /ScreenSaver
		iface:     "org.freedesktop.ScreenSaver",
		uninhibit: "UnInhibit",
	}
	gnomeSessionService = desktopService{
		name:      "org.gnome.SessionManager",
		paths:     []string{"/org/gnome/SessionManager"},
		iface:     "org.gnome.SessionManager",
		uninhibit: "Uninhibit",
	}
	powerManagementService = desktopService{
		name:      "org.freedesktop.PowerManagement",
		paths:     []string{"/org/freedesktop/PowerManagement/Inhibit"},
		iface:     "org.freedesktop.PowerManagement.Inhibit",
		uninhibit: "UnInhibit",
	}
)

// dbusInhibitor takes a logind block lock on the system bus and, for the
// desktop's own idle handling, inhibitors on the session bus. All of them
// are tied to our fd or connection, so the OS drops them if we die.
type dbusInhibitor struct {
	system  func(context.Context) (dbusConn, error)
	session func(context.Context) (dbusConn, error)
	// fallback takes the logind lock through a helper process when the
	// system bus route fails; nil disables it.
	fallback func(ctx context.Context, what, reason string) (Hold, error)
	// fallbackCheck reports the fallback for Mechanisms; nil omits it.
	fallbackCheck func() Mechanism
}

func (d *dbusInhibitor) Name() string { return "logind+desktop" }

func (d *dbusInhibitor) Acquire(ctx context.Context, o Options) (Hold, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	what := "sleep"
	if o.KeepDisplay {
		what = "sleep:idle"
	}
	reason := label(o)
	h := &dbusHold{what: what, lost: make(chan error, 1), done: make(chan struct{})}
	var problems []string
	hint := ""

	if err := h.lockLogind(ctx, d.system, what, reason); err != nil {
		problems = append(problems, err.Error())
		if isPolkitDenial(err) {
			hint = polkitHint
		}
		if d.fallback != nil && ctx.Err() == nil {
			fh, ferr := d.fallback(ctx, what, reason)
			if ferr == nil {
				h.fallback = fh
			} else {
				problems = append(problems, ferr.Error())
				if hint == "" {
					hint = Hint(ferr)
				}
			}
		}
	}
	if ctx.Err() == nil {
		problems = append(problems, h.inhibitDesktop(ctx, d.session, o.KeepDisplay, reason)...)
	}
	if err := ctx.Err(); err != nil {
		h.Release()
		return nil, err
	}
	if !h.held() {
		h.Release()
		if hint == "" {
			hint = noBusesHint
		}
		return nil, &Error{Err: fmt.Errorf("no sleep inhibitor available: %s", strings.Join(problems, "; ")), Hint: hint}
	}
	for _, p := range problems {
		slog.Info("power: inhibitor not used", "detail", p)
	}
	h.watch()
	slog.Debug("power: acquired", "mechanism", h.Describe(), "reason", o.Reason)
	return h, nil
}

func (d *dbusInhibitor) mechanisms(ctx context.Context) []Mechanism {
	var out []Mechanism
	if sys, err := d.system(ctx); err != nil {
		out = append(out,
			Mechanism{Name: "system bus", Detail: err.Error()},
			Mechanism{Name: "logind", Detail: "no system bus"})
	} else {
		out = append(out, Mechanism{Name: "system bus", Available: true, Detail: "connected"})
		out = append(out, ownerMechanism(ctx, sys, "logind", logindName))
		sys.Close()
	}
	if d.fallbackCheck != nil {
		out = append(out, d.fallbackCheck())
	}
	services := []desktopService{screenSaverService, gnomeSessionService, powerManagementService}
	if ses, err := d.session(ctx); err != nil {
		out = append(out, Mechanism{Name: "session bus", Detail: err.Error()})
		for _, s := range services {
			out = append(out, Mechanism{Name: s.name, Detail: "no session bus"})
		}
	} else {
		out = append(out, Mechanism{Name: "session bus", Available: true, Detail: "connected"})
		for _, s := range services {
			out = append(out, ownerMechanism(ctx, ses, s.name, s.name))
		}
		ses.Close()
	}
	return out
}

func ownerMechanism(ctx context.Context, c dbusConn, label, name string) Mechanism {
	ok, err := c.HasOwner(ctx, name)
	switch {
	case err != nil:
		return Mechanism{Name: label, Detail: err.Error()}
	case !ok:
		return Mechanism{Name: label, Detail: name + " has no owner"}
	}
	return Mechanism{Name: label, Available: true, Detail: name + " is running"}
}

// desktopCookie is an inhibitor to release with uninhibit. The cookie keeps
// the type the service returned (uint32, or int32 from KDE's PowerDevil).
type desktopCookie struct {
	svc    desktopService
	path   string
	cookie any
}

type dbusHold struct {
	what     string
	system   dbusConn  // nil unless the logind lock is held
	logindFD io.Closer // the logind lock; closing it releases the lock
	fallback Hold      // systemd-inhibit when logind could not be reached
	session  dbusConn  // nil unless a desktop inhibitor is held
	cookies  []desktopCookie

	once     sync.Once
	mu       sync.Mutex // guards released against markLost
	released bool
	done     chan struct{} // closed by Release; stops the watchers
	lost     chan error    // buffered(1); one loss at most
}

// lockLogind takes the logind block lock and keeps its fd and connection.
func (h *dbusHold) lockLogind(ctx context.Context, connect func(context.Context) (dbusConn, error), what, reason string) error {
	if connect == nil {
		return errors.New("system bus: not supported")
	}
	conn, err := connect(ctx)
	if err != nil {
		return fmt.Errorf("system bus: %w", err)
	}
	if ok, err := conn.HasOwner(ctx, logindName); err != nil || !ok {
		conn.Close()
		if err != nil {
			return fmt.Errorf("logind: %w", err)
		}
		return errors.New("logind: not running")
	}
	body, err := conn.Call(ctx, logindName, logindPath, logindInhibit, what, appName, reason, "block")
	if err != nil {
		conn.Close()
		return fmt.Errorf("logind: %w", err)
	}
	var fd io.Closer
	if len(body) > 0 {
		fd, _ = body[0].(io.Closer)
	}
	if fd == nil {
		closeAll(body)
		conn.Close()
		return fmt.Errorf("logind: Inhibit returned %v, want a file descriptor", body)
	}
	h.system, h.logindFD = conn, fd
	return nil
}

// inhibitDesktop takes the session-bus inhibitors that apply and returns
// what went wrong with the others. Services whose name has no owner are
// skipped silently: that desktop is simply not running.
func (h *dbusHold) inhibitDesktop(ctx context.Context, connect func(context.Context) (dbusConn, error), keepDisplay bool, reason string) []string {
	if connect == nil {
		return nil
	}
	conn, err := connect(ctx)
	if err != nil {
		return []string{fmt.Sprintf("session bus: %v", err)}
	}
	var problems []string
	try := func(svc desktopService, args ...any) {
		if ok, err := conn.HasOwner(ctx, svc.name); err != nil || !ok {
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", svc.name, err))
			}
			return
		}
		var lastErr error
		for _, path := range svc.paths {
			body, err := conn.Call(ctx, svc.name, path, svc.iface+".Inhibit", args...)
			if err != nil {
				lastErr = err
				continue
			}
			cookie, err := decodeCookie(body)
			if err != nil {
				lastErr = err
				continue
			}
			h.cookies = append(h.cookies, desktopCookie{svc: svc, path: path, cookie: cookie})
			return
		}
		problems = append(problems, fmt.Sprintf("%s: %v", svc.name, lastErr))
	}
	if keepDisplay {
		try(screenSaverService, appName, reason)
		try(gnomeSessionService, appName, uint32(0), reason, uint32(gnomeInhibitIdle|gnomeInhibitSuspend))
		try(powerManagementService, appName, reason)
	} else {
		try(gnomeSessionService, appName, uint32(0), reason, uint32(gnomeInhibitSuspend))
	}
	if len(h.cookies) == 0 {
		conn.Close()
		return problems
	}
	h.session = conn
	return problems
}

// decodeCookie accepts a uint32 or int32 cookie.
func decodeCookie(body []any) (any, error) {
	if len(body) == 0 {
		return nil, errors.New("no cookie in the Inhibit reply")
	}
	switch c := body[0].(type) {
	case uint32, int32:
		return c, nil
	}
	return nil, fmt.Errorf("the Inhibit reply has a %T cookie, want uint32 or int32", body[0])
}

func closeAll(body []any) {
	for _, v := range body {
		if c, ok := v.(io.Closer); ok {
			c.Close()
		}
	}
}

func (h *dbusHold) held() bool {
	return h.logindFD != nil || h.fallback != nil || len(h.cookies) > 0
}

// watch reports a lost connection or helper through Lost.
func (h *dbusHold) watch() {
	if h.system != nil {
		go h.watchConn(h.system, "system bus connection lost")
	}
	if h.session != nil {
		go h.watchConn(h.session, "session bus connection lost")
	}
	if w, ok := h.fallback.(Watcher); ok {
		go func() {
			select {
			case err := <-w.Lost():
				h.markLost(err)
			case <-h.done:
			}
		}()
	}
}

func (h *dbusHold) watchConn(c dbusConn, msg string) {
	select {
	case <-c.Done():
		h.markLost(errors.New(msg))
	case <-h.done:
	}
}

func (h *dbusHold) markLost(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.released {
		return
	}
	select {
	case h.lost <- err:
	default:
	}
}

func (h *dbusHold) Lost() <-chan error { return h.lost }

// Release uninhibits every cookie (best effort: closing the connection drops
// them anyway), closes the logind fd, stops the fallback and closes both
// connections.
func (h *dbusHold) Release() error {
	var err error
	h.once.Do(func() {
		h.mu.Lock()
		h.released = true
		h.mu.Unlock()
		close(h.done)

		var errs []error
		for i := len(h.cookies) - 1; i >= 0; i-- {
			c := h.cookies[i]
			ctx, cancel := context.WithTimeout(context.Background(), dbusReleaseTimeout)
			if _, uerr := h.session.Call(ctx, c.svc.name, c.path, c.svc.iface+"."+c.svc.uninhibit, c.cookie); uerr != nil {
				slog.Debug("power: uninhibit failed", "service", c.svc.name, "err", uerr)
			}
			cancel()
		}
		if h.logindFD != nil {
			if cerr := h.logindFD.Close(); cerr != nil {
				errs = append(errs, fmt.Errorf("close logind inhibitor: %w", cerr))
			}
		}
		if h.fallback != nil {
			if ferr := h.fallback.Release(); ferr != nil {
				errs = append(errs, ferr)
			}
		}
		if h.session != nil {
			h.session.Close()
		}
		if h.system != nil {
			h.system.Close()
		}
		err = errors.Join(errs...)
		slog.Debug("power: released", "mechanism", h.Describe(), "err", err)
	})
	return err
}

func (h *dbusHold) Describe() string {
	var parts []string
	if h.logindFD != nil {
		parts = append(parts, "logind("+h.what+")")
	}
	if h.fallback != nil {
		parts = append(parts, h.fallback.Describe())
	}
	for _, c := range h.cookies {
		parts = append(parts, c.svc.name)
	}
	return strings.Join(parts, " + ")
}
