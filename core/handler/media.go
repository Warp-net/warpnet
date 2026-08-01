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

package handler

import (
	"errors"
	"fmt"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
)

/*

	The system embeds encrypted metadata (node and user information) into the EXIF segment of media files
	during upload.
	A weak password is randomly generated for each file, used for encryption via Argon2id + AES-256-GCM,
	and immediately discarded.
	The password is never stored or logged.
	Decryption is only possible through brute-force attacks, requiring massive computational resources.
	Ordinary users cannot recover the metadata; only powerful entities (e.g., government data centers) can.
	EXIF metadata acts as proof of ownership and responsibility without revealing sensitive data.
	Salt and nonce are public and embedded with the media file.
	Security relies entirely on computational difficulty, not on secrecy of the password.

*/

const (
	nodeMetaKey = "node"
	userMetaKey = "user"
	macMetaKey  = "MAC"

	// ErrInvalidBase64Signature is shared: both media kinds arrive as a
	// "<mime>,<base64>" data URL and fail the same way.
	ErrInvalidBase64Signature warpnet.WarpError = "invalid base64 media data"
)

type MediaNodeInformer interface {
	NodeInfo() warpnet.NodeInfo
}

type MediaUserFetcher interface {
	Get(userId string) (user domain.User, err error)
}

type MediaStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

// MediaMetaStorer is the slice of MediaRepo the alt-text / focal-point
// handlers need.
type MediaMetaStorer interface {
	SetImageMeta(userId, key string, meta database.MediaMeta) error
	GetImageMeta(userId, key string) (database.MediaMeta, error)
}

func StreamUpdateMediaMetaHandler(repo MediaMetaStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UpdateMediaMetaEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("media meta: empty user id")
		}
		if ev.Key == "" {
			return nil, warpnet.WarpError("media meta: empty media key")
		}
		meta := database.MediaMeta{
			Description: ev.Description,
			FocusX:      ev.FocusX,
			FocusY:      ev.FocusY,
		}
		if err := repo.SetImageMeta(ev.UserId, ev.Key, meta); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamGetMediaHandler(repo MediaMetaStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetMediaEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("get media: empty user id")
		}
		if ev.Key == "" {
			return nil, warpnet.WarpError("get media: empty media key")
		}
		meta, err := repo.GetImageMeta(ev.UserId, ev.Key)
		if err != nil {
			return nil, err
		}
		return event.GetMediaResponse{
			Key:         ev.Key,
			Description: meta.Description,
			FocusX:      meta.FocusX,
			FocusY:      meta.FocusY,
		}, nil
	}
}

// buildEncryptedMediaMeta fetches the owner user and produces the
// AES-encrypted {node,user,MAC} blob embedded into uploaded media EXIF.
// The encryption password is random and immediately discarded (see the
// file header), so the metadata stands as proof of ownership without
// leaking its contents. Shared by the image-upload and archive-import
// handlers.
func buildEncryptedMediaMeta(
	info MediaNodeInformer,
	userRepo MediaUserFetcher,
) (encryptedMeta []byte, ownerUser domain.User, err error) {
	nodeInfo := info.NodeInfo()
	ownerUser, err = userRepo.Get(nodeInfo.OwnerId)
	if errors.Is(err, database.ErrUserNotFound) {
		return nil, ownerUser, err
	}
	if err != nil {
		return nil, ownerUser, fmt.Errorf("image meta: fetching user: %w", err)
	}

	metaData := map[string]any{
		nodeMetaKey: nodeInfo, userMetaKey: ownerUser, macMetaKey: warpnet.GetMacAddr(),
	}
	metaBytes, err := json.Marshal(metaData)
	if err != nil {
		return nil, ownerUser, fmt.Errorf("image meta: marshalling meta data: %w", err)
	}

	encryptedMeta, err = security.EncryptAES(metaBytes, nil) // unknown password
	if err != nil {
		return nil, ownerUser, fmt.Errorf("image meta: AES encrypting: %w", err)
	}
	return encryptedMeta, ownerUser, nil
}
