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

package database

import (
	"github.com/oklog/ulid/v2"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/domain"
	log "github.com/sirupsen/logrus"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/json"
)

var (
	ErrUserNotFound      = local_store.DBError("user not found")
	ErrUserAlreadyExists = local_store.DBError("user already exists")
)

const (
	UsersRepoName    = "/USERS"
	userSubNamespace = "USER"
	nodeSubNamespace = "NODE"

	defaultAverageRTT int64 = 125000

	DefaultWarpnetUserNetwork = "warpnet"
)

type UserStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
	Set(key local_store.DatabaseKey, value []byte) error
	Get(key local_store.DatabaseKey) ([]byte, error)
	Delete(key local_store.DatabaseKey) error
}

// NewUserNotifier records a "new user discovered" notification for the local
// owner when a previously-unknown user is first stored.
type NewUserNotifier interface {
	Add(not domain.Notification) error
}

type UserRepo struct {
	db UserStorer

	notifier    NewUserNotifier
	ownerUserId string
}

func NewUserRepo(db UserStorer) *UserRepo {
	return &UserRepo{db: db}
}

// NewUserRepoNotifying returns a UserRepo that, on first storing a genuinely
// new (non-owner) user, records a "new user discovered" notification for the
// owner via notifier.
func NewUserRepoNotifying(db UserStorer, notifier NewUserNotifier, ownerUserId string) *UserRepo {
	return &UserRepo{db: db, notifier: notifier, ownerUserId: ownerUserId}
}

// Create adds a new user to the database
func (repo *UserRepo) Create(user domain.User) (domain.User, error) {
	return repo.CreateWithTTL(user, math.MaxInt64)
}

func (repo *UserRepo) CreateWithTTL(user domain.User, ttl time.Duration) (domain.User, error) {
	if user.Id == "" {
		return user, local_store.DBError("user id is empty")
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}
	data, err := json.Marshal(user)
	if err != nil {
		return user, err
	}

	if user.RoundTripTime == 0 {
		user.RoundTripTime = defaultAverageRTT
	}
	if user.Network == "" {
		user.Network = DefaultWarpnetUserNetwork
	}

	rttRange := local_store.RangePrefix(strconv.FormatInt(user.RoundTripTime, 10))

	fixedKey := local_store.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		AddRange(local_store.FixedRangeKey).
		AddParentId(user.Id).
		Build()

	_, err = repo.db.Get(fixedKey)
	if !local_store.IsNotFoundError(err) {
		return user, ErrUserAlreadyExists
	}

	sortableKey := local_store.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		AddRange(rttRange).
		AddParentId(user.Id).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return user, err
	}
	defer txn.Rollback()

	if user.NodeId != "" {
		nodeUserKey := local_store.NewPrefixBuilder(UsersRepoName).
			AddSubPrefix(nodeSubNamespace).
			AddRootID(user.NodeId).
			Build()
		if err = txn.SetWithTTL(nodeUserKey, sortableKey.Bytes(), ttl); err != nil {
			return user, err
		}
	}

	if err = txn.SetWithTTL(fixedKey, sortableKey.Bytes(), ttl); err != nil {
		return user, err
	}
	if err = txn.SetWithTTL(sortableKey, data, ttl); err != nil {
		return user, err
	}
	if err = txn.Commit(); err != nil {
		return user, err
	}
	repo.notifyNewUser(user)
	return user, nil
}

// notifyNewUser records a "new user discovered" notification for the owner.
// No-op on a plain repo (nil notifier) or for the owner's own record.
// Best-effort: a store error is logged, not returned.
func (repo *UserRepo) notifyNewUser(user domain.User) {
	if repo.notifier == nil || user.Id == repo.ownerUserId {
		return
	}
	name := user.Username
	if name == "" {
		name = user.Id
	}
	if err := repo.notifier.Add(domain.Notification{
		Type:   domain.NotificationNewUserType,
		Text:   name + " joined Warpnet",
		UserId: repo.ownerUserId,
	}); err != nil {
		log.Warnf("user repo: notify new user: %v", err)
	}
}

func (repo *UserRepo) Update(userId string, newUser domain.User) (domain.User, error) {
	var existingUser domain.User

	fixedKey := local_store.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		AddRange(local_store.FixedRangeKey).
		AddParentId(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return existingUser, err
	}
	defer txn.Rollback()

	sortableKeyBytes, err := txn.Get(fixedKey) // GET IS HERE!
	if err != nil {
		return existingUser, err
	}

	data, err := txn.Get(local_store.DatabaseKey(sortableKeyBytes))
	if local_store.IsNotFoundError(err) {
		return existingUser, ErrUserNotFound
	}
	if err != nil {
		return existingUser, err
	}

	err = json.Unmarshal(data, &existingUser)
	if err != nil {
		return existingUser, err
	}

	if newUser.Birthdate != "" {
		existingUser.Birthdate = newUser.Birthdate
	}
	if newUser.Bio != "" {
		existingUser.Bio = newUser.Bio
	}
	if newUser.AvatarKey != "" {
		existingUser.AvatarKey = newUser.AvatarKey
	}
	if newUser.Username != "" {
		existingUser.Username = newUser.Username
	}
	if newUser.BackgroundImageKey != "" {
		existingUser.BackgroundImageKey = newUser.BackgroundImageKey
	}
	if newUser.Website != nil {
		existingUser.Website = newUser.Website
	}
	if newUser.NodeId != "" {
		existingUser.NodeId = newUser.NodeId
	}
	if newUser.Network != "" {
		existingUser.Network = newUser.Network
	}
	// Only ever set, never clear: a stale "" from a peer that doesn't report
	// the role must not wipe a role already learned from the node's NodeInfo.
	if newUser.Role != "" {
		existingUser.Role = newUser.Role
	}
	if newUser.Moderation != nil {
		if existingUser.Moderation == nil {
			existingUser.Moderation = newUser.Moderation
		} else {
			existingUser.Moderation.IsModerated = newUser.Moderation.IsModerated
			existingUser.Moderation.Reason = newUser.Moderation.Reason
			existingUser.Moderation.IsOk = newUser.Moderation.IsOk
			existingUser.Moderation.TimeAt = newUser.Moderation.TimeAt

			existingUser.Moderation.Strikes += newUser.Moderation.Strikes
		}
	}
	existingUser.RoundTripTime = newUser.RoundTripTime
	existingUser.IsOffline = newUser.IsOffline
	now := time.Now()
	existingUser.UpdatedAt = &now

	bt, err := json.Marshal(existingUser)
	if err != nil {
		return existingUser, err
	}
	if err = txn.Set(fixedKey, sortableKeyBytes); err != nil {
		return existingUser, err
	}
	if err = txn.Set(local_store.DatabaseKey(sortableKeyBytes), bt); err != nil {
		return existingUser, err
	}

	if newUser.NodeId != "" {
		nodeUserKey := local_store.NewPrefixBuilder(UsersRepoName).
			AddSubPrefix(nodeSubNamespace).
			AddRootID(newUser.NodeId).
			Build()
		if err = txn.Set(nodeUserKey, sortableKeyBytes); err != nil {
			return existingUser, err
		}
	}
	return existingUser, txn.Commit()
}

// Get retrieves a user by their ID
func (repo *UserRepo) Get(userId string) (user domain.User, err error) {
	if userId == "" {
		return user, ErrUserNotFound
	}
	fixedKey := local_store.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		AddRange(local_store.FixedRangeKey).
		AddParentId(userId).
		Build()
	sortableKeyBytes, err := repo.db.Get(fixedKey)
	if local_store.IsNotFoundError(err) {
		return user, ErrUserNotFound
	}
	if err != nil {
		return user, err
	}

	data, err := repo.db.Get(local_store.DatabaseKey(sortableKeyBytes))
	if local_store.IsNotFoundError(err) {
		return user, ErrUserNotFound
	}
	if err != nil {
		return user, err
	}

	err = json.Unmarshal(data, &user)
	if err != nil {
		return user, err
	}

	return user, nil
}

func (repo *UserRepo) GetByNodeID(nodeID string) (user domain.User, err error) {
	if nodeID == "" {
		return user, ErrUserNotFound
	}
	nodeUserKey := local_store.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(nodeSubNamespace).
		AddRootID(nodeID).
		Build()

	sortableKeyBytes, err := repo.db.Get(nodeUserKey)
	if local_store.IsNotFoundError(err) {
		return user, ErrUserNotFound
	}
	if err != nil {
		return user, err
	}

	data, err := repo.db.Get(local_store.DatabaseKey(sortableKeyBytes))
	if local_store.IsNotFoundError(err) {
		return user, ErrUserNotFound
	}
	if err != nil {
		return user, err
	}

	err = json.Unmarshal(data, &user)
	if err != nil {
		return user, err
	}

	return user, nil
}

// Delete removes a user by their ID
func (repo *UserRepo) Delete(userId string) error {
	fixedKey := local_store.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		AddRange(local_store.FixedRangeKey).
		AddParentId(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	sortableKeyBytes, err := txn.Get(fixedKey)
	if local_store.IsNotFoundError(err) {
		return nil
	}
	if err != nil {
		return err
	}

	data, err := txn.Get(local_store.DatabaseKey(sortableKeyBytes))
	if err != nil {
		return err
	}

	var u domain.User
	err = json.Unmarshal(data, &u)
	if err != nil {
		return err
	}

	if err = txn.Delete(fixedKey); err != nil {
		return err
	}
	if err = txn.Delete(local_store.DatabaseKey(sortableKeyBytes)); err != nil {
		return err
	}
	if u.NodeId != "" {
		nodeUserKey := local_store.NewPrefixBuilder(UsersRepoName).
			AddSubPrefix(nodeSubNamespace).
			AddRootID(u.NodeId).
			Build()

		if err = txn.Delete(nodeUserKey); err != nil {
			return err
		}
	}
	return txn.Commit()
}

func (repo *UserRepo) List(limit *uint64, cursor *string) ([]domain.User, string, error) {
	prefix := local_store.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	if err = txn.Commit(); err != nil {
		return nil, "", err
	}

	users := make([]domain.User, 0, len(items))
	for _, item := range items {
		var u domain.User
		err = json.Unmarshal(item.Value, &u)
		if err != nil {
			return nil, "", err
		}
		users = append(users, u)
	}

	return users, cur, nil
}

// Search returns users whose Username, Bio, or NodeId contains the
// (lower-cased) query. This is a server-side scan-and-filter — it
// preserves the cursor for incremental pages but still touches every
// matching record in the prefix range. A true substring index belongs
// here later; the API shape is forward-compatible.
func (repo *UserRepo) Search(query string, limit *uint64, cursor *string) ([]domain.User, string, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, "", local_store.DBError("empty search query")
	}

	prefix := local_store.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		Build()

	want := uint64(20)
	if limit != nil && *limit > 0 {
		want = *limit
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	scanPage := want * 4

	hits := make([]domain.User, 0, want)
	pageCursor := ""
	if cursor != nil {
		pageCursor = *cursor
	}

	for {
		c := pageCursor
		items, next, err := txn.List(prefix, &scanPage, &c)
		if err != nil {
			return nil, "", err
		}

		for _, item := range items {
			var u domain.User
			if err := json.Unmarshal(item.Value, &u); err != nil {
				return nil, "", err
			}
			if !matchesUserQuery(u, q) {
				continue
			}
			hits = append(hits, u)
			if uint64(len(hits)) >= want {
				return hits, next, nil
			}
		}

		if next == local_store.EndCursor || next == "" || len(items) == 0 {
			return hits, local_store.EndCursor, nil
		}
		pageCursor = next
	}
}

func matchesUserQuery(u domain.User, q string) bool {
	if strings.Contains(strings.ToLower(u.Id), q) {
		return true
	}
	if strings.Contains(strings.ToLower(u.Username), q) {
		return true
	}
	if strings.Contains(strings.ToLower(u.Bio), q) {
		return true
	}
	if strings.Contains(strings.ToLower(u.NodeId), q) {
		return true
	}
	return false
}

func (repo *UserRepo) WhoToFollow(limit *uint64, cursor *string) ([]domain.User, string, error) {
	want := uint64(20)
	if limit != nil && *limit > 0 {
		want = *limit
	}

	prefix := local_store.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	// maxScan bounds the work under a large Mastodon flood; scanPage is the
	// per-iteration window handed to the underlying list.
	const maxScan = 5000
	scanPage := uint64(200)

	native := make([]domain.User, 0, want)
	other := make([]domain.User, 0, want)
	pageCursor := ""
	if cursor != nil {
		pageCursor = *cursor
	}
	scanned := 0

	for uint64(len(native)) < want && scanned < maxScan {
		c := pageCursor
		items, next, err := txn.List(prefix, &scanPage, &c)
		if err != nil {
			return nil, "", err
		}

		for _, item := range items {
			scanned++
			var u domain.User
			if err := json.Unmarshal(item.Value, &u); err != nil {
				return nil, "", err
			}
			if u.IsOffline {
				continue
			}
			// Warpnet-native peers (ULID id) come first; everyone else fills the
			// remaining slots. No avatar or tweet gating — a freshly joined
			// account (no picture, no posts yet) is still a valid recommendation.
			if isULID(u.Id) {
				native = append(native, u)
				continue
			}
			if uint64(len(other)) < want {
				other = append(other, u)
			}
		}

		if next == local_store.EndCursor || next == "" || len(items) == 0 {
			break
		}
		pageCursor = next
	}

	// Native peers first, then fill remaining slots with the rest.
	recommended := native
	for _, u := range other {
		if uint64(len(recommended)) >= want {
			break
		}
		recommended = append(recommended, u)
	}

	return recommended, local_store.EndCursor, nil
}

func (repo *UserRepo) GetBatch(userIDs ...string) (users []domain.User, err error) {
	if len(userIDs) == 0 {
		return users, nil
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, err
	}
	defer txn.Rollback()

	users = make([]domain.User, 0, len(userIDs))

	for _, userID := range userIDs {
		fixedKey := local_store.NewPrefixBuilder(UsersRepoName).
			AddSubPrefix(userSubNamespace).
			AddRootID("None").
			AddRange(local_store.FixedRangeKey).
			AddParentId(userID).
			Build()
		sortableKey, err := txn.Get(fixedKey)
		if local_store.IsNotFoundError(err) {
			continue
		}
		if err != nil {
			return nil, err
		}

		data, err := txn.Get(local_store.DatabaseKey(sortableKey))
		if local_store.IsNotFoundError(err) {
			continue
		}
		if err != nil {
			return nil, err
		}

		var u domain.User
		err = json.Unmarshal(data, &u)
		if err != nil {
			log.Errorln("cannot unmarshal batch user data:", string(data))
			return nil, err
		}
		users = append(users, u)
	}

	return users, txn.Commit()
}

func isULID(id string) bool {
	_, err := ulid.ParseStrict(id)
	return err == nil
}
