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

	"github.com/Warp-net/warpnet/core/rating"
	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	log "github.com/sirupsen/logrus"
)

func NewCRDTRatingStore(
	ctx context.Context,
	crdtStore *Store,
	node host.Host,
	privKey ed25519.PrivateKey,
	nodeType string,
) (*rating.Store, error) {
	if node == nil || crdtStore == nil {
		return nil, fmt.Errorf("rating: incomplete dependencies") //nolint:err113
	}

	open := func(hooks rating.Hooks) (rating.Storer, error) {
		crdtStore.OnPut(func(k ds.Key, v []byte) { hooks.Put(k.String(), v) })
		crdtStore.OnDelete(func(k ds.Key) { hooks.Delete(k.String()) })
		return crdtStore, nil
	}

	store, err := rating.NewStore(rating.Config{
		Ctx:        ctx,
		Self:       node.ID(),
		PrivKey:    privKey,
		Dimensions: rating.DimensionsFor(nodeType),
		Acquainted: connectionAge{host: node},
	}, open)
	if err != nil {
		return nil, err
	}

	log.Infof("rating: store started for a %s node", nodeType)
	return store, nil
}

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
	oldest := time.Time{}
	for _, conn := range conns {
		opened := conn.Stat().Opened
		if oldest.IsZero() || opened.Before(oldest) {
			oldest = opened
		}
	}
	return oldest, !oldest.IsZero()
}
