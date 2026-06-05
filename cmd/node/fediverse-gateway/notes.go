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
	"encoding/json"
	"html"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

const asPublic = "https://www.w3.org/ns/activitystreams#Public"

// buildNote renders a Warpnet tweet as an ActivityPub Note authored by
// localUser. The Note id is deterministic (.../statuses/{id}) so serveStatus
// can resolve it back to the tweet without local storage.
func (g *gateway) buildNote(localUser string, t tweet) note {
	actorID := g.actorID(localUser)
	followers := actorID + pathFollowers

	n := note{
		ID:           actorID + pathStatuses + t.Id,
		Type:         typeNote,
		AttributedTo: actorID,
		Content:      "<p>" + html.EscapeString(t.Text) + "</p>",
		Published:    t.CreatedAt.UTC().Format(time.RFC3339),
		To:           []string{asPublic},
		Cc:           []string{followers},
	}
	for _, key := range t.ImageKeys {
		n.Attachment = append(n.Attachment, attachment{
			Type: typeDocument,
			URL:  g.baseURL() + pathMedia + encodeMediaRef(t.UserId, key),
		})
	}
	return n
}

// buildCreateNote wraps a Warpnet tweet as an ActivityPub Create(Note) authored
// by localUser, addressed to the public and the author's followers.
func (g *gateway) buildCreateNote(localUser string, t tweet) activity {
	n := g.buildNote(localUser, t)
	return activity{
		Context: asContext,
		ID:      n.ID + "/activity",
		Type:    typeCreate,
		Actor:   n.AttributedTo,
		Object:  n,
		To:      n.To,
		Cc:      n.Cc,
	}
}

// serveStatus resolves one of our deterministic Note ids back to the Warpnet
// tweet (PUBLIC_GET_TWEET) and renders it as a standalone Note, so peers can
// dereference, reply to, and boost the gateway's posts. Needs a node.
func (g *gateway) serveStatus(w http.ResponseWriter, r *http.Request, user, tweetID string) {
	if g.req == nil {
		http.NotFound(w, r)
		return
	}
	bt, err := g.req.request(routeGetTweet, getTweetEvent{
		TweetId: tweetID,
		UserId:  user,
	})
	if err != nil {
		log.Warnf("status: fetch %s/%s: %v", user, tweetID, err)
		http.NotFound(w, r)
		return
	}
	var t tweet
	if jerr := json.Unmarshal(bt, &t); jerr != nil || t.Id == "" {
		http.NotFound(w, r)
		return
	}
	n := g.buildNote(user, t)
	n.Context = asContext
	writeJSON(w, contentTypeAP, n)
}
