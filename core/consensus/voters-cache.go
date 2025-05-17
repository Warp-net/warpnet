/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package consensus

import (
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/hashicorp/raft"
	log "github.com/sirupsen/logrus"
	"sync"
	"time"
)

var errVoterNotFound = warpnet.WarpError("consensus: voter not found")

type voterTimed struct {
	voter   raft.Server
	addedAt time.Time
}

type votersCache struct {
	mutex *sync.RWMutex
	m     map[raft.ServerID]voterTimed
}

func newVotersCache() *votersCache {
	pc := &votersCache{
		mutex: new(sync.RWMutex),
		m:     make(map[raft.ServerID]voterTimed),
	}

	return pc
}

func (d *votersCache) addVoter(key raft.ServerID, srv raft.Server) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.m[key] = voterTimed{srv, time.Now()}
}

var errTooSoonToRemoveVoter = warpnet.WarpError("consensus:too soon to remove voter")

func (d *votersCache) removeVoter(key raft.ServerID) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	v, ok := d.m[key]
	if !ok {
		return nil
	}

	if time.Since(v.addedAt) < (time.Minute * 5) {
		return errTooSoonToRemoveVoter // flapping prevention
	}

	delete(d.m, key)
	return nil
}

func (d *votersCache) getVoter(key raft.ServerID) (_ raft.Server, err error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	v, ok := d.m[key]
	if !ok {
		return raft.Server{}, errVoterNotFound
	}

	return v.voter, nil
}

func (d *votersCache) print() {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	log.Info("consensus: voters list in cache:")
	for k := range d.m {
		fmt.Printf("========== %s", k)
	}
}
