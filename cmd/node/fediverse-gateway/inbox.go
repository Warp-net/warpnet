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
	"context"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

const acceptDeliveryTimeout = 30 * time.Second

func (g *gateway) handleSharedInbox(w http.ResponseWriter, r *http.Request) {
	g.handleInbox(w, r, "")
}

// handleInbox verifies the HTTP signature on an inbound activity and, for the
// Phase-1 skeleton, answers Follow with a signed Accept. Other activity types
// are acknowledged but not yet translated into Warpnet (Phase 2/3).
func (g *gateway) handleInbox(w http.ResponseWriter, r *http.Request, user string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if err := verifyRequest(r, body, func(keyID string) (*rsa.PublicKey, error) {
		return g.fetchKey(r.Context(), keyID)
	}); err != nil {
		log.Warnf("inbox: signature verification failed: %v", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	typ, _ := raw[keyType].(string)
	remoteActor, _ := raw[keyActor].(string)
	log.Infof("inbox: %s from %s", typ, remoteActor)

	switch typ {
	case typeFollow:
		localUser := userFromActorURL(stringField(raw, keyObject))
		if localUser == "" {
			localUser = user
		}
		// Only sign/persist for an actor we actually host — otherwise the
		// shared inbox becomes a signing oracle and an unbounded follower-state
		// sink for attacker-chosen usernames.
		if _, ok := g.source.GetUser(localUser); !ok {
			log.Warnf("inbox: Follow targets unknown local user %q from %s", localUser, remoteActor)
			http.Error(w, "unknown actor", http.StatusNotFound)
			return
		}
		// Bound concurrent Accept deliveries; on saturation ask the peer to
		// retry rather than spawning unbounded goroutines.
		select {
		case g.sem <- struct{}{}:
			go func() {
				defer func() { <-g.sem }()
				g.acceptFollow(localUser, remoteActor, raw)
			}()
			w.WriteHeader(http.StatusAccepted)
		default:
			log.Warnf("inbox: delivery pool full, asking %s to retry", remoteActor)
			http.Error(w, "busy", http.StatusServiceUnavailable)
		}
	case typeDelete:
		g.handleDelete(w, raw)
	default:
		g.handleInboundActivity(w, typ, raw)
	}
}

// handleInboundActivity translates a non-Follow inbound activity into a Warpnet
// route and forwards it to the owner's node, bounded by the delivery semaphore.
func (g *gateway) handleInboundActivity(w http.ResponseWriter, typ string, raw map[string]any) {
	route, payload, ok := g.translateInbound(raw)
	if !ok {
		log.Infof("inbox: %q acknowledged, not handled", typ)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if g.req == nil {
		log.Warnf("inbox: %q needs a Warpnet node connection", typ)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	select {
	case g.sem <- struct{}{}:
		go func() {
			defer func() { <-g.sem }()
			if _, err := g.req.request(route, payload); err != nil {
				log.Errorf("inbox: forward %s -> %s: %v", typ, route, err)
			}
		}()
		w.WriteHeader(http.StatusAccepted)
	default:
		log.Warnf("inbox: delivery pool full, dropping %s from %s", typ, raw[keyActor])
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}
}

// handleDelete is a stub. The gateway can't delete on the owner's node (deletion
// is the owner-only PRIVATE_DELETE_TWEET route), so it acknowledges the activity
// — stopping peer retries — and logs the target so it can be removed manually
// through direct node access.
func (g *gateway) handleDelete(w http.ResponseWriter, raw map[string]any) {
	log.Infof("inbox: Delete from %s for %s acknowledged; gateway cannot delete "+
		"(owner-only) — remove it on the node directly if needed",
		stringField(raw, keyActor), stringField(raw, keyObject))
	w.WriteHeader(http.StatusAccepted)
}

// acceptFollow resolves the follower's inbox and delivers a signed Accept.
func (g *gateway) acceptFollow(localUser, remoteActorURL string, follow map[string]any) {
	if remoteActorURL == "" {
		log.Warnf("accept: missing remote actor")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), acceptDeliveryTimeout)
	defer cancel()

	inbox, err := g.remoteInbox(ctx, remoteActorURL)
	if err != nil {
		log.Errorf("accept: resolve inbox for %s: %v", remoteActorURL, err)
		return
	}
	accept := activity{
		Context: asContext,
		ID:      g.actorID(localUser) + "#accept-" + randomToken(),
		Type:    "Accept",
		Actor:   g.actorID(localUser),
		Object:  follow,
	}
	if err := g.postSigned(ctx, localUser, inbox, accept); err != nil {
		log.Errorf("accept: deliver to %s: %v", inbox, err)
		return
	}
	if err := g.followers.Add(localUser, remoteActorURL); err != nil {
		log.Errorf("accept: persist follower %s: %v", remoteActorURL, err)
	}
	log.Infof("accept: Follow accepted (local=%s remote=%s)", localUser, remoteActorURL)

	// localUser now has a Fediverse follower — start federating their posts/follows.
	if g.onFollowed != nil {
		g.onFollowed(localUser)
	}
}

// stringField reads a string value from a loosely-typed activity, tolerating
// the "object": {"id": "..."} object form as well as a bare string.
func stringField(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return v
	case map[string]any:
		id, _ := v["id"].(string)
		return id
	default:
		return ""
	}
}
