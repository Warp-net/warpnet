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
	"time"

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type DevicesRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *DevicesRepo
}

func (s *DevicesRepoTestSuite) SetupTest() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	s.Require().NoError(s.db.Run("test", "test"))
	s.repo = NewDevicesRepo(s.db)
}

func (s *DevicesRepoTestSuite) TearDownTest() {
	s.db.Close()
}

func (s *DevicesRepoTestSuite) TestSetAndGetDevice_RoundTrip() {
	const ownerNodeId = "12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"
	deviceNodeId := warpnet.FromStringToPeerID(
		"12D3KooWQ3umNTQweTREML1gqyag4T2Ps82wLnHV7fUNQA8CnMa9",
	)

	d := domain.Device{
		NodeId:   deviceNodeId,
		Token:    "session-token",
		Platform: "android",
	}
	s.Require().NoError(s.repo.SetDevice(ownerNodeId, d))

	devices, err := s.repo.GetDevices(ownerNodeId)
	s.Require().NoError(err)
	s.Require().Len(devices, 1)
	got := devices[0]

	assert.Equal(s.T(), deviceNodeId, got.NodeId)
	assert.Equal(s.T(), d.Token, got.Token)
	assert.Equal(s.T(), d.Platform, got.Platform)
	assert.NotEmpty(s.T(), got.ID, "SetDevice should populate ID with a ULID")
	assert.False(s.T(), got.CreatedAt.IsZero(), "SetDevice should populate CreatedAt")
}

func (s *DevicesRepoTestSuite) TestGetDevices_EmptyOwner_ReturnsNoError() {
	devices, err := s.repo.GetDevices("nonexistent-owner")
	s.Require().NoError(err)
	assert.Empty(s.T(), devices)
}

func (s *DevicesRepoTestSuite) TestGetDevices_MultipleDevices() {
	const ownerNodeId = "12D3KooWOwnerXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
	peers := []string{
		"12D3KooWQ3umNTQweTREML1gqyag4T2Ps82wLnHV7fUNQA8CnMa9",
		"12D3KooWNXSGyfTuYc3JznW48jay73BtQgHszWfPpyF581EWcpGJ",
		"12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU",
	}
	for _, p := range peers {
		s.Require().NoError(s.repo.SetDevice(ownerNodeId, domain.Device{
			NodeId: warpnet.FromStringToPeerID(p),
			Token:  "tok-" + p,
		}))
	}

	devices, err := s.repo.GetDevices(ownerNodeId)
	s.Require().NoError(err)
	assert.Len(s.T(), devices, len(peers))
}

func (s *DevicesRepoTestSuite) TestGetDevices_NilRepo() {
	repo := &DevicesRepo{}
	_, err := repo.GetDevices("any")
	assert.ErrorIs(s.T(), err, ErrNilDevicesRepo)
}

func (s *DevicesRepoTestSuite) TestSetDevice_NilRepo() {
	repo := &DevicesRepo{}
	err := repo.SetDevice("any", domain.Device{})
	assert.ErrorIs(s.T(), err, ErrNilDevicesRepo)
}

// TestDevice_JSONRoundTrip locks down the on-disk encoding: NodeId is a
// libp2p peer.ID alias and must round-trip through the same encoder we
// store with so reads decode cleanly.
func TestDevice_JSONRoundTrip(t *testing.T) {
	d := domain.Device{
		ID:         "01KPC4AP7FDEHD8CPTYZAKKDBK",
		CreatedAt:  time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC),
		NodeId:     warpnet.FromStringToPeerID("12D3KooWQ3umNTQweTREML1gqyag4T2Ps82wLnHV7fUNQA8CnMa9"),
		Token:      "session-token",
		Platform:   "android",
		LastActive: time.Date(2026, 4, 27, 10, 5, 0, 0, time.UTC),
	}

	raw, err := json.Marshal(d)
	assert.NoError(t, err)

	var got domain.Device
	err = json.Unmarshal(raw, &got)
	assert.NoError(t, err)

	assert.Equal(t, d.ID, got.ID)
	assert.Equal(t, d.NodeId, got.NodeId)
	assert.Equal(t, d.Token, got.Token)
	assert.Equal(t, d.Platform, got.Platform)
	assert.True(t, d.CreatedAt.Equal(got.CreatedAt))
	assert.True(t, d.LastActive.Equal(got.LastActive))
}

func TestDevicesRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(DevicesRepoTestSuite))
}
