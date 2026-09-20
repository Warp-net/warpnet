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

package selfupdate

import (
	"context"
	"sync"

	"github.com/Warp-net/warpnet/domain"
	log "github.com/sirupsen/logrus"
)

// UpdateGate holds a release back until the owner of the node allows it. The
// verdict travels through a channel, the way the login handshake reports a
// started node: the update service blocks on the channel while the frontend is
// asked, and resumes with whatever comes back.
type UpdateGate struct {
	ctx      context.Context
	verdicts chan domain.UpdateInfo
	mx       *sync.RWMutex
	pending  domain.UpdateInfo
}

func NewUpdateGate(ctx context.Context) *UpdateGate {
	return &UpdateGate{
		ctx: ctx,
		// unbuffered: an answer nobody waits for is dropped instead of being
		// kept for the next release
		verdicts: make(chan domain.UpdateInfo),
		mx:       new(sync.RWMutex),
	}
}

// IsUpdateAllowed publishes the release for the frontend to ask about and blocks
// until the user answers it. A node shutting down leaves the release unanswered,
// which reads as a refusal.
func (g *UpdateGate) IsUpdateAllowed(info domain.UpdateInfo) bool {
	if g == nil {
		return false
	}
	g.mx.Lock()
	g.pending = info
	g.mx.Unlock()

	defer func() {
		g.mx.Lock()
		g.pending = domain.UpdateInfo{}
		g.mx.Unlock()
	}()

	log.Infof("selfupdate: waiting for permission to update %s -> %s", info.CurrentVersion, info.NewVersion)

	select {
	case <-g.ctx.Done():
		return false
	case verdict := <-g.verdicts:
		return verdict.IsAllowed
	}
}

// Pending returns the release waiting for an answer. A zero value means nothing
// is waiting.
func (g *UpdateGate) Pending() domain.UpdateInfo {
	if g == nil {
		return domain.UpdateInfo{}
	}
	g.mx.RLock()
	defer g.mx.RUnlock()
	return g.pending
}

// Answer hands the user's verdict to the waiting update service. An answer to a
// release nobody is waiting for - a second click, a stale dashboard tab - is
// dropped.
func (g *UpdateGate) Answer(isAllowed bool) {
	if g == nil {
		return
	}
	select {
	case g.verdicts <- domain.UpdateInfo{IsAllowed: isAllowed}:
	default:
		log.Warnln("selfupdate: no release is waiting for an answer")
	}
}
