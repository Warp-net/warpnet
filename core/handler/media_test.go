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
package handler

import (
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// Shared test doubles for the media handlers.

type (
	n struct{}
	m struct{}
	u struct{}
	s struct{}
)

func (u u) GetOtherNetworkUser(network, userId string) (user domain.User, err error) {
	return domain.User{}, err
}

func (m m) SetForeignImageWithTTL(userId, key string, img database.Base64Image) error {
	return nil
}

func (m m) GetImage(userId, key string) (database.Base64Image, error) {
	return "", nil
}

func (m m) SetImage(userId string, img database.Base64Image) (key database.ImageKey, err error) {
	return "", nil
}

func (n n) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{}
}

func (s s) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (s s) Write(p []byte) (n int, err error) {
	return 0, nil
}

func (s s) Close() error {
	return nil
}

func (s s) CloseWrite() error {
	return nil
}

func (s s) CloseRead() error {
	return nil
}

func (s s) Reset() error {
	return nil
}

func (s s) ResetWithError(errCode network.StreamErrorCode) error {
	return nil
}

func (s s) SetDeadline(time time.Time) error {
	return nil
}

func (s s) SetReadDeadline(time time.Time) error {
	return nil
}

func (s s) SetWriteDeadline(time time.Time) error {
	return nil
}

func (s s) ID() string {
	return ""
}

func (s s) Protocol() protocol.ID {
	return ""
}

func (s s) SetProtocol(id protocol.ID) error {
	return nil
}

func (s s) Stat() network.Stats {
	return network.Stats{}
}

func (s s) Conn() network.Conn {
	return nil
}

func (s s) Scope() network.StreamScope {
	return nil
}

func (u u) Get(userId string) (user domain.User, err error) {
	return domain.User{}, nil
}

type cachedMediaRepo struct {
	cached database.Base64Image
}

func (c cachedMediaRepo) GetImage(userId, key string) (database.Base64Image, error) {
	return c.cached, nil
}

func (c cachedMediaRepo) SetImage(userId string, img database.Base64Image) (database.ImageKey, error) {
	return "", nil
}

func (c cachedMediaRepo) SetForeignImageWithTTL(userId, key string, img database.Base64Image) error {
	return nil
}

type recordingStreamer struct {
	streamed *bool
}

func (r recordingStreamer) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	*r.streamed = true
	return json.Marshal(event.GetImageResponse{File: "from-gateway"})
}

func (r recordingStreamer) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{OwnerId: "owner-id"}
}

type foreignUserRepo struct{}

func (foreignUserRepo) Get(userId string) (domain.User, error) {
	return domain.User{Id: userId, NodeId: "gateway-node", Network: "mastodon"}, nil
}
