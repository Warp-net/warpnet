/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU Affero General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU Affero General Public License for more details.

 You should have received a copy of the GNU Affero General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	contentTypeAP  = "application/activity+json"
	contentTypeJRD = "application/jrd+json"
	asContext      = "https://www.w3.org/ns/activitystreams"
	secContext     = "https://w3id.org/security/v1"

	maxBodyBytes = 1 << 20

	// maxInflightDeliveries bounds concurrent outbound Accept deliveries so a
	// burst of inbound Follow activities can't spawn unbounded goroutines.
	maxInflightDeliveries = 16

	// maxRedirects caps redirect hops for outbound federation fetches.
	maxRedirects = 5

	pathUsers     = "/users/"
	pathInbox     = "/inbox"
	pathFollowers = "/followers"
	pathStatuses  = "/statuses/"
	pathMedia     = "/media/"

	headerContentType = "Content-Type"
)

var (
	errActorMalformed   = errors.New("actor document malformed")
	errRemoteStatus     = errors.New("remote returned error status")
	errInsecureURL      = errors.New("remote URL must be https")
	errBlockedHost      = errors.New("remote URL host is not allowed")
	errTooManyRedirects = errors.New("too many redirects")
)

// gateway is the ActivityPub front for one bridged Warpnet user. Documents are
// rendered on demand from source; the state it keeps on disk is the RSA signing
// key and the follower store (followers.go). Warpnet content is never stored
// here.
type gateway struct {
	host        string // public hostname, e.g. name.tailnet.ts.net (no scheme)
	key         *rsa.PrivateKey
	keyPubPEM   string
	source      warpnetSource
	signingUser string // optional: user to sign authorized-fetch GETs as ("" = unsigned)
	client      *http.Client
	sem         chan struct{} // bounds concurrent Accept deliveries
	followers   followerStore
	req         nodeRequester          // connector to the owner's node; nil in dev/no-node mode
	onFollowed  func(localUser string) // starts outbound federation for a user; nil without a node

	// allowPrivateTargets disables the SSRF guard's loopback/private-range
	// rejection for outbound delivery. Test-only; never set in main.go.
	allowPrivateTargets bool
}

func (g *gateway) baseURL() string            { return "https://" + g.host }
func (g *gateway) actorID(user string) string { return g.baseURL() + pathUsers + user }
func (g *gateway) keyID(user string) string   { return g.actorID(user) + "#main-key" }

func (g *gateway) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/webfinger", g.handleWebFinger)
	mux.HandleFunc("/.well-known/nodeinfo", g.handleNodeInfoLinks)
	mux.HandleFunc("/nodeinfo/2.0", g.handleNodeInfo)
	mux.HandleFunc(pathUsers, g.handleUsers)
	mux.HandleFunc(pathInbox, g.handleSharedInbox)
	mux.HandleFunc(pathMedia, g.handleMedia)
	return mux
}

func (g *gateway) handleWebFinger(w http.ResponseWriter, r *http.Request) {
	acct := strings.TrimPrefix(r.URL.Query().Get("resource"), "acct:")
	at := strings.LastIndexByte(acct, '@')
	if at < 0 {
		http.Error(w, "bad resource", http.StatusBadRequest)
		return
	}
	user, domain := acct[:at], acct[at+1:]
	if domain != g.host {
		http.NotFound(w, r)
		return
	}
	if _, ok := g.source.GetUser(user); !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, contentTypeJRD, webFingerJRD{
		Subject: "acct:" + user + "@" + g.host,
		Links: []webFingerLink{{
			Rel:  "self",
			Type: contentTypeAP,
			Href: g.actorID(user),
		}},
	})
}

// handleUsers serves the actor document at /users/{user} and dispatches the
// per-actor sub-collections and inbox.
func (g *gateway) handleUsers(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, pathUsers), "/")
	user := parts[0]
	wu, ok := g.source.GetUser(user)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 || parts[1] == "" {
		g.serveActor(w, wu)
		return
	}
	switch parts[1] {
	case "inbox":
		g.handleInbox(w, r, user)
	case "outbox":
		g.serveEmptyCollection(w, g.actorID(user)+"/outbox")
	case "followers":
		g.serveFollowers(w, user)
	case "following":
		g.serveEmptyCollection(w, g.actorID(user)+"/following")
	case "statuses":
		if len(parts) < 3 || parts[2] == "" {
			http.NotFound(w, r)
			return
		}
		g.serveStatus(w, r, user, parts[2])
	default:
		http.NotFound(w, r)
	}
}

func (g *gateway) serveActor(w http.ResponseWriter, wu warpnetUser) {
	id := g.actorID(wu.PreferredUsername)
	writeJSON(w, contentTypeAP, actor{
		Context:           []string{asContext, secContext},
		ID:                id,
		Type:              "Person",
		PreferredUsername: wu.PreferredUsername,
		Name:              wu.DisplayName,
		Summary:           wu.Summary,
		Inbox:             id + pathInbox,
		Outbox:            id + "/outbox",
		Followers:         id + pathFollowers,
		Following:         id + "/following",
		PublicKey: publicKey{
			ID:           g.keyID(wu.PreferredUsername),
			Owner:        id,
			PublicKeyPEM: g.keyPubPEM,
		},
		Endpoints: &actorEndpoints{SharedInbox: g.baseURL() + pathInbox},
	})
}

func (g *gateway) serveEmptyCollection(w http.ResponseWriter, id string) {
	writeJSON(w, contentTypeAP, orderedCollection{
		Context:      asContext,
		ID:           id,
		Type:         "OrderedCollection",
		TotalItems:   0,
		OrderedItems: []any{},
	})
}

func (g *gateway) serveFollowers(w http.ResponseWriter, user string) {
	urls, err := g.followers.List(user)
	if err != nil {
		log.Errorf("followers: list %s: %v", user, err)
	}
	items := make([]any, 0, len(urls))
	for _, u := range urls {
		items = append(items, u)
	}
	writeJSON(w, contentTypeAP, orderedCollection{
		Context:      asContext,
		ID:           g.actorID(user) + pathFollowers,
		Type:         "OrderedCollection",
		TotalItems:   len(items),
		OrderedItems: items,
	})
}

func (g *gateway) handleNodeInfoLinks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, "application/json", nodeInfoLinks{Links: []nodeInfoLink{{
		Rel:  "http://nodeinfo.diaspora.software/ns/schema/2.0",
		Href: g.baseURL() + "/nodeinfo/2.0",
	}}})
}

func (g *gateway) handleNodeInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, "application/json", map[string]any{
		"version":           "2.0",
		"software":          map[string]any{"name": "warpnet-fediverse-gateway", "version": gatewayVersion},
		"protocols":         []string{"activitypub"},
		"usage":             map[string]any{"users": map[string]any{"total": 1}},
		"openRegistrations": false,
		"services":          map[string]any{"inbound": []any{}, "outbound": []any{}},
		"metadata":          map[string]any{},
	})
}

func writeJSON(w http.ResponseWriter, contentType string, v any) {
	bt, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "marshal", http.StatusInternalServerError)
		return
	}
	w.Header().Set(headerContentType, contentType)
	_, _ = w.Write(bt)
}

// validateRemoteURL is the SSRF guard for dereferencing attacker-supplied
// actor/key URLs: it requires https, a host, and rejects localhost plus literal
// IPs in loopback/private/link-local/unspecified ranges.
// TODO(prod): also guard hostnames that resolve into those ranges (DNS
// rebinding) at dial time before exposing the gateway publicly.
func validateRemoteURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse url %q: %w", raw, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("url %q: %w", raw, errInsecureURL)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url %q has no host: %w", raw, errBlockedHost)
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return fmt.Errorf("url %q targets localhost: %w", raw, errBlockedHost)
	}
	if addr, perr := netip.ParseAddr(host); perr == nil && isBlockedIP(addr) {
		return fmt.Errorf("url %q targets a disallowed address: %w", raw, errBlockedHost)
	}
	return nil
}

// isBlockedIP reports whether ip is in a range outbound federation must never
// reach (loopback, private, link-local, multicast, unspecified).
func isBlockedIP(ip netip.Addr) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()
}

// newSafeClient builds the HTTP client for outbound federation, hardened against
// SSRF on attacker-supplied actor/key URLs: CheckRedirect re-applies the URL
// guard to every redirect hop (and caps the chain), and the dialer's Control
// validates the *resolved* IP at connect time, closing DNS-rebinding that the
// hostname checks can't see. Signatures aren't re-applied across redirects, so a
// redirected signed fetch fails at the target — failing closed.
func newSafeClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	dialer.Control = func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip, perr := netip.ParseAddr(host); perr == nil && isBlockedIP(ip) {
			return fmt.Errorf("dial %s: %w", address, errBlockedHost)
		}
		return nil
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = dialer.DialContext
	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errTooManyRedirects
			}
			return validateRemoteURL(req.URL.String())
		},
	}
}

// fetchActor dereferences a remote actor document, signing the GET so it
// works against instances running in authorized-fetch / secure mode.
func (g *gateway) fetchActor(ctx context.Context, actorURL string) (map[string]any, error) {
	if !g.allowPrivateTargets {
		if err := validateRemoteURL(actorURL); err != nil {
			return nil, err
		}
	}
	// G704: dereferencing remote actor URLs is intrinsic to ActivityPub
	// federation; validateRemoteURL enforces https, full SSRF hardening is a
	// documented production TODO.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, actorURL, nil) //nolint:gosec // see note above
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", contentTypeAP)
	// Sign the fetch only when a signing user is configured (authorized-fetch /
	// secure-mode peers require it); otherwise fetch unsigned.
	if g.signingUser != "" {
		if err := signRequest(req, g.keyID(g.signingUser), g.key, nil); err != nil {
			return nil, err
		}
	}
	resp, err := g.client.Do(req) //nolint:gosec // see G704 note above
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d: %w", actorURL, resp.StatusCode, errRemoteStatus)
	}
	bt, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(bt, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// fetchKey resolves a keyId (actorURL#main-key) to its RSA public key.
func (g *gateway) fetchKey(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	actorURL := strings.SplitN(keyID, "#", 2)[0]
	m, err := g.fetchActor(ctx, actorURL)
	if err != nil {
		return nil, err
	}
	pk, ok := m["publicKey"].(map[string]any)
	if !ok {
		// Some servers send publicKey as an array; take the first object.
		if arr, isArr := m["publicKey"].([]any); isArr && len(arr) > 0 {
			pk, ok = arr[0].(map[string]any)
		}
	}
	if !ok || pk == nil {
		return nil, fmt.Errorf("actor %s has no publicKey: %w", actorURL, errActorMalformed)
	}
	pemStr, _ := pk["publicKeyPem"].(string)
	if pemStr == "" {
		return nil, fmt.Errorf("actor %s has no publicKeyPem: %w", actorURL, errActorMalformed)
	}
	return parseRSAPublicKeyPEM(pemStr)
}

// remoteInbox returns the best inbox URL for a remote actor (sharedInbox if
// advertised, otherwise the personal inbox).
func (g *gateway) remoteInbox(ctx context.Context, actorURL string) (string, error) {
	m, err := g.fetchActor(ctx, actorURL)
	if err != nil {
		return "", err
	}
	if ep, ok := m["endpoints"].(map[string]any); ok {
		if si, ok := ep["sharedInbox"].(string); ok && si != "" {
			return si, nil
		}
	}
	if inbox, ok := m["inbox"].(string); ok && inbox != "" {
		return inbox, nil
	}
	return "", fmt.Errorf("actor %s has no inbox: %w", actorURL, errActorMalformed)
}

// postSigned delivers a signed POST of doc to target, as localUser.
func (g *gateway) postSigned(ctx context.Context, localUser, target string, doc any) error {
	if !g.allowPrivateTargets {
		if err := validateRemoteURL(target); err != nil {
			return err
		}
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	// G704: target is a federation peer inbox; validateRemoteURL (above) enforces
	// https and newSafeClient guards redirects + the resolved dial IP.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body)) //nolint:gosec // see note above
	if err != nil {
		return err
	}
	req.Header.Set(headerContentType, contentTypeAP)
	if err := signRequest(req, g.keyID(localUser), g.key, body); err != nil {
		return err
	}
	resp, err := g.client.Do(req) //nolint:gosec // see G704 note above
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("deliver to %s: status %d: %w", target, resp.StatusCode, errRemoteStatus)
	}
	return nil
}

func randomToken() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// userFromActorURL extracts the username from one of our own actor URLs
// (https://host/users/NAME).
func userFromActorURL(u string) string {
	_, rest, ok := strings.Cut(u, pathUsers)
	if !ok {
		return ""
	}
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return rest
}
