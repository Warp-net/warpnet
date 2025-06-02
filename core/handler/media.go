/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/docker/go-units"
	"github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	jis "github.com/dsoprea/go-jpeg-image-structure/v2"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"strings"
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
	imageDescriptionTag = "ImageDescription"

	nodeMetaKey = "node"
	userMetaKey = "user"
	macMetaKey  = "MAC"

	imagePrefix = "data:image/jpeg;base64,"
)

type MediaNodeInformer interface {
	NodeInfo() warpnet.NodeInfo
}

type MediaStorer interface {
	GetImage(userId, key string) (database.Base64Image, error)
	SetImage(userId string, img database.Base64Image) (_ database.ImageKey, err error)
	SetForeignImageWithTTL(userId, key string, img database.Base64Image) error
}

type MediaUserFetcher interface {
	Get(userId string) (user domain.User, err error)
}

func StreamUploadImageHandler(
	info MediaNodeInformer,
	mediaRepo MediaStorer,
	userRepo MediaUserFetcher,
) middleware.WarpHandler {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UploadImageEvent
		if err := json.JSON.Unmarshal(input, &ev); err != nil {
			return nil, err
		}

		parts := strings.SplitN(ev.File, ",", 2)
		if len(parts) != 2 {
			return nil, warpnet.WarpError("invalid base64 image data")
		}

		imgBytes, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("upload: base64 decoding: %w", err)
		}

		if size := binary.Size(imgBytes); size > units.MiB*50 {
			return nil, warpnet.WarpError("image is too large")
		}

		img, _, err := image.Decode(bytes.NewReader(imgBytes))
		if errors.Is(err, image.ErrFormat) {
			return nil, warpnet.WarpError("invalid image format: PNG, JPG, JPEG, GIF are only allowed") // TODO add more types
		}
		if err != nil {
			return nil, fmt.Errorf("upload: image decoding: %w", err)
		}

		var imageBuf bytes.Buffer
		err = jpeg.Encode(&imageBuf, img, &jpeg.Options{Quality: 100})
		if err != nil {
			return nil, fmt.Errorf("upload: JPEG encoding: %w", err)
		}

		nodeInfo := info.NodeInfo()
		ownerUser, err := userRepo.Get(nodeInfo.OwnerId)
		if err != nil {
			return nil, fmt.Errorf("upload: fetching user: %w", err)
		}

		metaData := map[string]any{
			nodeMetaKey: nodeInfo, userMetaKey: ownerUser, macMetaKey: warpnet.GetMacAddr(),
		}
		metaBytes, err := json.JSON.Marshal(metaData)
		if err != nil {
			return nil, fmt.Errorf("upload: marshalling meta data: %w", err)
		}

		encryptedMeta, err := security.EncryptAES(metaBytes, nil) // unknown password
		if err != nil {
			return nil, fmt.Errorf("upload: AES encrypting: %w", err)
		}

		amendedImg, err := amendExifMetadata(imageBuf.Bytes(), encryptedMeta)
		if err != nil {
			return nil, fmt.Errorf("upload: meta data amending: %w", err)
		}

		encoded := base64.StdEncoding.EncodeToString(amendedImg)

		key, err := mediaRepo.SetImage(ownerUser.Id, database.Base64Image(imagePrefix+encoded))
		if err != nil {
			return nil, fmt.Errorf("upload: storing media: %w", err)
		}

		return event.UploadImageResponse{Key: string(key)}, nil
	}
}

type MediaStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

func StreamGetImageHandler(
	streamer MediaStreamer,
	mediaRepo MediaStorer,
	userRepo MediaUserFetcher,
) middleware.WarpHandler {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetImageEvent
		if err := json.JSON.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get image: unmarshalling event: %w", err)
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get image: empty image key")
		}

		ownerId := streamer.NodeInfo().OwnerId
		if ev.UserId == "" {
			ev.UserId = ownerId
		}

		img, err := mediaRepo.GetImage(ev.UserId, ev.Key)
		if err != nil && !errors.Is(err, database.ErrMediaNotFound) {
			return nil, fmt.Errorf("get image: fetching media: %w", err)
		}
		if img != "" {
			return event.GetImageResponse{File: string(img)}, nil
		}

		u, err := userRepo.Get(ev.UserId)
		if err != nil {
			return nil, fmt.Errorf("get image: fetching user: %w", err)
		}

		resp, err := streamer.GenericStream(u.NodeId, event.PUBLIC_GET_IMAGE, ev)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.GetImageResponse{File: ""}, nil
		}
		if err != nil {
			return nil, err
		}

		var imgResp event.GetImageResponse
		if err := json.JSON.Unmarshal(resp, &imgResp); err != nil {
			return nil, fmt.Errorf("get image: unmarshalling response: %w", err)
		}

		return resp, mediaRepo.SetForeignImageWithTTL(u.Id, ev.Key, database.Base64Image(imgResp.File))
	}
}

func amendExifMetadata(imageBytes, metadata []byte) ([]byte, error) {
	parser := jis.NewJpegMediaParser()

	intfc, err := parser.ParseBytes(imageBytes)
	if err != nil {
		return nil, fmt.Errorf("amend EXIF: parse bytes: %w", err)
	}

	sl, ok := intfc.(*jis.SegmentList)
	if !ok {
		return nil, warpnet.WarpError("amend EXIF: invalid exif type: not a segment list")
	}

	ifdMapping, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return nil, fmt.Errorf("amend EXIF: new IFD mapping: %w", err)
	}

	ti := exif.NewTagIndex()

	err = exif.LoadStandardTags(ti)
	if err != nil {
		return nil, fmt.Errorf("amend EXIF: load standard tags: %w", err)
	}

	identity := exifcommon.NewIfdIdentity(
		exifcommon.IfdStandardIfdIdentity.IfdTag(),
		exifcommon.IfdIdentityPart{
			Name:  exifcommon.IfdStandardIfdIdentity.Name(),
			Index: exifcommon.IfdStandardIfdIdentity.Index(),
		},
	)

	rootIb := exif.NewIfdBuilder(ifdMapping, ti, identity, exifcommon.EncodeDefaultByteOrder)

	encodedMetadata := base64.StdEncoding.EncodeToString(metadata)

	err = rootIb.SetStandardWithName(imageDescriptionTag, encodedMetadata)
	if err != nil {
		return nil, fmt.Errorf("amend EXIF: add standard tag: %w", err)
	}

	err = sl.SetExif(rootIb)
	if err != nil {
		return nil, fmt.Errorf("amend EXIF: set: %w", err)
	}

	buf := new(bytes.Buffer)
	err = sl.Write(buf)
	if err != nil {
		return nil, fmt.Errorf("amend EXIF: write bytes: %w", err)
	}

	return buf.Bytes(), nil
}
