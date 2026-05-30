// Package deeplink parses warpnet:// URLs and registers the scheme per OS.
package deeplink

import (
	"errors"
	"net/url"
	"path"
	"strings"
)

const Scheme = "warpnet"

type Kind string

const (
	KindUser Kind = "user"
)

type Link struct {
	Kind Kind
	ID   string
	Raw  string
}

var ErrNotWarpnetURL = errors.New("deeplink: not a warpnet:// URL")
var ErrUnsupportedKind = errors.New("deeplink: unsupported resource kind")
var ErrMissingID = errors.New("deeplink: missing resource id")

func Parse(raw string) (Link, error) {
	original := strings.TrimSpace(raw)
	if original == "" {
		return Link{}, ErrNotWarpnetURL
	}
	// Canonicalise "warpnet:user/x" -> "warpnet://user/x"; keep Raw as the original.
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
		id := strings.Trim(path.Clean("/"+u.Path), "/")
		if id == "" || id == "." {
			return Link{}, ErrMissingID
		}
		return Link{Kind: KindUser, ID: id, Raw: original}, nil
	default:
		return Link{}, ErrUnsupportedKind
	}
}

// FromArgs returns the first parseable warpnet:// arg, tolerating leading "-"/"--".
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
