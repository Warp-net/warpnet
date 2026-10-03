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
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

func StreamGetSponsoredImageHandler(
	streamer SponsoredStreamer,
	identityKey ed25519.PrivateKey,
	mediaRepo SponsoredMediaStorer,
	copyRepo SponsoredCopyStorer,
	userRepo SponsoredUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetImageEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get sponsored image: unmarshalling event: %w", err)
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get sponsored image: %w", ErrEmptyImageKey)
		}

		ownNodeInfo := streamer.NodeInfo()
		ownerId := ownNodeInfo.OwnerId
		if ev.UserId == "" {
			ev.UserId = ownerId
		}

		if ev.UserId == ownerId {
			c, isBuyer := buyerCopy(s, copyRepo, userRepo, ownerId, ev.Key)
			if !isBuyer {
				log.Warnf("get sponsored image: refused key %s", ev.Key)
				return event.GetImageResponse{File: ""}, nil
			}
			original, err := mediaRepo.GetImage(ownerId, c.OriginalKey)
			if err != nil {
				return nil, fmt.Errorf("get sponsored image: fetching original: %w", err)
			}
			signer := media_meta.Metadata{PrivKey: identityKey, NodeId: ownNodeInfo.ID.String(), OwnerId: ownerId}
			img, err := markFile(string(original), c.EncryptedOrder, imageMarker(c, signer))
			if err != nil {
				return nil, fmt.Errorf("get sponsored image: marking copy: %w", err)
			}
			return event.GetImageResponse{File: img}, nil
		}

		if !isOwnRequest(s, ownNodeInfo) {
			return event.GetImageResponse{File: ""}, nil
		}

		if stored, err := copyRepo.GetImage(ev.UserId, ev.Key); err == nil && stored != "" {
			return event.GetImageResponse{File: string(stored)}, nil
		}

		u, err := userRepo.Get(ev.UserId)
		if err != nil {
			return nil, fmt.Errorf("get sponsored image: fetching user: %w", err)
		}
		if u.NodeId == "" || u.NodeId == ownNodeInfo.ID.String() {
			return event.GetImageResponse{File: ""}, nil
		}

		resp, err := streamer.GenericStream(u.NodeId, event.PUBLIC_GET_SPONSORED_IMAGE, ev)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.GetImageResponse{File: ""}, nil
		}
		if err != nil {
			return nil, err
		}

		var imgResp event.GetImageResponse
		if err := json.Unmarshal(resp, &imgResp); err != nil {
			return nil, fmt.Errorf("get sponsored image: unmarshalling response: %w", err)
		}

		if err := verifyForeignImage(u, ev.Key, imgResp.File); err != nil {
			log.Warnf("get sponsored image: refused media of %s from node %s: %v", u.Id, u.NodeId, err)
			return event.GetImageResponse{File: ""}, nil
		}

		if imgResp.File != "" {
			if err := copyRepo.SetForeignImageWithTTL(
				u.Id, ev.Key, domain.Base64Image(imgResp.File),
			); err != nil {
				log.Errorf("get sponsored image: storing copy: %v", err)
			}
		}

		return imgResp, nil
	}
}

func StreamGetSponsoredVideoHandler(
	streamer SponsoredStreamer,
	mediaRepo SponsoredMediaStorer,
	copyRepo SponsoredCopyStorer,
	userRepo SponsoredUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetVideoEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get sponsored video: unmarshalling event: %w", err)
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get sponsored video: %w", ErrEmptyVideoKey)
		}

		ownNodeInfo := streamer.NodeInfo()
		ownerId := ownNodeInfo.OwnerId
		if ev.UserId == "" {
			ev.UserId = ownerId
		}

		if ev.UserId == ownerId {
			c, isBuyer := buyerCopy(s, copyRepo, userRepo, ownerId, ev.Key)
			if !isBuyer {
				log.Warnf("get sponsored video: refused key %s", ev.Key)
				return event.GetVideoResponse{File: ""}, nil
			}
			original, err := mediaRepo.GetVideo(ownerId, c.OriginalKey)
			if err != nil {
				return nil, fmt.Errorf("get sponsored video: fetching original: %w", err)
			}
			video, err := markFile(string(original), c.EncryptedOrder, media_meta.EmbedOrderInVideo)
			if err != nil {
				return nil, fmt.Errorf("get sponsored video: marking copy: %w", err)
			}
			return newVideoResponse(domain.Base64Video(video), ev.Deferred), nil
		}

		if !isOwnRequest(s, ownNodeInfo) {
			return event.GetVideoResponse{File: ""}, nil
		}

		if stored, err := copyRepo.GetVideo(ev.UserId, ev.Key); err == nil && stored != "" {
			return newVideoResponse(stored, ev.Deferred), nil
		}

		u, err := userRepo.Get(ev.UserId)
		if err != nil {
			return nil, fmt.Errorf("get sponsored video: fetching user: %w", err)
		}
		if u.NodeId == "" || u.NodeId == ownNodeInfo.ID.String() {
			return event.GetVideoResponse{File: ""}, nil
		}
		if ev.Deferred {
			return event.GetVideoResponse{File: "", Deferred: true}, nil
		}

		resp, err := streamer.GenericStream(u.NodeId, event.PUBLIC_GET_SPONSORED_VIDEO, ev)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.GetVideoResponse{File: ""}, nil
		}
		if err != nil {
			return nil, err
		}

		var videoResp event.GetVideoResponse
		if err := json.Unmarshal(resp, &videoResp); err != nil {
			return nil, fmt.Errorf("get sponsored video: unmarshalling response: %w", err)
		}

		if err := verifyForeignVideo(u, ev.Key, videoResp.File); err != nil {
			log.Warnf("get sponsored video: refused media of %s from node %s: %v", u.Id, u.NodeId, err)
			return event.GetVideoResponse{File: ""}, nil
		}

		if videoResp.File != "" {
			if err := copyRepo.SetForeignVideoWithTTL(
				u.Id, ev.Key, domain.Base64Video(videoResp.File),
			); err != nil {
				log.Errorf("get sponsored video: storing copy: %v", err)
			}
		}

		return videoResp, nil
	}
}

func buyerCopy(
	s warpnet.WarpStream,
	copyRepo SponsoredCopyStorer,
	userRepo SponsoredUserFetcher,
	userId, key string,
) (domain.MediaCopy, bool) {
	if s == nil || s.Conn() == nil {
		return domain.MediaCopy{}, false
	}
	c, err := copyRepo.GetCopy(userId, key)
	if err != nil {
		return domain.MediaCopy{}, false
	}
	buyer, err := userRepo.GetByNodeID(s.Conn().RemotePeer().String())
	return c, err == nil && buyer.Id == c.BuyerId
}

func markFile(
	file string,
	encryptedOrder []byte,
	mark func(raw, encryptedOrder []byte) ([]byte, error),
) (string, error) {
	header, raw, err := splitDataURI(file)
	if err != nil {
		return "", err
	}
	marked, err := mark(raw, encryptedOrder)
	if err != nil {
		return "", err
	}
	return header + "," + base64.StdEncoding.EncodeToString(marked), nil
}

func imageMarker(c domain.MediaCopy, signer media_meta.Metadata) func(raw, encryptedOrder []byte) ([]byte, error) {
	if len(c.Watermark) == 0 {
		return media_meta.EmbedOrderInJPEG
	}
	return func(raw, encryptedOrder []byte) ([]byte, error) {
		redrawn, err := media_meta.DrawWatermark(raw, c.Watermark)
		if err != nil {
			return nil, err
		}
		signedJPEG, err := signer.SignChangedJPEG(raw, redrawn)
		if err != nil {
			return nil, err
		}
		return media_meta.EmbedOrderInJPEG(signedJPEG, encryptedOrder)
	}
}
