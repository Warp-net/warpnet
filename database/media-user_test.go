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
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type MediaUserTestSuite struct {
	suite.Suite

	db    *local_store.DB
	media *MediaRepo
	users *UserRepo
}

func (s *MediaUserTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	auth := NewAuthRepo(s.db, "test")
	s.Require().NoError(auth.Authenticate("test", "test"))

	s.media = NewMediaRepo(s.db)
	s.users = NewUserRepo(s.db)
}

func (s *MediaUserTestSuite) TearDownSuite() {
	s.db.Close()
}

// ---------------------------------------------------------------------------
// Media storage — content-addressed blobs.
// ---------------------------------------------------------------------------

func (s *MediaUserTestSuite) TestNilMediaRepoNeverPanics() {
	var repo *MediaRepo

	_, err := repo.GetImage("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.SetImage("u", "img")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.GetVideo("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.SetVideo("u", "vid")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.GetImageMeta("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)

	s.ErrorIs(repo.SetImageMeta("u", "k", MediaMeta{}), ErrMediaRepoNotInit)
	s.ErrorIs(repo.SetForeignImageWithTTL("u", "k", "img"), ErrMediaRepoNotInit)
	s.ErrorIs(repo.SetForeignVideoWithTTL("u", "k", "vid"), ErrMediaRepoNotInit)
}

// Media is content-addressed: the same bytes must always produce the same key,
// so re-uploading an image doesn't duplicate megabytes of storage.
func (s *MediaUserTestSuite) TestImageKeyIsContentAddressed() {
	user := uuid.New().String()
	img := Base64Video("data:image/jpeg;base64,AAAA")

	first, err := s.media.SetImage(user, Base64Image(img))
	s.Require().NoError(err)
	second, err := s.media.SetImage(user, Base64Image(img))
	s.Require().NoError(err)
	s.Equal(first, second, "identical bytes must hash to the same key")

	other, err := s.media.SetImage(user, "data:image/jpeg;base64,BBBB")
	s.Require().NoError(err)
	s.NotEqual(first, other, "different bytes must not collide")
}

func (s *MediaUserTestSuite) TestImageRoundTripAndIsolation() {
	alice := uuid.New().String()
	bob := uuid.New().String()

	key, err := s.media.SetImage(alice, "data:image/jpeg;base64,ALICE")
	s.Require().NoError(err)

	got, err := s.media.GetImage(alice, string(key))
	s.Require().NoError(err)
	s.Equal(Base64Image("data:image/jpeg;base64,ALICE"), got)

	// The same content key under another user must not resolve — media is
	// namespaced per owner.
	_, err = s.media.GetImage(bob, string(key))
	s.ErrorIs(err, ErrMediaNotFound)
}

func (s *MediaUserTestSuite) TestVideoRoundTripAndIsolation() {
	alice := uuid.New().String()
	bob := uuid.New().String()

	key, err := s.media.SetVideo(alice, "data:video/mp4;base64,ALICE")
	s.Require().NoError(err)

	got, err := s.media.GetVideo(alice, string(key))
	s.Require().NoError(err)
	s.Equal(Base64Video("data:video/mp4;base64,ALICE"), got)

	_, err = s.media.GetVideo(bob, string(key))
	s.ErrorIs(err, ErrMediaNotFound)
}

func (s *MediaUserTestSuite) TestMediaRejectsEmptyInput() {
	user := uuid.New().String()

	_, err := s.media.SetImage(user, "")
	s.Error(err)
	_, err = s.media.SetImage("", "data")
	s.Error(err)
	_, err = s.media.SetVideo(user, "")
	s.Error(err)
	_, err = s.media.SetVideo("", "data")
	s.Error(err)

	_, err = s.media.GetImage("", "k")
	s.ErrorIs(err, ErrMediaNotFound)
	_, err = s.media.GetImage(user, "")
	s.ErrorIs(err, ErrMediaNotFound)
	_, err = s.media.GetVideo("", "k")
	s.ErrorIs(err, ErrMediaNotFound)
	_, err = s.media.GetVideo(user, "")
	s.ErrorIs(err, ErrMediaNotFound)

	s.Error(s.media.SetForeignImageWithTTL(user, "", "data"))
	s.Error(s.media.SetForeignImageWithTTL(user, "k", ""))
	s.Error(s.media.SetForeignImageWithTTL("", "k", "data"))
	s.Error(s.media.SetForeignVideoWithTTL(user, "", "data"))
	s.Error(s.media.SetForeignVideoWithTTL(user, "k", ""))
	s.Error(s.media.SetForeignVideoWithTTL("", "k", "data"))
}

func (s *MediaUserTestSuite) TestForeignMediaIsReadableUnderTheGivenKey() {
	user := uuid.New().String()

	s.Require().NoError(s.media.SetForeignImageWithTTL(user, "remote-key", "data:image/jpeg;base64,REMOTE"))
	got, err := s.media.GetImage(user, "remote-key")
	s.Require().NoError(err)
	s.Equal(Base64Image("data:image/jpeg;base64,REMOTE"), got)

	s.Require().NoError(s.media.SetForeignVideoWithTTL(user, "remote-clip", "data:video/mp4;base64,REMOTE"))
	vid, err := s.media.GetVideo(user, "remote-clip")
	s.Require().NoError(err)
	s.Equal(Base64Video("data:video/mp4;base64,REMOTE"), vid)
}

// Images and videos live in separate namespaces: a video key must never
// resolve as an image, even when the bytes happen to match.
func (s *MediaUserTestSuite) TestImageAndVideoNamespacesDoNotCollide() {
	user := uuid.New().String()
	payload := "data:application/octet-stream;base64,SAME"

	imgKey, err := s.media.SetImage(user, Base64Image(payload))
	s.Require().NoError(err)
	vidKey, err := s.media.SetVideo(user, Base64Video(payload))
	s.Require().NoError(err)
	s.Equal(string(imgKey), string(vidKey), "the content hash is the same")

	_, err = s.media.GetVideo(user, string(imgKey))
	s.Require().NoError(err, "each namespace holds its own copy")

	other := uuid.New().String()
	_, err = s.media.GetImage(other, string(imgKey))
	s.ErrorIs(err, ErrMediaNotFound)
}

// ---------------------------------------------------------------------------
// Alt-text / focal point metadata.
// ---------------------------------------------------------------------------

// "Never described" and "described as empty" must be indistinguishable to the
// caller, so a missing record is a zero value rather than an error.
func (s *MediaUserTestSuite) TestImageMetaDefaultsToZeroValue() {
	user := uuid.New().String()

	meta, err := s.media.GetImageMeta(user, "never-described")
	s.Require().NoError(err)
	s.Equal(MediaMeta{}, meta)
}

func (s *MediaUserTestSuite) TestImageMetaRoundTripAndOverwrite() {
	user := uuid.New().String()

	s.Require().NoError(s.media.SetImageMeta(user, "key", MediaMeta{
		Description: "a cat on a keyboard",
		FocusX:      -0.5,
		FocusY:      0.25,
	}))

	meta, err := s.media.GetImageMeta(user, "key")
	s.Require().NoError(err)
	s.Equal("a cat on a keyboard", meta.Description)
	s.InDelta(-0.5, meta.FocusX, 0.0001)
	s.InDelta(0.25, meta.FocusY, 0.0001)

	// Editing alt-text must replace, not merge.
	s.Require().NoError(s.media.SetImageMeta(user, "key", MediaMeta{Description: "corrected"}))
	meta, err = s.media.GetImageMeta(user, "key")
	s.Require().NoError(err)
	s.Equal("corrected", meta.Description)
	s.Zero(meta.FocusX)
	s.Zero(meta.FocusY)
}

func (s *MediaUserTestSuite) TestImageMetaSurvivesExoticDescriptions() {
	user := uuid.New().String()

	inputs := []string{
		strings.Repeat("описание ", 500),
		`<img src=x onerror="alert(1)">`,
		"line\nbreak\ttab",
		"🔥🙈 emoji alt text",
		`{"json":"injection"}`,
	}

	for i, desc := range inputs {
		key := uuid.New().String()
		s.Require().NoErrorf(s.media.SetImageMeta(user, key, MediaMeta{Description: desc}), "case %d", i)

		meta, err := s.media.GetImageMeta(user, key)
		s.Require().NoError(err)
		s.Equalf(desc, meta.Description, "alt-text must round trip verbatim (case %d)", i)
	}
}

func (s *MediaUserTestSuite) TestImageMetaRejectsEmptyIdentifiers() {
	s.Error(s.media.SetImageMeta("", "k", MediaMeta{}))
	s.Error(s.media.SetImageMeta("u", "", MediaMeta{}))

	meta, err := s.media.GetImageMeta("", "k")
	s.ErrorIs(err, ErrMediaNotFound)
	s.Equal(MediaMeta{}, meta)

	meta, err = s.media.GetImageMeta("u", "")
	s.ErrorIs(err, ErrMediaNotFound)
	s.Equal(MediaMeta{}, meta)
}

// Metadata lives beside the blob, so describing an image must not touch the
// (potentially megabyte-sized) payload.
func (s *MediaUserTestSuite) TestImageMetaDoesNotDisturbTheBlob() {
	user := uuid.New().String()
	key, err := s.media.SetImage(user, "data:image/jpeg;base64,PAYLOAD")
	s.Require().NoError(err)

	s.Require().NoError(s.media.SetImageMeta(user, string(key), MediaMeta{Description: "alt"}))

	img, err := s.media.GetImage(user, string(key))
	s.Require().NoError(err)
	s.Equal(Base64Image("data:image/jpeg;base64,PAYLOAD"), img)
}

// ---------------------------------------------------------------------------
// Profile updates — partial payloads must never erase a profile.
// ---------------------------------------------------------------------------

func (s *MediaUserTestSuite) createUser(u domain.User) domain.User {
	s.T().Helper()
	if u.Id == "" {
		u.Id = uuid.New().String()
	}
	created, err := s.users.Create(u)
	s.Require().NoError(err)
	return created
}

func (s *MediaUserTestSuite) TestUpdateOfUnknownUserFails() {
	_, err := s.users.Update(uuid.New().String(), domain.User{Username: "ghost"})
	s.Error(err, "updating a profile that was never created must not create it")
}

// A client that only changes the bio sends only the bio — every other field
// must survive untouched.
func (s *MediaUserTestSuite) TestPartialUpdateKeepsUntouchedFields() {
	site := "https://example.org"
	original := s.createUser(domain.User{
		Username:           "alice",
		Bio:                "original bio",
		Birthdate:          "1990-01-01",
		AvatarKey:          "avatar-1",
		BackgroundImageKey: "bg-1",
		Website:            &site,
		NodeId:             "node-1",
		Network:            "warpnet",
	})

	updated, err := s.users.Update(original.Id, domain.User{Bio: "new bio"})
	s.Require().NoError(err)

	s.Equal("new bio", updated.Bio)
	s.Equal("alice", updated.Username, "an unset username must not blank the profile")
	s.Equal("1990-01-01", updated.Birthdate)
	s.Equal("avatar-1", updated.AvatarKey)
	s.Equal("bg-1", updated.BackgroundImageKey)
	s.Require().NotNil(updated.Website)
	s.Equal(site, *updated.Website)
	s.Equal("node-1", updated.NodeId)
	s.Require().NotNil(updated.UpdatedAt)
}

func (s *MediaUserTestSuite) TestUpdatePersistsAndIsReadableBack() {
	original := s.createUser(domain.User{Username: "bob", Bio: "before"})

	_, err := s.users.Update(original.Id, domain.User{Bio: "after"})
	s.Require().NoError(err)

	got, err := s.users.Get(original.Id)
	s.Require().NoError(err)
	s.Equal("after", got.Bio)
	s.Equal("bob", got.Username)
}

// A peer that doesn't report a role must not wipe a role we already learned.
func (s *MediaUserTestSuite) TestUpdateNeverClearsRole() {
	original := s.createUser(domain.User{Username: "carol", Role: "member"})

	updated, err := s.users.Update(original.Id, domain.User{Bio: "hi", Role: ""})
	s.Require().NoError(err)
	s.Equal("member", updated.Role, "an empty role from a peer must not clear the known one")

	updated, err = s.users.Update(original.Id, domain.User{Role: "moderator"})
	s.Require().NoError(err)
	s.Equal("moderator", updated.Role, "an explicit role must still win")
}

// Moderation strikes accumulate across reports — resetting them would let an
// abuser wipe their record by triggering one more update.
func (s *MediaUserTestSuite) TestModerationStrikesAccumulate() {
	original := s.createUser(domain.User{Username: "dave"})

	reason := "spam"
	updated, err := s.users.Update(original.Id, domain.User{
		Moderation: &domain.UserModeration{IsModerated: true, Strikes: 1, Reason: &reason},
	})
	s.Require().NoError(err)
	s.Require().NotNil(updated.Moderation)
	s.Equal(uint8(1), updated.Moderation.Strikes)

	updated, err = s.users.Update(original.Id, domain.User{
		Moderation: &domain.UserModeration{IsModerated: true, Strikes: 2},
	})
	s.Require().NoError(err)
	s.Equal(uint8(3), updated.Moderation.Strikes, "strikes must add up, not overwrite")

	// An update that carries no verdict must leave the record alone.
	updated, err = s.users.Update(original.Id, domain.User{Bio: "unrelated change"})
	s.Require().NoError(err)
	s.Require().NotNil(updated.Moderation)
	s.Equal(uint8(3), updated.Moderation.Strikes)
	s.True(updated.Moderation.IsModerated)
}

func (s *MediaUserTestSuite) TestMetadataMergesInsteadOfReplacing() {
	original := s.createUser(domain.User{
		Username: "erin",
		Metadata: map[string]string{"pronouns": "they/them"},
	})

	updated, err := s.users.Update(original.Id, domain.User{
		Metadata: map[string]string{"location": "Berlin"},
	})
	s.Require().NoError(err)
	s.Equal("they/them", updated.Metadata["pronouns"], "existing metadata must survive")
	s.Equal("Berlin", updated.Metadata["location"])

	// A repeated key is overwritten, not duplicated.
	updated, err = s.users.Update(original.Id, domain.User{
		Metadata: map[string]string{"location": "Lisbon"},
	})
	s.Require().NoError(err)
	s.Equal("Lisbon", updated.Metadata["location"])
	s.Len(updated.Metadata, 2)
}

// Liveness fields are authoritative on every update — unlike profile text they
// must track the latest observation, including back to "online".
func (s *MediaUserTestSuite) TestLivenessFieldsAlwaysFollowTheLatestUpdate() {
	original := s.createUser(domain.User{Username: "frank"})

	updated, err := s.users.Update(original.Id, domain.User{IsOffline: true, RoundTripTime: 500})
	s.Require().NoError(err)
	s.True(updated.IsOffline)
	s.Equal(int64(500), updated.RoundTripTime)

	updated, err = s.users.Update(original.Id, domain.User{IsOffline: false, RoundTripTime: 20})
	s.Require().NoError(err)
	s.False(updated.IsOffline, "a peer coming back online must be reflected")
	s.Equal(int64(20), updated.RoundTripTime)
}

// Rebinding a user to a new node must make them findable by that node id —
// otherwise a migrated account becomes unreachable.
func (s *MediaUserTestSuite) TestUpdateRebindsNodeIndex() {
	original := s.createUser(domain.User{Username: "grace", NodeId: "node-old"})

	_, err := s.users.Update(original.Id, domain.User{NodeId: "node-new"})
	s.Require().NoError(err)

	got, err := s.users.GetByNodeID("node-new")
	s.Require().NoError(err)
	s.Equal(original.Id, got.Id)
}

func (s *MediaUserTestSuite) TestUpdateHandlesExoticProfileText() {
	original := s.createUser(domain.User{Username: "heidi"})

	longText := strings.Repeat("я", 3000)
	updated, err := s.users.Update(original.Id, domain.User{
		Bio:      longText,
		Username: `<script>alert(1)</script>`,
	})
	s.Require().NoError(err)
	s.Equal(longText, updated.Bio)
	s.Equal(`<script>alert(1)</script>`, updated.Username, "escaping is the client's job, storage must be verbatim")
}

func TestMediaUserTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(MediaUserTestSuite))
}
