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

// Package trust keeps the owner's trust circle: the users this node
// vouches for through its owner's follow graph. It only ever changes how
// content is shown, never whether it is carried. See docs/TRUST_CIRCLE.md.
package trust

import (
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/bridge"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const ErrClosed = warpnet.WarpError("trust: circle closed")

const (
	// threshold is the score a stranger needs: one vouching mutual with at
	// most 50 mutuals, or several bigger ones. To be set from a mainnet
	// measurement.
	threshold = 0.02
	// maxListed bounds the mutuals kept per mutual. Past it a mutual vouches
	// with less than 1/maxListed per user, which cannot reach threshold
	// alone, so its members are not kept.
	maxListed                = 2000
	pageLimit         uint64 = 1000
	firstRefreshDelay        = time.Minute
	refreshInterval          = 6 * time.Hour
	refreshJitter            = time.Hour
	// newMutualsInterval is how soon a new mutual starts vouching: its
	// listing is read on the next tick instead of the next full refresh.
	newMutualsInterval = 5 * time.Minute
	requestPause       = 2 * time.Second
)

type FollowsReader interface {
	GetFollowings(userId string, limit *uint64, cursor *string) ([]domain.ID, string, error)
	IsFollowing(ownerId, otherUserId string) bool
	IsFollower(ownerId, otherUserId string) bool
}

type UsersReader interface {
	Get(userId string) (domain.User, error)
}

type NodeStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
}

// listing is one mutual's own mutuals, as its node lists them.
type listing struct {
	mutuals map[string]struct{}
	degree  int
}

type Circle struct {
	ownerId  string
	follows  FollowsReader
	users    UsersReader
	streamer NodeStreamer
	pause    time.Duration

	mu       sync.RWMutex
	listings map[string]listing // mutual -> its listing; replaced whole on refresh

	stop      chan struct{}
	closeOnce sync.Once
	loop      sync.WaitGroup
}

func NewCircle(
	ownerId string,
	follows FollowsReader,
	users UsersReader,
	streamer NodeStreamer,
) *Circle {
	return &Circle{
		ownerId:  ownerId,
		follows:  follows,
		users:    users,
		streamer: streamer,
		pause:    requestPause,
		listings: map[string]listing{},
		stop:     make(chan struct{}),
	}
}

// Trust places a user in the circle. The direct part is read live, so a
// follow or an unfollow counts at once; the second hop is as fresh as the
// last refresh, minus vouchers that are no longer mutuals.
func (r *Circle) Trust(userId string) domain.Trust {
	if userId == r.ownerId || r.follows.IsFollowing(r.ownerId, userId) {
		return domain.Trust{IsTrusted: true, Score: 1}
	}

	r.mu.RLock()
	listings := r.listings
	r.mu.RUnlock()

	var (
		score    float64
		vouchers int
	)
	for mutualId, l := range listings {
		if _, ok := l.mutuals[userId]; !ok {
			continue
		}
		if !r.isMutual(mutualId) {
			continue
		}
		score += 1 / float64(l.degree)
		vouchers++
	}
	score = min(score, 1)
	return domain.Trust{IsTrusted: score >= threshold, Score: score, Vouchers: vouchers}
}

// Start refreshes the second hop until Close: in full every few hours, and
// in between only for mutuals it has not listed yet.
func (r *Circle) Start() {
	r.loop.Go(r.run)
}

// Close stops the refresh loop and waits for it. A refresh under way stops
// before its next mutual.
func (r *Circle) Close() {
	r.closeOnce.Do(func() { close(r.stop) })
	r.loop.Wait()
}

func (r *Circle) run() {
	full := time.NewTimer(firstRefreshDelay)
	defer full.Stop()
	newMutuals := time.NewTicker(newMutualsInterval)
	defer newMutuals.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-newMutuals.C:
			if err := r.RefreshNew(); err != nil {
				log.Warnf("trust: listing new mutuals: %v", err)
			}
		case <-full.C:
			if err := r.Refresh(); err != nil {
				log.Warnf("trust: refreshing circle: %v", err)
			}
			full.Reset(refreshInterval + rand.N(refreshJitter)) //#nosec
		}
	}
}

// Refresh rebuilds the second hop from each mutual's own lists. A mutual
// that cannot be reached keeps what it listed last time.
func (r *Circle) Refresh() error {
	mutuals, err := r.mutuals()
	if err != nil {
		return err
	}

	r.mu.RLock()
	previous := r.listings
	r.mu.RUnlock()

	listings := make(map[string]listing, len(mutuals))
	err = r.fill(listings, mutuals, previous)

	r.mu.Lock()
	r.listings = listings
	r.mu.Unlock()
	return err
}

// RefreshNew lists only the mutuals that have no listing yet, so a new
// mutual vouches within minutes rather than at the next full refresh.
func (r *Circle) RefreshNew() error {
	mutuals, err := r.mutuals()
	if err != nil {
		return err
	}

	r.mu.RLock()
	previous := r.listings
	r.mu.RUnlock()

	unlisted := slices.DeleteFunc(mutuals, func(id string) bool {
		_, ok := previous[id]
		return ok
	})
	if len(unlisted) == 0 {
		return nil
	}

	listings := maps.Clone(previous)
	err = r.fill(listings, unlisted, previous)

	r.mu.Lock()
	r.listings = listings
	r.mu.Unlock()
	return err
}

// fill reads each mutual's listing into listings, pausing between mutuals.
// A mutual that cannot be reached keeps its previous listing, if any.
func (r *Circle) fill(listings map[string]listing, mutuals []string, previous map[string]listing) error {
	var errs []error
	for i, mutualId := range mutuals {
		if i > 0 {
			select {
			case <-r.stop:
				return ErrClosed
			case <-time.After(r.pause):
			}
		}
		l, err := r.listing(mutualId)
		if err != nil {
			if !errors.Is(err, warpnet.ErrNodeIsOffline) {
				errs = append(errs, fmt.Errorf("listing %s: %w", mutualId, err))
			}
			if old, ok := previous[mutualId]; ok {
				listings[mutualId] = old
			}
			continue
		}
		listings[mutualId] = l
	}
	return errors.Join(errs...)
}

func (r *Circle) isMutual(userId string) bool {
	return r.follows.IsFollowing(r.ownerId, userId) && r.follows.IsFollower(r.ownerId, userId)
}

// mutuals are the owner's mutual follows, bridged accounts excluded: their
// lists come from the gateway, not from the person.
func (r *Circle) mutuals() ([]string, error) {
	var (
		ids    []string
		cursor string
		limit  = pageLimit
	)
	for {
		page, next, err := r.follows.GetFollowings(r.ownerId, &limit, &cursor)
		if err != nil {
			return nil, err
		}
		for _, id := range page {
			if id == r.ownerId || bridge.IsBridgedUserID(id) {
				continue
			}
			if r.follows.IsFollower(r.ownerId, id) {
				ids = append(ids, id)
			}
		}
		if uint64(len(page)) < limit {
			return ids, nil
		}
		cursor = next
	}
}

func (r *Circle) listing(userId string) (listing, error) {
	user, err := r.users.Get(userId)
	if err != nil {
		return listing{}, err
	}
	if user.NodeId == "" {
		return listing{}, warpnet.ErrNodeIsOffline
	}

	l := listing{mutuals: map[string]struct{}{}}
	var (
		cursor string
		limit  = pageLimit
	)
	for l.degree <= maxListed {
		data, err := r.streamer.GenericStream(user.NodeId, event.PUBLIC_GET_FOLLOWINGS, event.GetFollowingsEvent{
			UserId:       userId,
			Limit:        &limit,
			Cursor:       &cursor,
			IsMutualOnly: true,
		})
		if err != nil {
			return listing{}, err
		}
		var possibleError event.ResponseError
		if _ = json.Unmarshal(data, &possibleError); possibleError.Message != "" {
			return listing{}, possibleError
		}
		var page event.FollowingsResponse
		if err := json.Unmarshal(data, &page); err != nil {
			return listing{}, err
		}
		if !page.IsMutualOnly { // a node too old to tell its mutuals vouches for nobody
			return listing{}, nil
		}
		for _, id := range page.Followings {
			l.degree++
			if !bridge.IsBridgedUserID(id) {
				l.mutuals[id] = struct{}{}
			}
		}
		if uint64(len(page.Followings)) < limit {
			break
		}
		cursor = page.Cursor
	}
	if l.degree > maxListed {
		l.mutuals = nil
	}
	return l, nil
}
