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
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

/*

	The system embeds encrypted metadata (node and user information) into the EXIF segment of media files
	during upload.
	A weak password is randomly generated for each upload, used for encryption via Argon2id + AES-256-GCM,
	and immediately discarded. Files uploaded together share one blob: they carry identical metadata,
	so a per-file password would not raise the cost of recovering it.
	The password is never stored or logged.
	Decryption is only possible through brute-force attacks, requiring massive computational resources.
	Ordinary users cannot recover the metadata; only powerful entities (e.g., government data centers) can.
	EXIF metadata acts as proof of ownership and responsibility without revealing sensitive data.
	Salt and nonce are public and embedded with the media file.
	Security relies entirely on computational difficulty, not on secrecy of the password.

*/

const (
	ErrEmptyImageKey    warpnet.WarpError = "empty image key"
	ErrNoImagesProvided warpnet.WarpError = "at least one image must be provided"
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

type MediaStorer interface {
	GetImage(userId, key string) (domain.Base64Image, error)
	SetImage(userId string, img domain.Base64Image) (_ domain.ImageKey, err error)
	SetForeignImageWithTTL(userId, key string, img domain.Base64Image) error
}

func StreamUploadImageHandler(
	info MediaNodeInformer,
	privKey ed25519.PrivateKey,
	mediaRepo MediaStorer,
	userRepo MediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UploadImageEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, err
		}

		images := [4]string{ev.Image1, ev.Image2, ev.Image3, ev.Image4}

		hasImages := false
		for _, img := range images {
			if img != "" {
				hasImages = true
				break
			}
		}
		if !hasImages {
			return nil, ErrNoImagesProvided
		}

		nodeInfo := info.NodeInfo()

		owner, err := userRepo.Get(nodeInfo.OwnerId)
		if err != nil {
			return nil, fmt.Errorf("upload: image: fetching owner: %w", err)
		}

		metadata, err := media_meta.BuildMetadata(nodeInfo, privKey, owner)
		if err != nil {
			return nil, err
		}

		var keys [4]string
		for i, file := range images {
			if file == "" {
				continue
			}

			img, err := media_meta.SignUploadedImage(file, metadata)
			if err != nil {
				return nil, fmt.Errorf("upload: image%d: %w", i+1, err)
			}

			key, err := mediaRepo.SetImage(metadata.OwnerId, img)
			if err != nil {
				return nil, fmt.Errorf("upload: image%d: storing media: %w", i+1, err)
			}
			keys[i] = string(key)
		}

		return event.UploadImageResponse{
			Key1: keys[0],
			Key2: keys[1],
			Key3: keys[2],
			Key4: keys[3],
		}, nil
	}
}

func StreamGetImageHandler(
	streamer MediaStreamer,
	mediaRepo MediaStorer,
	userRepo MediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetImageEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get image: unmarshalling event: %w", err)
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get image: %w", ErrEmptyImageKey)
		}

		ownNodeInfo := streamer.NodeInfo()
		ownerId := ownNodeInfo.OwnerId
		if ev.UserId == "" {
			ev.UserId = ownerId
		}

		isOwnImageRequest := ownerId == ev.UserId

		if isOwnImageRequest {
			img, err := mediaRepo.GetImage(ev.UserId, ev.Key)
			if errors.Is(err, database.ErrMediaNotFound) || img == "" {
				log.Warnf("get image: key not found: %s", ev.Key)
				return event.GetImageResponse{File: ""}, nil
			}
			if err != nil {
				return nil, fmt.Errorf("get image: fetching media: %w", err)
			}
			return event.GetImageResponse{File: string(img)}, nil
		}

		u, err := userRepo.Get(ev.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			img, _ := mediaRepo.GetImage(ev.UserId, ev.Key)
			return event.GetImageResponse{File: string(img)}, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get image: fetching user: %w", err)
		}

		isOwnAlias := ownNodeInfo.ID.String() == u.NodeId
		if isOwnAlias {
			return event.GetImageResponse{File: ""}, nil
		}

		// Serve the persisted copy first so a foreign avatar (e.g. Mastodon,
		// keyed by URL) survives node restarts and doesn't need a gateway
		// round-trip on every view.
		if stored, cErr := mediaRepo.GetImage(ev.UserId, ev.Key); cErr == nil && stored != "" {
			return event.GetImageResponse{File: string(stored)}, nil
		}

		resp, err := streamer.GenericStream(u.NodeId, event.PUBLIC_GET_IMAGE, ev)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.GetImageResponse{File: ""}, nil
		}
		if err != nil {
			return nil, err
		}

		var imgResp event.GetImageResponse
		if err := json.Unmarshal(resp, &imgResp); err != nil {
			return nil, fmt.Errorf("get image: unmarshalling response: %w", err)
		}

		if err := media_meta.VerifyForeignImage(u, ev.Key, imgResp.File); err != nil {
			log.Warnf("get image: refused media of %s from node %s: %v", u.Id, u.NodeId, err)
			return event.GetImageResponse{File: ""}, nil
		}

		if imgResp.File != "" {
			if err := mediaRepo.SetForeignImageWithTTL(
				u.Id, ev.Key, domain.Base64Image(imgResp.File),
			); err != nil {
				log.Errorf("get image: storing foreign image: %v", err)
			}
		}

		return imgResp, nil
	}
}
