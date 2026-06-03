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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Warp-net/warpnet/json"
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
)

var (
	errActorMalformed = errors.New("actor document malformed")
	errRemoteStatus   = errors.New("remote returned error status")
	errInsecureURL    = errors.New("remote URL must be https")
)

// gateway is the stateless ActivityPub front for one bridged Warpnet user.
// It holds no content: documents are rendered on demand from source, and the
// only durable secret is the RSA signing key (loaded from disk).
type gateway struct {
	host        string // public hostname, e.g. name.tailnet.ts.net (no scheme)
	key         *rsa.PrivateKey
	keyPubPEM   string
	source      warpnetSource
	signingUser string // SKELETON: the single user the gateway signs outbound as
	client      *http.Client
	sem         chan struct{} // bounds concurrent Accept deliveries
}

func (g *gateway) baseURL() string            { return "https://" + g.host }
func (g *gateway) actorID(user string) string { return g.baseURL() + "/users/" + user }
func (g *gateway) keyID(user string) string   { return g.actorID(user) + "#main-key" }

func (g *gateway) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/webfinger", g.handleWebFinger)
	mux.HandleFunc("/.well-known/nodeinfo", g.handleNodeInfoLinks)
	mux.HandleFunc("/nodeinfo/2.0", g.handleNodeInfo)
	mux.HandleFunc("/users/", g.handleUsers)
	mux.HandleFunc("/inbox", g.handleSharedInbox)
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
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/users/"), "/")
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
		g.serveEmptyCollection(w, g.actorID(user)+"/followers")
	case "following":
		g.serveEmptyCollection(w, g.actorID(user)+"/following")
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
		Inbox:             id + "/inbox",
		Outbox:            id + "/outbox",
		Followers:         id + "/followers",
		Following:         id + "/following",
		PublicKey: publicKey{
			ID:           g.keyID(wu.PreferredUsername),
			Owner:        id,
			PublicKeyPEM: g.keyPubPEM,
		},
		Endpoints: &actorEndpoints{SharedInbox: g.baseURL() + "/inbox"},
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
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(bt)
}

// validateRemoteURL rejects non-HTTPS targets before any outbound fetch. This
// is the minimum SSRF guard for dereferencing attacker-supplied actor/key URLs.
// TODO(prod): also block private/link-local/loopback IPs and guard against
// DNS rebinding before exposing the gateway publicly.
func validateRemoteURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse url %q: %w", raw, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("url %q: %w", raw, errInsecureURL)
	}
	return nil
}

// fetchActor dereferences a remote actor document, signing the GET so it
// works against instances running in authorized-fetch / secure mode.
func (g *gateway) fetchActor(ctx context.Context, actorURL string) (map[string]any, error) {
	if err := validateRemoteURL(actorURL); err != nil {
		return nil, err
	}
	// G704: dereferencing remote actor URLs is intrinsic to ActivityPub
	// federation; validateRemoteURL enforces https, full SSRF hardening is a
	// documented production TODO.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, actorURL, nil) //nolint:gosec // see note above
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", contentTypeAP)
	if err := signRequest(req, g.keyID(g.signingUser), g.key, nil); err != nil {
		return nil, err
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
	if err := validateRemoteURL(target); err != nil {
		return err
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentTypeAP)
	if err := signRequest(req, g.keyID(localUser), g.key, body); err != nil {
		return err
	}
	resp, err := g.client.Do(req)
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
	_, rest, ok := strings.Cut(u, "/users/")
	if !ok {
		return ""
	}
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return rest
}
