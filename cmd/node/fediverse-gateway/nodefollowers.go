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
	"encoding/json"
)

// nodeRequester is the subset of nodeClient that the follower store needs.
type nodeRequester interface {
	request(route string, payload any) ([]byte, error)
}

// nodeFollowerStore keeps the AP follow graph in Warpnet by reusing the
// existing follow routes: PUBLIC_POST_FOLLOW records "remote actor follows
// owner" on the owner's node, PUBLIC_GET_FOLLOWERS reads them back. Remote
// actor URLs travel as base64url follower ids (see encodeActorID), so the
// gateway itself stores no follower state.
type nodeFollowerStore struct {
	req nodeRequester
}

func (s nodeFollowerStore) Add(localUser, actorURL string) error {
	_, err := s.req.request(routePostFollow, newFollowEvent{
		FollowerId:  encodeActorID(actorURL),
		FollowingId: localUser,
	})
	return err
}

func (s nodeFollowerStore) List(localUser string) ([]string, error) {
	bt, err := s.req.request(routeGetFollowers, getFollowersEvent{UserId: localUser})
	if err != nil {
		return nil, err
	}
	var resp followersResponse
	if err := json.Unmarshal(bt, &resp); err != nil {
		return nil, err
	}
	urls := make([]string, 0, len(resp.Followers))
	for _, id := range resp.Followers {
		actorURL, derr := decodeActorID(id)
		if derr != nil {
			continue // native Warpnet follower id, not an AP actor — skip
		}
		urls = append(urls, actorURL)
	}
	return urls, nil
}
