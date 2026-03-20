package selfupdate

import (
	"net/url"
	"strings"
)

// canUseTokenForDomain returns true if other URL is in the same domain as origin URL
func canUseTokenForDomain(origin, other string) (bool, error) {
	originURL, err := url.Parse(origin)
	if err != nil {
		return false, err
	}
	otherURL, err := url.Parse(other)
	if err != nil {
		return false, err
	}
	return originURL.Hostname() != "" && strings.HasSuffix(otherURL.Hostname(), originURL.Hostname()), nil
}
