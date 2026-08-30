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

// Package authorship resolves the actor behind an incoming event and checks the
// event came from that actor's own node. It sits above warpnet.VerifyAuthorship,
// which only compares an already-known node id, and below the handlers, which
// each read the actor id out of their own payload.
package authorship

import (
	"errors"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type UserStorer interface {
	Get(userId string) (user domain.User, err error)
	Create(user domain.User) (domain.User, error)
}

type NodeStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
}

func FetchActor(
	userRepo UserStorer, streamer NodeStreamer, s warpnet.WarpStream, actorId string,
) (domain.User, error) {
	actor, err := userRepo.Get(actorId)
	if err == nil {
		return actor, nil
	}
	if !errors.Is(err, database.ErrUserNotFound) {
		return domain.User{}, err
	}
	if s == nil || s.Conn() == nil {
		return domain.User{}, err
	}

	senderNodeId := s.Conn().RemotePeer().String()
	resp, err := streamer.GenericStream(senderNodeId, event.PUBLIC_GET_USER, event.GetUserEvent{
		UserId: domain.ID(actorId),
		NodeId: senderNodeId,
	})
	if err != nil {
		return domain.User{}, err
	}
	if err := json.Unmarshal(resp, &actor); err != nil {
		return domain.User{}, err
	}
	if actor.Id == "" {
		return domain.User{}, database.ErrUserNotFound
	}
	if _, err := userRepo.Create(actor); err != nil && !errors.Is(err, database.ErrUserAlreadyExists) {
		return domain.User{}, err
	}
	return actor, nil
}

func VerifyActor(
	userRepo UserStorer, streamer NodeStreamer, s warpnet.WarpStream, actorId string,
) (domain.User, error) {
	actor, err := FetchActor(userRepo, streamer, s, actorId)
	if err != nil {
		log.Infof("verify actor %s: %v", actorId, err)
	}
	return actor, warpnet.VerifyAuthorship(s, actor.NodeId)
}
