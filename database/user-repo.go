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
	"errors"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	log "github.com/sirupsen/logrus"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/database/storage"
	"github.com/Warp-net/warpnet/json"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
)

const (
	UsersRepoName    = "/USERS"
	userSubNamespace = "USER"
	nodeSubNamespace = "NODE"

	defaultAverageLatency int64 = 125000

	DefaultWarpnetUserNetwork = "warpnet"
)

type UserStorer interface {
	NewTxn() (storage.WarpTransactioner, error)
	Set(key storage.DatabaseKey, value []byte) error
	Get(key storage.DatabaseKey) ([]byte, error)
	Delete(key storage.DatabaseKey) error
}

type UserRepo struct {
	db UserStorer
}

func NewUserRepo(db UserStorer) *UserRepo {
	return &UserRepo{db: db}
}

// Create adds a new user to the database
func (repo *UserRepo) Create(user domain.User) (domain.User, error) {
	return repo.CreateWithTTL(user, math.MaxInt64)
}

func (repo *UserRepo) CreateWithTTL(user domain.User, ttl time.Duration) (domain.User, error) {
	if user.Id == "" {
		return user, errors.New("user id is empty")
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}
	data, err := json.JSON.Marshal(user)
	if err != nil {
		return user, err
	}

	if user.Latency == 0 {
		user.Latency = defaultAverageLatency
	}
	if user.Network == "" {
		user.Network = DefaultWarpnetUserNetwork
	}

	rttRange := storage.RangePrefix(strconv.FormatInt(user.Latency, 10))

	fixedKey := storage.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		AddRange(storage.FixedRangeKey).
		AddParentId(user.Id).
		Build()

	_, err = repo.db.Get(fixedKey)
	if !errors.Is(err, storage.ErrKeyNotFound) {
		return user, ErrUserAlreadyExists
	}

	sortableKey := storage.NewPrefixBuilder(UsersRepoName).
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
		nodeUserKey := storage.NewPrefixBuilder(UsersRepoName).
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
	return user, txn.Commit()
}

func (repo *UserRepo) Update(userId string, newUser domain.User) (domain.User, error) {
	var existingUser domain.User

	fixedKey := storage.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		AddRange(storage.FixedRangeKey).
		AddParentId(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return existingUser, err
	}
	defer txn.Rollback()

	sortableKeyBytes, err := txn.Get(fixedKey)
	if err != nil {
		return existingUser, err
	}

	data, err := txn.Get(storage.DatabaseKey(sortableKeyBytes))
	if errors.Is(err, storage.ErrKeyNotFound) {
		return existingUser, ErrUserNotFound
	}
	if err != nil {
		return existingUser, err
	}

	err = json.JSON.Unmarshal(data, &existingUser)
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
	existingUser.Latency = newUser.Latency

	bt, err := json.JSON.Marshal(existingUser)
	if err != nil {
		return existingUser, err
	}
	if err = txn.Set(fixedKey, sortableKeyBytes); err != nil {
		return existingUser, err
	}
	if err = txn.Set(storage.DatabaseKey(sortableKeyBytes), bt); err != nil {
		return existingUser, err
	}

	if newUser.NodeId != "" {
		nodeUserKey := storage.NewPrefixBuilder(UsersRepoName).
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
	fixedKey := storage.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		AddRange(storage.FixedRangeKey).
		AddParentId(userId).
		Build()
	sortableKeyBytes, err := repo.db.Get(fixedKey)
	if errors.Is(err, storage.ErrKeyNotFound) {
		return user, ErrUserNotFound
	}
	if err != nil {
		return user, err
	}

	data, err := repo.db.Get(storage.DatabaseKey(sortableKeyBytes))
	if errors.Is(err, storage.ErrKeyNotFound) {
		return user, ErrUserNotFound
	}
	if err != nil {
		return user, err
	}

	err = json.JSON.Unmarshal(data, &user)
	if err != nil {
		return user, err
	}

	return user, nil
}

func (repo *UserRepo) GetByNodeID(nodeID string) (user domain.User, err error) {
	if nodeID == "" {
		return user, ErrUserNotFound
	}
	nodeUserKey := storage.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(nodeSubNamespace).
		AddRootID(nodeID).
		Build()

	sortableKeyBytes, err := repo.db.Get(nodeUserKey)
	if errors.Is(err, storage.ErrKeyNotFound) {
		return user, ErrUserNotFound
	}
	if err != nil {
		return user, err
	}

	data, err := repo.db.Get(storage.DatabaseKey(sortableKeyBytes))
	if errors.Is(err, storage.ErrKeyNotFound) {
		return user, ErrUserNotFound
	}
	if err != nil {
		return user, err
	}

	err = json.JSON.Unmarshal(data, &user)
	if err != nil {
		return user, err
	}

	return user, nil
}

// Delete removes a user by their ID
func (repo *UserRepo) Delete(userId string) error {
	fixedKey := storage.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		AddRange(storage.FixedRangeKey).
		AddParentId(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	sortableKeyBytes, err := txn.Get(fixedKey)
	if errors.Is(err, storage.ErrKeyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	data, err := txn.Get(storage.DatabaseKey(sortableKeyBytes))
	if err != nil {
		return err
	}

	var u domain.User
	err = json.JSON.Unmarshal(data, &u)
	if err != nil {
		return err
	}

	if err = txn.Delete(fixedKey); err != nil {
		return err
	}
	if err = txn.Delete(storage.DatabaseKey(sortableKeyBytes)); err != nil {
		return err
	}
	if u.NodeId != "" {
		nodeUserKey := storage.NewPrefixBuilder(UsersRepoName).
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
	prefix := storage.NewPrefixBuilder(UsersRepoName).
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
		err = json.JSON.Unmarshal(item.Value, &u)
		if err != nil {
			return nil, "", err
		}
		users = append(users, u)
	}

	return users, cur, nil
}

// TODO refactor
func (repo *UserRepo) WhoToFollow(profileId string, limit *uint64, cursor *string) ([]domain.User, string, error) {
	profile, err := repo.Get(profileId)
	if err != nil {
		return nil, "", err
	}
	if cursor == nil {
		cursor = new(string)
	}

	users, cur, err := repo.List(limit, cursor)
	if err != nil {
		return nil, "", err
	}

	recommended := make([]domain.User, 0, len(users))
	for _, u := range users {
		if u.IsOffline {
			continue
		}
		if u.AvatarKey == "" || strings.Contains(strings.ToLower(u.AvatarKey), "missing") {
			continue
		}
		if u.TweetsCount == 0 {
			continue
		}
		if profile.Network != u.Network { // if profile from Warpnet - don't show other network recommendations
			continue
		}
		recommended = append(recommended, u)
	}

	left := len(users) - len(recommended)
	if left <= 0 || cur == "end" || cur == *cursor {
		return recommended, cur, nil
	}

	left64 := uint64(left)
	leftUsers, cur, err := repo.WhoToFollow(profileId, &left64, &cur)
	if err != nil {
		return nil, "", err
	}
	recommended = append(recommended, leftUsers...)
	return recommended, cur, nil
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
		fixedKey := storage.NewPrefixBuilder(UsersRepoName).
			AddSubPrefix(userSubNamespace).
			AddRootID("None").
			AddRange(storage.FixedRangeKey).
			AddParentId(userID).
			Build()
		sortableKey, err := txn.Get(fixedKey)
		if errors.Is(err, storage.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}

		data, err := txn.Get(storage.DatabaseKey(sortableKey))
		if errors.Is(err, storage.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}

		var u domain.User
		err = json.JSON.Unmarshal(data, &u)
		if err != nil {
			log.Errorln("cannot unmarshal batch user data:", string(data))
			return nil, err
		}
		users = append(users, u)
	}

	return users, txn.Commit()
}

// ValidateUser if already taken
func (repo *UserRepo) ValidateUserID(ev event.ValidationEvent) error {
	if repo == nil {
		return nil
	}

	if ev.User == nil {
		return nil
	}

	innerUser, err := repo.Get(ev.User.Id)

	isUserAlreadyExists := !errors.Is(err, ErrUserNotFound) || err == nil
	isSameNode := ev.User.NodeId == innerUser.NodeId
	isOuterNewer := ev.User.CreatedAt.After(innerUser.CreatedAt)

	if isUserAlreadyExists && isOuterNewer && !isSameNode {
		return errors.New("validator rejected new user")
	}

	return nil
}
