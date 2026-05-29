// Package deeplink parses warpnet:// URLs received from the OS and
// ships per-platform registration helpers so the running binary can
// claim the warpnet:// scheme without an installer.
//
// macOS registration is handled by Info.plist generated from
// wails.json's info.protocols entry — the Register() helpers in this
// package are no-ops there. Windows uses HKCU\Software\Classes
// (per-user, no admin needed). Linux writes a .desktop file under
// ~/.local/share/applications and asks xdg-mime to associate the
// scheme with it.
package deeplink

import (
	"errors"
	"net/url"
	"path"
	"strings"
)

// Scheme is the URL scheme the app claims OS-wide.
const Scheme = "warpnet"

// Kind tags a parsed deep link by which screen it targets in the UI.
type Kind string

const (
	// KindUser opens the profile of a given user id: warpnet://user/{id}.
	KindUser Kind = "user"
)

// Link is the parsed, validated form of a warpnet:// URL safe to hand
// off to the frontend.
type Link struct {
	Kind Kind   // which screen
	ID   string // resource id (e.g. user id)
	Raw  string // original URL, for diagnostics
}

// ErrNotWarpnetURL is returned when the input is not a warpnet:// URL.
var ErrNotWarpnetURL = errors.New("deeplink: not a warpnet:// URL")

// ErrUnsupportedKind is returned when the host (user, tweet, …) is not
// one we know how to route yet.
var ErrUnsupportedKind = errors.New("deeplink: unsupported resource kind")

// ErrMissingID is returned for warpnet://user/ with no id.
var ErrMissingID = errors.New("deeplink: missing resource id")

// Parse accepts a single argv-style string and returns a Link if it
// looks like a warpnet:// deep link. It tolerates trailing slashes
// and accepts either warpnet://user/{id} or warpnet:user/{id} (some
// shells strip the // when forwarding).
func Parse(raw string) (Link, error) {
	original := strings.TrimSpace(raw)
	if original == "" {
		return Link{}, ErrNotWarpnetURL
	}
	// Tolerate the rare "warpnet:user/x" form by canonicalising to
	// "warpnet://user/x" before handing it to net/url. Keep the
	// original for Link.Raw so callers can log exactly what the OS
	// handed us.
	parseable := original
	lower := strings.ToLower(parseable)
	if strings.HasPrefix(lower, Scheme+":") && !strings.HasPrefix(lower, Scheme+"://") {
		parseable = Scheme + "://" + parseable[len(Scheme)+1:]
	}

	u, err := url.Parse(parseable)
	if err != nil {
		return Link{}, ErrNotWarpnetURL
	}
	if !strings.EqualFold(u.Scheme, Scheme) {
		return Link{}, ErrNotWarpnetURL
	}

	host := strings.ToLower(u.Host)
	switch Kind(host) {
	case KindUser:
		// path is "/{id}" — strip leading slash, reject if blank
		id := strings.Trim(path.Clean("/"+u.Path), "/")
		if id == "" || id == "." {
			return Link{}, ErrMissingID
		}
		return Link{Kind: KindUser, ID: id, Raw: original}, nil
	default:
		return Link{}, ErrUnsupportedKind
	}
}

// FromArgs scans os.Args-style argv for a warpnet:// argument and
// returns the first parseable link, if any. Tolerates a leading
// "-" or "--" before the scheme (some shell wrappers and the
// canonical "warpnet --warpnet://..." invocation pass the link
// that way). Returns ok=false when none of the arguments are a
// deep link; callers can ignore the error in that case.
func FromArgs(args []string) (Link, bool) {
	for _, a := range args {
		candidate := strings.TrimLeft(strings.TrimSpace(a), "-")
		if !strings.HasPrefix(strings.ToLower(candidate), Scheme+":") {
			continue
		}
		l, err := Parse(candidate)
		if err == nil {
			return l, true
		}
	}
	return Link{}, false
}
