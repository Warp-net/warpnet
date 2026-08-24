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

package rating

import (
	"sync/atomic"

	"github.com/Warp-net/warpnet/core/warpnet"
	log "github.com/sirupsen/logrus"
)

// Handle is the node's rating as every other subsystem sees it: one
// object created with the node and shared by middleware, discovery,
// gossip and the stream handlers. It owns everything an enforcement
// point would otherwise have to reimplement — the no-store default,
// the swap of the store built later (after gossip, and on a moderator
// after the node is already serving), the fail-open policy on a read
// failure, and the logging of refused records.
type Handle struct {
	rater atomic.Pointer[Rater]
}

func NewHandle() *Handle { return &Handle{} }

// Set attaches the store. Safe while serving: readers swap atomically.
func (h *Handle) Set(r Rater) {
	if h == nil || r == nil {
		return
	}
	h.rater.Store(&r)
}

// Record charges an offence. A refusal means this node reported
// something its role cannot witness — a bug at the call site, not
// misbehaviour by the peer — so it is logged, not returned: enforcement
// points sit on request paths with no caller to hand it to.
func (h *Handle) Record(subject warpnet.WarpPeerID, k Kind) {
	if h == nil {
		return
	}
	r := h.rater.Load()
	if r == nil {
		return
	}
	if err := (*r).Record(subject, k); err != nil {
		log.Warnf("rating: recording %s for %s: %v", k, subject, err)
	}
}

// Band is a peer's standing for an enforcement decision. Fail-open by
// policy: no store yet, or a store that cannot be read, is BandTrusted
// — a peer whose evidence we cannot see must not be penalised for it.
func (h *Handle) Band(subject warpnet.WarpPeerID) Band {
	if h == nil {
		return BandTrusted
	}
	r := h.rater.Load()
	if r == nil {
		return BandTrusted
	}
	band, err := (*r).Band(subject)
	if err != nil {
		log.Warnf("rating: reading standing of %s: %v", subject, err)
		return BandTrusted
	}
	return band
}
