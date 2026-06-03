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
	"os"
	"sync"

	"github.com/Warp-net/warpnet/json"
)

// follower is a remote Fediverse follower of a local (bridged) user.
type follower struct {
	Actor string `json:"actor"` // remote actor URL
	Inbox string `json:"inbox"` // delivery inbox (sharedInbox or personal)
}

// followerStore persists remote followers per local user. The gateway runs as
// a separate process (chosen architecture), so it keeps its own small
// JSON-backed store; once the libp2p node connector lands this can be moved
// into Warpnet's followRepo.
type followerStore struct {
	mu   sync.RWMutex
	path string
	data map[string][]follower // localUser -> followers
}

func newFollowerStore(path string) (*followerStore, error) {
	s := &followerStore{path: path, data: map[string][]follower{}}
	bt, err := os.ReadFile(path) //#nosec G304 -- operator-provided path
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(bt) > 0 {
		if err := json.Unmarshal(bt, &s.data); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Add records a follower (idempotent by actor URL) and persists the store.
func (s *followerStore) Add(localUser string, f follower) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ex := range s.data[localUser] {
		if ex.Actor == f.Actor {
			return nil
		}
	}
	s.data[localUser] = append(s.data[localUser], f)
	return s.persistLocked()
}

// List returns a copy of localUser's followers.
func (s *followerStore) List(localUser string) []follower {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.data[localUser]
	out := make([]follower, len(src))
	copy(out, src)
	return out
}

func (s *followerStore) persistLocked() error {
	bt, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, bt, 0o600)
}
