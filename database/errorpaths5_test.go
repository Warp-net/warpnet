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

//nolint:all
package database

import (
	"testing"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

// failingNotifier stands in for the notifications repo when the "new user
// discovered" write fails; the user write must still succeed.
type failingNotifier struct{}

func (failingNotifier) Add(domain.Notification) error { return errFault }

func TestUserRepoErrorPaths(t *testing.T) {
	userId := ulid.Make().String()
	nodeId := "12D3KooWTestNodeIdentifier"
	user := func() domain.User {
		return domain.User{Id: userId, Username: "alice", NodeId: nodeId}
	}
	seedUser := func(t *testing.T, s *faultStore) {
		_, err := NewUserRepo(s).Create(user())
		require.NoError(t, err)
	}

	userKey := local_store.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		AddRange(local_store.FixedRangeKey).
		AddParentId(userId).
		Build()

	runFaultCases(t, []faultCase{
		{
			name: "CreateWithTTL",
			run: func(s *faultStore) error {
				_, err := NewUserRepo(s).Create(user())
				return err
			},
			ops: []faultOp{op("SetWithTTL"), opN("SetWithTTL", 2), opN("SetWithTTL", 3), op("Commit")},
		},
		{
			name: "Update",
			seed: seedUser,
			run: func(s *faultStore) error {
				_, err := NewUserRepo(s).Update(userId, domain.User{Bio: "updated"})
				return err
			},
			ops: []faultOp{op("Get"), opN("Get", 2), op("Set"), op("Commit")},
		},
		{
			name: "Delete",
			seed: seedUser,
			run:  func(s *faultStore) error { return NewUserRepo(s).Delete(userId) },
			ops: []faultOp{
				op("Get"), opN("Get", 2), op("Delete"), opN("Delete", 2), opN("Delete", 3), op("Commit"),
			},
		},
		{
			name: "List",
			seed: seedUser,
			run: func(s *faultStore) error {
				_, _, err := NewUserRepo(s).List(nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
		{
			name: "Search",
			seed: seedUser,
			run: func(s *faultStore) error {
				_, _, err := NewUserRepo(s).Search("alice", nil, nil)
				return err
			},
			ops: []faultOp{op("List")},
		},
		{
			name: "WhoToFollow",
			seed: seedUser,
			run: func(s *faultStore) error {
				_, _, err := NewUserRepo(s).WhoToFollow(nil, nil)
				return err
			},
			ops: []faultOp{op("List")},
		},
		{
			name: "GetBatch",
			seed: seedUser,
			run: func(s *faultStore) error {
				_, err := NewUserRepo(s).GetBatch(userId)
				return err
			},
			ops: []faultOp{op("Get"), opN("Get", 2), op("Commit")},
		},
	})

	t.Run("CreateStoreLookupFails", func(t *testing.T) {
		s := newFaultStore(t).arm("db.Get", 1)
		_, err := NewUserRepo(s).Create(user())
		// a failed existence probe is indistinguishable from "found"
		require.ErrorIs(t, err, ErrUserAlreadyExists)
	})

	t.Run("CreateTwice", func(t *testing.T) {
		s := newFaultStore(t)
		seedUser(t, s)
		_, err := NewUserRepo(s).Create(user())
		require.ErrorIs(t, err, ErrUserAlreadyExists)
	})

	t.Run("CreateWithoutNodeId", func(t *testing.T) {
		s := newFaultStore(t)
		stored, err := NewUserRepo(s).Create(domain.User{Id: ulid.Make().String(), Username: "bob"})
		require.NoError(t, err)
		require.Equal(t, DefaultWarpnetUserNetwork, stored.Network)
		require.Equal(t, int64(defaultAverageRTT), stored.RoundTripTime)
	})

	t.Run("Validation", func(t *testing.T) {
		repo := NewUserRepo(newFaultStore(t))

		_, err := repo.Create(domain.User{})
		require.Error(t, err)

		_, err = repo.Get("")
		require.ErrorIs(t, err, ErrUserNotFound)
		_, err = repo.Get("absent")
		require.ErrorIs(t, err, ErrUserNotFound)

		_, err = repo.GetByNodeID("")
		require.ErrorIs(t, err, ErrUserNotFound)
		_, err = repo.GetByNodeID("absent")
		require.ErrorIs(t, err, ErrUserNotFound)

		_, _, err = repo.Search("", nil, nil)
		require.Error(t, err)
		_, _, err = repo.Search("   ", nil, nil)
		require.Error(t, err)

		users, err := repo.GetBatch()
		require.NoError(t, err)
		require.Empty(t, users)
	})

	t.Run("GetStoreFails", func(t *testing.T) {
		s := newFaultStore(t)
		seedUser(t, s)
		repo := NewUserRepo(s)

		s.arm("db.Get", 1)
		_, err := repo.Get(userId)
		require.ErrorIs(t, err, errFault)

		s.arm("db.Get", 2)
		_, err = repo.Get(userId)
		require.ErrorIs(t, err, errFault)

		s.arm("db.Get", 1)
		_, err = repo.GetByNodeID(nodeId)
		require.ErrorIs(t, err, errFault)

		s.arm("db.Get", 2)
		_, err = repo.GetByNodeID(nodeId)
		require.ErrorIs(t, err, errFault)
	})

	t.Run("CorruptPayload", func(t *testing.T) {
		s := newFaultStore(t)
		seedUser(t, s)

		sortable, err := s.db.Get(userKey)
		require.NoError(t, err)
		require.NoError(t, s.db.Set(local_store.DatabaseKey(sortable), []byte("{not-json")))

		repo := NewUserRepo(s)
		_, err = repo.Get(userId)
		require.Error(t, err)
		_, err = repo.GetByNodeID(nodeId)
		require.Error(t, err)
		_, _, err = repo.List(nil, nil)
		require.Error(t, err)
		_, _, err = repo.Search("alice", nil, nil)
		require.Error(t, err)
		_, _, err = repo.WhoToFollow(nil, nil)
		require.Error(t, err)
		_, err = repo.GetBatch(userId)
		require.Error(t, err)
		_, err = repo.Update(userId, domain.User{Bio: "x"})
		require.Error(t, err)
		require.Error(t, repo.Delete(userId))
	})

	t.Run("DeleteAbsentIsNoop", func(t *testing.T) {
		require.NoError(t, NewUserRepo(newFaultStore(t)).Delete("absent"))
	})

	t.Run("UpdateAbsent", func(t *testing.T) {
		repo := NewUserRepo(newFaultStore(t))
		_, err := repo.Update("absent", domain.User{Bio: "x"})
		require.Error(t, err)
	})

	t.Run("UpdateMergesFields", func(t *testing.T) {
		s := newFaultStore(t)
		seedUser(t, s)
		repo := NewUserRepo(s)

		site := "https://example.org"
		reason := "spam"
		updated, err := repo.Update(userId, domain.User{
			Bio:                "new bio",
			Birthdate:          "2000-01-01",
			AvatarKey:          "avatar-key",
			Username:           "alice2",
			BackgroundImageKey: "bg-key",
			Website:            &site,
			NodeId:             "other-node",
			Network:            "warpnet",
			Metadata:           map[string]string{"a": "1"},
			Moderation:         &domain.UserModeration{Strikes: 1, Reason: &reason},
		})
		require.NoError(t, err)
		require.Equal(t, "new bio", updated.Bio)
		require.Equal(t, "alice2", updated.Username)
		require.Equal(t, &site, updated.Website)
		require.Equal(t, map[string]string{"a": "1"}, updated.Metadata)
		require.NotNil(t, updated.UpdatedAt)

		// a second moderation update accumulates strikes on the existing record
		updated, err = repo.Update(userId, domain.User{
			Moderation: &domain.UserModeration{Strikes: 2},
			Metadata:   map[string]string{"b": "2"},
		})
		require.NoError(t, err)
		require.EqualValues(t, 3, updated.Moderation.Strikes)
		require.Equal(t, map[string]string{"a": "1", "b": "2"}, updated.Metadata)
	})

	t.Run("SearchMatchesEveryIndexedField", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewUserRepo(s)

		id := ulid.Make().String()
		_, err := repo.Create(domain.User{Id: id, Username: "Charlie", Bio: "loves GARDENING", NodeId: "12D3NodeXyz"})
		require.NoError(t, err)

		for _, q := range []string{"charlie", "gardening", "12d3nodexyz", id[:6]} {
			hits, _, err := repo.Search(q, nil, nil)
			require.NoError(t, err, q)
			require.NotEmpty(t, hits, q)
		}

		hits, _, err := repo.Search("nobody-matches-this", nil, nil)
		require.NoError(t, err)
		require.Empty(t, hits)
	})

	t.Run("SearchStopsAtLimit", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewUserRepo(s)
		for range 5 {
			_, err := repo.Create(domain.User{Id: ulid.Make().String(), Username: "shared-name"})
			require.NoError(t, err)
		}

		limit := uint64(2)
		hits, _, err := repo.Search("shared-name", &limit, nil)
		require.NoError(t, err)
		require.Len(t, hits, 2)
	})

	t.Run("WhoToFollowPrefersNativeAndSkipsOffline", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewUserRepo(s)

		nativeId := ulid.Make().String()
		_, err := repo.Create(domain.User{Id: nativeId, Username: "native"})
		require.NoError(t, err)
		_, err = repo.Create(domain.User{Id: "not-a-ulid", Username: "foreign"})
		require.NoError(t, err)
		_, err = repo.Create(domain.User{Id: ulid.Make().String(), Username: "gone", IsOffline: true})
		require.NoError(t, err)

		limit := uint64(10)
		recommended, cur, err := repo.WhoToFollow(&limit, nil)
		require.NoError(t, err)
		require.Equal(t, local_store.EndCursor, cur)

		ids := make([]string, 0, len(recommended))
		for _, u := range recommended {
			ids = append(ids, u.Id)
		}
		require.Contains(t, ids, nativeId)
		require.Contains(t, ids, "not-a-ulid")
		require.Equal(t, nativeId, recommended[0].Id)
		for _, u := range recommended {
			require.False(t, u.IsOffline)
		}
	})

	t.Run("GetBatchSkipsMissing", func(t *testing.T) {
		s := newFaultStore(t)
		seedUser(t, s)

		users, err := NewUserRepo(s).GetBatch(userId, "absent")
		require.NoError(t, err)
		require.Len(t, users, 1)
		require.Equal(t, userId, users[0].Id)
	})

	t.Run("NotifiesOnNewUser", func(t *testing.T) {
		s := newFaultStore(t)
		notifications := NewNotificationsRepo(s)
		ownerId := ulid.Make().String()
		repo := NewUserRepoNotifying(s, notifications, ownerId)

		// the owner's own record is not announced
		_, err := repo.Create(domain.User{Id: ownerId, Username: "owner"})
		require.NoError(t, err)
		count, err := notifications.UnreadCount(ownerId)
		require.NoError(t, err)
		require.Zero(t, count)

		_, err = repo.Create(domain.User{Id: ulid.Make().String(), Username: "newcomer"})
		require.NoError(t, err)
		count, err = notifications.UnreadCount(ownerId)
		require.NoError(t, err)
		require.Equal(t, uint64(1), count)

		// a user with no username falls back to the id in the notification text
		_, err = repo.Create(domain.User{Id: ulid.Make().String()})
		require.NoError(t, err)
		count, err = notifications.UnreadCount(ownerId)
		require.NoError(t, err)
		require.Equal(t, uint64(2), count)
	})

	t.Run("NotifierFailureDoesNotFailCreate", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewUserRepoNotifying(s, failingNotifier{}, ulid.Make().String())

		_, err := repo.Create(domain.User{Id: ulid.Make().String(), Username: "newcomer"})
		require.NoError(t, err)
	})
}
