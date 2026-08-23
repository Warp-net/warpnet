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

package crdt

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/rating"
	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	log "github.com/sirupsen/logrus"
)

// NewCRDTRatingStore creates a new CRDT-based node rating store.
//
// The three node types differ only in the datastore they hand over: a
// member has a Badger-backed repo, a relay and a moderator hand over
// the in-memory map datastore they already build. Both are replicated
// the same way — which is the point of putting rating on a CRDT at
// all, since a node with no disk gets its view back from the DAG after
// a restart and cannot recover it any other way.
func NewCRDTRatingStore(
	ctx context.Context,
	broadcaster Broadcaster,
	datastore CRDTStorer,
	node host.Host,
	router CRDTRouter,
	privKey ed25519.PrivateKey,
	nodeType string,
	shadow rating.ShadowReporter,
) (*rating.Store, error) {
	if node == nil || datastore == nil {
		return nil, fmt.Errorf("rating: incomplete dependencies") //nolint:err113
	}

	mode, err := rating.ParseMode(config.Config().Node.RatingMode)
	if err != nil {
		// A typo in a flag must not silently arm enforcement.
		log.Warnf("rating: %v, falling back to shadow", err)
		mode = rating.ModeShadow
	}

	open := func(hooks rating.Hooks) (rating.Datastore, error) {
		return NewDatastore(ctx, broadcaster, datastore, node, router, DatastoreHooks{
			Put:    func(k ds.Key, v []byte) { hooks.Put(k.String(), v) },
			Delete: func(k ds.Key) { hooks.Delete(k.String()) },
		})
	}

	store, err := rating.NewStore(rating.Config{
		Ctx:        ctx,
		Self:       node.ID(),
		PrivKey:    privKey,
		Dimensions: rating.DimensionsFor(nodeType),
		Mode:       mode,
		Acquainted: connectionAge{host: node},
		Shadow:     shadow,
	}, open)
	if err != nil {
		return nil, err
	}

	log.Infof("rating: store started in %s mode for a %s node", mode, nodeType)
	return store, nil
}

// connectionAge answers how long we have been connected to a peer,
// which is what gates whether a remote observer's records count. It
// reads libp2p's own connection stats rather than keeping a second
// bookkeeping of the same thing.
type connectionAge struct {
	host host.Host
}

func (c connectionAge) ConnectedSince(id peer.ID) (time.Time, bool) {
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
