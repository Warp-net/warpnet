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
	"encoding/json"
	"time"

	"github.com/Warp-net/warpnet/event"
	log "github.com/sirupsen/logrus"
)

const followPollInterval = 30 * time.Second

// followPoller federates the owner's *outbound* follows. It polls the owner's
// followings; those that are Fediverse actors (ap:-encoded ids, i.e. accounts
// the gateway ingested) get a signed Follow delivered to their inbox, and an
// Undo(Follow) when the owner unfollows. The first poll only records a baseline
// (history isn't replayed), matching the tweet poller.
type followPoller struct {
	req        nodeRequester
	owner      string
	onFollow   func(actorURL string)
	onUnfollow func(actorURL string)
	interval   time.Duration
	known      map[string]bool // ap: actor URLs already federated; nil until first poll
}

func newFollowPoller(req nodeRequester, owner string, onFollow, onUnfollow func(string)) *followPoller {
	return &followPoller{
		req:        req,
		owner:      owner,
		onFollow:   onFollow,
		onUnfollow: onUnfollow,
		interval:   followPollInterval,
	}
}

func (p *followPoller) run(ctx context.Context) {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		if err := p.poll(); err != nil {
			log.Warnf("follow poll: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// poll reads the owner's followings and fires onFollow/onUnfollow for added and
// removed Fediverse actors. The first call only seeds the baseline.
func (p *followPoller) poll() error {
	bt, err := p.req.request(event.PUBLIC_GET_FOLLOWINGS, event.GetFollowingsEvent{UserId: p.owner})
	if err != nil {
		return err
	}
	var resp event.FollowingsResponse
	if err := json.Unmarshal(bt, &resp); err != nil {
		return err
	}

	current := make(map[string]bool)
	for _, id := range resp.Followings {
		if actorURL, derr := decodeActorID(id); derr == nil {
			current[actorURL] = true
		}
	}

	if p.known == nil { // baseline only — don't replay existing follows
		p.known = current
		return nil
	}
	for actorURL := range current {
		if !p.known[actorURL] {
			p.onFollow(actorURL)
		}
	}
	for actorURL := range p.known {
		if !current[actorURL] {
			p.onUnfollow(actorURL)
		}
	}
	p.known = current
	return nil
}

// sendFollow delivers a signed Follow from localUser to a remote Fediverse actor.
func (g *gateway) sendFollow(localUser, remoteActorURL string) {
	g.deliverFollow(localUser, remoteActorURL, false)
}

// sendUndoFollow delivers a signed Undo(Follow) (the owner unfollowed the actor).
func (g *gateway) sendUndoFollow(localUser, remoteActorURL string) {
	g.deliverFollow(localUser, remoteActorURL, true)
}

func (g *gateway) deliverFollow(localUser, remoteActorURL string, undo bool) {
	ctx, cancel := context.WithTimeout(context.Background(), acceptDeliveryTimeout)
	defer cancel()

	inbox, err := g.remoteInbox(ctx, remoteActorURL)
	if err != nil {
		log.Errorf("follow: resolve inbox for %s: %v", remoteActorURL, err)
		return
	}

	actorID := g.actorID(localUser)
	follow := activity{
		Context: asContext,
		ID:      actorID + "#follow-" + randomToken(),
		Type:    typeFollow,
		Actor:   actorID,
		Object:  remoteActorURL,
	}
	doc := any(follow)
	if undo {
		doc = activity{
			Context: asContext,
			ID:      actorID + "#unfollow-" + randomToken(),
			Type:    typeUndo,
			Actor:   actorID,
			Object:  follow,
		}
	}

	if err := g.postSigned(ctx, localUser, inbox, doc); err != nil {
		log.Errorf("follow: deliver to %s (undo=%v): %v", remoteActorURL, undo, err)
		return
	}
	log.Infof("follow: delivered to %s (undo=%v)", remoteActorURL, undo)
}
