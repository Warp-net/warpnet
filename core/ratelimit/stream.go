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

package ratelimit

import (
	"strconv"
	"time"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	log "github.com/sirupsen/logrus"
)

const (
	streamBucketsSize = 4096
	streamBucketsTTL  = 10 * time.Minute
)

var (
	delivery  = PerMinute(200, 1200)
	media     = PerMinute(150, 600)
	messaging = PerMinute(60, 300)
	upload    = PerMinute(10, 30)
	report    = PerMinute(10, 30)
	pairing   = PerMinute(5, 15)
	gateway   = PerMinute(600, 6000)
)

// PeersRatings answers how much of a route a peer may spend, which is how a
// node serves a badly rated peer more slowly.
type PeersRatings interface {
	RateMultiplier(peerID warpnet.WarpPeerID) float64
}

// StreamLimiter is what a peer may spend on a node's routes: the budgets the
// owner set for reads and writes, the ones a route carries of its own, and
// what its rating leaves it of either.
type StreamLimiter struct {
	read, write Limit
	routes      map[string]Limit
	buckets     *Buckets
	ratings     PeersRatings
}

func NewStreamLimiter(s Settings, ratings PeersRatings) *StreamLimiter {
	s = s.WithDefaults()
	read := PerMinute(int64(s.StreamReadBurst), int64(s.StreamReadPerMinute))
	write := PerMinute(int64(s.StreamWriteBurst), int64(s.StreamWritePerMinute))

	return &StreamLimiter{
		read:  read,
		write: write,
		routes: map[string]Limit{
			event.PUBLIC_GET_IMAGE: media,
			event.PUBLIC_GET_VIDEO: media,

			event.PRIVATE_POST_UPLOAD_IMAGE: upload,
			event.PRIVATE_POST_UPLOAD_VIDEO: upload,

			event.PUBLIC_POST_TIMELINE:          delivery,
			event.PUBLIC_POST_MODERATION_RESULT: delivery,

			event.PUBLIC_POST_CHAT:    messaging,
			event.PUBLIC_POST_MESSAGE: messaging,

			event.PUBLIC_POST_IS_FOLLOWING:       read,
			event.PUBLIC_POST_IS_FOLLOWER:        read,
			event.PUBLIC_POST_VIEW:               read,
			event.PRIVATE_POST_NOTIFICATION_READ: read,

			event.PUBLIC_POST_REPORT: report,

			event.PRIVATE_POST_PAIR:          pairing,
			event.PUBLIC_POST_NODE_CHALLENGE: pairing,
		},
		buckets: NewBuckets(streamBucketsSize, streamBucketsTTL),
		ratings: ratings,
	}
}

func (l *StreamLimiter) Allow(route stream.WarpRoute, remotePeer warpnet.WarpPeerID) bool {
	if l == nil {
		return true
	}

	multiplier := l.multiplier(remotePeer)
	limit := l.route(route, remotePeer).MultipliedBy(multiplier)
	if multiplier < 1 {
		log.Infof(
			"ratelimit: rating leaves %s %d calls per minute on %s",
			remotePeer, limit.PerMinute(), route,
		)
	}
	// A peer whose standing moved does not keep the bucket it filled under
	// the old one.
	key := route.String() + "|" + remotePeer.String() + "|" + strconv.FormatFloat(multiplier, 'f', 2, 64)
	return l.buckets.Allow(key, limit)
}

func (l *StreamLimiter) Close() {
	if l == nil {
		return
	}
	l.buckets.Close()
}

func (l *StreamLimiter) route(route stream.WarpRoute, remotePeer warpnet.WarpPeerID) Limit {
	if remotePeer.String() == fediverse.GatewayNodeID() {
		return gateway
	}
	if limit, ok := l.routes[route.String()]; ok {
		return limit
	}
	if route.IsGet() {
		return l.read
	}
	return l.write
}

// multiplier is the share of a route a peer may spend. A node with no ratings
// serves every peer in full.
func (l *StreamLimiter) multiplier(peerID warpnet.WarpPeerID) float64 {
	if l.ratings == nil {
		return 1
	}
	return l.ratings.RateMultiplier(peerID)
}
