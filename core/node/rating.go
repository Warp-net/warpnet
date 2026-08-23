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

 WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
 Use at your own risk. The maintainers shall not be liable for any damages or data loss
 resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/crdt"
	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	log "github.com/sirupsen/logrus"
)

// RatingDeps is everything a rating store needs from its node. The
// three node types differ only in the datastore they hand over: a
// member node has a Badger-backed repo, a relay and a moderator hand
// over the in-memory map datastore they already build. Both are
// replicated the same way — which is the point of putting rating on a
// CRDT at all, since a node with no disk gets its view back from the
// DAG after a restart and cannot recover it any other way.
type RatingDeps struct {
	Ctx      context.Context
	Self     warpnet.WarpPeerID
	PrivKey  ed25519.PrivateKey
	NodeType string
	Host     warpnet.P2PNode
	Router   crdt.CRDTRouter
	Store    crdt.CRDTStorer
	Gossip   crdt.GossipPubSuber
	Shadow   rating.ShadowReporter
}

// NewRatingStore builds the rating store and its CRDT replica.
func NewRatingStore(d RatingDeps) (*rating.Store, error) {
	if d.Host == nil || d.Gossip == nil || d.Store == nil {
		return nil, fmt.Errorf("node: rating: incomplete dependencies") //nolint:err113
	}

	mode, err := rating.ParseMode(config.Config().Node.RatingMode)
	if err != nil {
		// A typo in a flag must not silently arm enforcement.
		log.Warnf("node: rating: %v, falling back to shadow", err)
		mode = rating.ModeShadow
	}

	broadcaster, err := crdt.NewGossipBroadcasterOn(d.Ctx, d.Gossip, crdt.RatingTopic)
	if err != nil {
		return nil, fmt.Errorf("node: rating: broadcaster: %w", err)
	}

	open := func(hooks rating.Hooks) (rating.Datastore, error) {
		return crdt.NewDatastore(d.Ctx, broadcaster, d.Store, d.Host, d.Router, crdt.DatastoreHooks{
			Put:    func(k ds.Key, v []byte) { hooks.Put(k.String(), v) },
			Delete: func(k ds.Key) { hooks.Delete(k.String()) },
		})
	}

	store, err := rating.NewStore(rating.Config{
		Ctx:        d.Ctx,
		Self:       d.Self,
		PrivKey:    d.PrivKey,
		Dimensions: rating.DimensionsFor(d.NodeType),
		Mode:       mode,
		Acquainted: connectionAge{host: d.Host},
		Shadow:     d.Shadow,
	}, open)
	if err != nil {
		return nil, err
	}

	log.Infof("node: rating: store started in %s mode for a %s node", mode, d.NodeType)
	return store, nil
}

// connectionAge answers how long we have been connected to a peer,
// which is what gates whether a remote observer's records count. It
// reads libp2p's own connection stats rather than keeping a second
// bookkeeping of the same thing.
type connectionAge struct {
	host warpnet.P2PNode
}

func (c connectionAge) ConnectedSince(id warpnet.WarpPeerID) (time.Time, bool) {
	if c.host == nil || c.host.Network() == nil {
		return time.Time{}, false
	}
	conns := c.host.Network().ConnsToPeer(id)
	if len(conns) == 0 {
		return time.Time{}, false
	}
	// The oldest live connection: a peer that reconnects should not
	// reset its acquaintance, but one that has genuinely just arrived
	// should not inherit any either.
	oldest := time.Time{}
	for _, conn := range conns {
		opened := conn.Stat().Opened
		if oldest.IsZero() || opened.Before(oldest) {
			oldest = opened
		}
	}
	return oldest, !oldest.IsZero()
}
