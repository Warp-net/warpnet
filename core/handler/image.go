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
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"strings"

	"github.com/Warp-net/warpnet/core/mastodon"
	"github.com/Warp-net/warpnet/core/media_meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/docker/go-units"
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
	nodeMetaKey = "node"
	userMetaKey = "user"
	macMetaKey  = "MAC"

	contentKeyLen = 64

	ErrInvalidBase64Signature warpnet.WarpError = "invalid base64 media data"
	ErrMediaKeyMismatch       warpnet.WarpError = "media content does not match the requested key"

	imagePrefix = "data:image/jpeg;base64,"

	ErrTooLargeImage    warpnet.WarpError = "image is too large"
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

		watermark, err := buildWatermark(nodeInfo, privKey, owner)
		if err != nil {
			return nil, err
		}

		var keys [4]string
		for i, file := range images {
			if file == "" {
				continue
			}

			img, err := watermarkUploadedImage(file, watermark)
			if err != nil {
				return nil, fmt.Errorf("upload: image%d: %w", i+1, err)
			}

			key, err := mediaRepo.SetImage(watermark.OwnerId, img)
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
		if cached, err := mediaRepo.GetImage(ev.UserId, ev.Key); err == nil && cached != "" {
			return event.GetImageResponse{File: string(cached)}, nil
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

		if err := acceptForeignImage(u, ev.Key, imgResp.File); err != nil {
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

func acceptForeignImage(u domain.User, key, file string) error {
	return acceptForeignMedia(u, key, file, media_meta.VerifyImage)
}

func isForeignOriginMedia(u domain.User) bool {
	return u.Network == mastodon.Network || u.NodeId == mastodon.GatewayNodeID()
}

func isContentKey(key string) bool {
	if len(key) != contentKeyLen {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}

func verifyContentKey(key, file string) error {
	if !isContentKey(key) {
		return nil
	}
	if hex.EncodeToString(security.ConvertToSHA256([]byte(file))) != key {
		return ErrMediaKeyMismatch
	}
	return nil
}

func acceptForeignMedia(
	u domain.User,
	key, file string,
	verify func(raw []byte, nodeId, ownerId string) error,
) error {
	if file == "" || isForeignOriginMedia(u) {
		return nil
	}
	if err := verifyContentKey(key, file); err != nil {
		return err
	}

	_, raw, err := splitDataURI(file)
	if err != nil {
		return err
	}
	return verify(raw, u.NodeId, u.Id)
}

func splitDataURI(file string) (header string, data []byte, err error) {
	parts := strings.SplitN(file, ",", 2) //nolint:mnd
	if len(parts) != 2 {                  //nolint:mnd
		return "", nil, ErrInvalidBase64Signature
	}

	data, err = base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, fmt.Errorf("base64 decoding: %w", err)
	}
	return parts[0], data, nil
}

func buildWatermark(
	nodeInfo warpnet.NodeInfo,
	privKey ed25519.PrivateKey,
	owner domain.User,
) (media_meta.Watermark, error) {
	metaData := map[string]any{
		nodeMetaKey: nodeInfo, userMetaKey: owner, macMetaKey: warpnet.GetMacAddr(),
	}
	metaBytes, err := json.Marshal(metaData)
	if err != nil {
		return media_meta.Watermark{}, fmt.Errorf("image meta: marshalling meta data: %w", err)
	}

	password, err := security.NewWeakPassword()
	if err != nil {
		return media_meta.Watermark{}, fmt.Errorf("image meta: weak password: %w", err)
	}
	defer security.Wipe(password)

	encryptedMeta, err := security.EncryptAES(metaBytes, password)
	if err != nil {
		return media_meta.Watermark{}, fmt.Errorf("image meta: AES encrypting: %w", err)
	}

	return media_meta.Watermark{
		PrivKey:       privKey,
		NodeId:        nodeInfo.ID.String(),
		OwnerId:       owner.Id,
		EncryptedMeta: encryptedMeta,
	}, nil
}

func watermarkUploadedImage(file string, watermark media_meta.Watermark) (domain.Base64Image, error) {
	_, imgBytes, err := splitDataURI(file)
	if err != nil {
		return "", err
	}

	jpegBytes, err := transcodeToJPEG(imgBytes)
	if err != nil {
		return "", err
	}

	watermarked, err := watermarkJPEG(jpegBytes, watermark)
	if err != nil {
		return "", err
	}

	encoded := base64.StdEncoding.EncodeToString(watermarked)
	return domain.Base64Image(imagePrefix + encoded), nil
}

func transcodeToJPEG(imgBytes []byte) ([]byte, error) {
	if size := binary.Size(imgBytes); size > units.MiB*50 {
		return nil, ErrTooLargeImage
	}

	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if errors.Is(err, image.ErrFormat) {
		return nil, warpnet.WarpError(
			"invalid image format: PNG, JPG, JPEG, GIF are only allowed", // TODO add more types
		)
	}
	if err != nil {
		return nil, fmt.Errorf("image decoding: %w", err)
	}

	var imageBuf bytes.Buffer
	if err := jpeg.Encode(&imageBuf, img, &jpeg.Options{Quality: 100}); err != nil { //nolint:mnd
		return nil, fmt.Errorf("JPEG encoding: %w", err)
	}
	return imageBuf.Bytes(), nil
}

func watermarkJPEG(jpegBytes []byte, watermark media_meta.Watermark) ([]byte, error) {
	watermarkBytes, err := watermark.Sign(security.ConvertToSHA256(jpegBytes))
	if err != nil {
		return nil, fmt.Errorf("meta data signing: %w", err)
	}

	watermarked, err := media_meta.EmbedInJPEG(jpegBytes, watermarkBytes)
	if err != nil {
		return nil, fmt.Errorf("meta data amending: %w", err)
	}

	if err := media_meta.VerifyImage(watermarked, watermark.NodeId, watermark.OwnerId); err != nil {
		return nil, fmt.Errorf("meta data self check: %w", err)
	}
	return watermarked, nil
}
