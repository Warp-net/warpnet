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
	"io"
	"net/http"
	"time"

	"github.com/Warp-net/warpnet/json"
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
	typ, _ := raw["type"].(string)
	remoteActor, _ := raw["actor"].(string)
	log.Infof("inbox: %s from %s", typ, remoteActor)

	switch typ {
	case "Follow":
		localUser := userFromActorURL(stringField(raw, "object"))
		if localUser == "" {
			localUser = user
		}
		if localUser == "" {
			log.Warnf("inbox: Follow without resolvable local user")
			w.WriteHeader(http.StatusAccepted)
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
	default:
		// Phase 2/3: translate Create/Like/Announce/Undo/Delete into Warpnet.
		log.Infof("inbox: %q acknowledged but not handled yet (skeleton)", typ)
		w.WriteHeader(http.StatusAccepted)
	}
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
	if err := g.followers.Add(localUser, follower{Actor: remoteActorURL, Inbox: inbox}); err != nil {
		log.Errorf("accept: persist follower %s: %v", remoteActorURL, err)
	}
	log.Infof("accept: Follow accepted (local=%s remote=%s)", localUser, remoteActorURL)
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
