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
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
)

// followerStore records, per local (bridged) user, the remote Fediverse actors
// that follow them. The production path (nodeFollowerStore) keeps the follow
// graph in Warpnet via the existing follow routes; fileFollowerStore is a local
// dev fallback when no Warpnet node is configured. Either way the gateway keeps
// only keys, not Warpnet content.
type followerStore interface {
	Add(localUser, actorURL string) error
	List(localUser string) ([]string, error)
}

// apFollowerPrefix tags follower ids that encode a remote ActivityPub actor, so
// AP followers can be told apart from native Warpnet user ids (which are valid
// base64url too). The ':' is safe as a key segment — the datastore delimiter is
// '/'.
const apFollowerPrefix = "ap:"

var errNotAPFollower = errors.New("not an AP follower id")

// encodeActorID encodes a remote actor URL into a Warpnet follower id that is
// safe as a datastore key segment (base64url has no '/') and reversible, so the
// gateway can recover the actor URL from a follower list.
func encodeActorID(actorURL string) string {
	return apFollowerPrefix + base64.RawURLEncoding.EncodeToString([]byte(actorURL))
}

func decodeActorID(id string) (string, error) {
	enc, ok := strings.CutPrefix(id, apFollowerPrefix)
	if !ok {
		return "", errNotAPFollower
	}
	bt, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	return string(bt), nil
}

// fileFollowerStore is a JSON-backed dev fallback used only when the gateway has
// no Warpnet node connection.
type fileFollowerStore struct {
	mu   sync.RWMutex
	path string
	data map[string][]string // localUser -> actor URLs
}

func newFileFollowerStore(path string) (*fileFollowerStore, error) {
	s := &fileFollowerStore{path: path, data: map[string][]string{}}
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

func (s *fileFollowerStore) Add(localUser, actorURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Contains(s.data[localUser], actorURL) {
		return nil
	}
	s.data[localUser] = append(s.data[localUser], actorURL)
	bt, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, bt, 0o600) //#nosec G703 -- operator-provided path
}

func (s *fileFollowerStore) List(localUser string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.data[localUser]
	out := make([]string, len(src))
	copy(out, src)
	return out, nil
}
