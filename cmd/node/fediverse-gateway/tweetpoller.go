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

	log "github.com/sirupsen/logrus"
)

const tweetPollInterval = 60 * time.Second

// tweetPoller watches the bridged owner's Warpnet tweets and federates new
// original top-level posts to Fediverse followers via publish. It is stateless
// across restarts: at startup it marks existing tweets as seen, so only posts
// created afterwards are federated (no replaying history on every restart).
//
// TODO: the seen set grows unbounded; bound it (or track a cursor/timestamp)
// once volume warrants. Subscribing to the owner's gossip would replace polling.
type tweetPoller struct {
	req      nodeRequester
	owner    string
	interval time.Duration
	seen     map[string]struct{}
	publish  func(ctx context.Context, owner string, t tweet)
}

func newTweetPoller(req nodeRequester, owner string, publish func(context.Context, string, tweet)) *tweetPoller {
	return &tweetPoller{
		req:      req,
		owner:    owner,
		interval: tweetPollInterval,
		seen:     map[string]struct{}{},
		publish:  publish,
	}
}

func (p *tweetPoller) run(ctx context.Context) {
	for _, t := range p.fetch() { // seed: don't replay history
		p.seen[t.Id] = struct{}{}
	}
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *tweetPoller) poll(ctx context.Context) {
	for _, t := range p.fetch() {
		if _, ok := p.seen[t.Id]; ok {
			continue
		}
		p.seen[t.Id] = struct{}{}
		if publishableTweet(t, p.owner) {
			p.publish(ctx, p.owner, t)
		}
	}
}

func (p *tweetPoller) fetch() []tweet {
	bt, err := p.req.request(routeGetTweets, getAllTweetsEvent{UserId: p.owner})
	if err != nil {
		log.Errorf("poller: get tweets for %s: %v", p.owner, err)
		return nil
	}
	var resp tweetsResponse
	if jerr := json.Unmarshal(bt, &resp); jerr != nil {
		log.Errorf("poller: decode tweets: %v", jerr)
		return nil
	}
	return resp.Tweets
}

// publishableTweet reports whether a tweet should be federated outbound: an
// original top-level post authored by the owner. Retweets and replies are
// skipped for now (inReplyTo / Announce mapping is a later step).
func publishableTweet(t tweet, owner string) bool {
	if t.UserId != owner || t.RetweetedBy != nil {
		return false
	}
	return t.ParentId == nil || *t.ParentId == ""
}
