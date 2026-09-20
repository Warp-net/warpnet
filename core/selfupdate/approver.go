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

type UserApprover struct {
	ctx      context.Context
	verdicts chan domain.UpdateInfo
	mx       *sync.RWMutex
	pending  domain.UpdateInfo
}

func NewUserApprover(ctx context.Context) *UserApprover {
	return &UserApprover{
		ctx:      ctx,
		verdicts: make(chan domain.UpdateInfo),
		mx:       new(sync.RWMutex),
	}
}

func (a *UserApprover) IsUpdateAllowed(info domain.UpdateInfo) bool {
	if a == nil {
		return false
	}
	a.mx.Lock()
	a.pending = info
	a.mx.Unlock()

	defer func() {
		a.mx.Lock()
		a.pending = domain.UpdateInfo{}
		a.mx.Unlock()
	}()

	log.Infof("selfupdate: waiting for permission to update %s -> %s", info.CurrentVersion, info.NewVersion)

	select {
	case <-a.ctx.Done():
		return false
	case verdict := <-a.verdicts:
		return verdict.IsAllowed
	}
}

func (a *UserApprover) GetPendingUpdate() domain.UpdateInfo {
	if a == nil {
		return domain.UpdateInfo{}
	}
	a.mx.RLock()
	defer a.mx.RUnlock()
	return a.pending
}

func (a *UserApprover) AnswerUpdate(isAllowed bool) {
	if a == nil {
		return
	}
	select {
	case a.verdicts <- domain.UpdateInfo{IsAllowed: isAllowed}:
	default:
		log.Warnln("selfupdate: no release is waiting for an answer")
	}
}
