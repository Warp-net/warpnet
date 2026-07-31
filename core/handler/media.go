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
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"math"
	"strings"

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
	log "github.com/sirupsen/logrus"
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

	// maxVideoSize caps the decoded upload. Kept in step with
	// middleware.VideoMaxLimit, which caps the base64 envelope carrying it.
	maxVideoSize = units.MiB * 50

	ErrTooLargeImage          warpnet.WarpError = "image is too large"
	ErrInvalidBase64Signature warpnet.WarpError = "invalid base64 image data"
	ErrEmptyImageKey          warpnet.WarpError = "empty image key"
	ErrNoImagesProvided       warpnet.WarpError = "at least one image must be provided"
	ErrInvalidEXIF            warpnet.WarpError = "invalid exif type: not a segment list"

	ErrTooLargeVideo    warpnet.WarpError = "video is too large: 50 MiB is the maximum"
	ErrEmptyVideoKey    warpnet.WarpError = "empty video key"
	ErrNoVideoProvided  warpnet.WarpError = "a video must be provided"
	ErrUnsupportedVideo warpnet.WarpError = "unsupported video format: only MP4 and MOV (QuickTime) files are accepted"
)

type MediaNodeInformer interface {
	NodeInfo() warpnet.NodeInfo
}

type MediaStorer interface {
	GetImage(userId, key string) (database.Base64Image, error)
	SetImage(userId string, img database.Base64Image) (_ database.ImageKey, err error)
	SetForeignImageWithTTL(userId, key string, img database.Base64Image) error
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

type MediaUserFetcher interface {
	Get(userId string) (user domain.User, err error)
}

func StreamUploadImageHandler(
	info MediaNodeInformer,
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

		encryptedMeta, ownerUser, err := buildEncryptedImageMeta(info, userRepo)
		if err != nil {
			return nil, err
		}

		var keys [4]string
		for i, file := range images {
			if file == "" {
				continue
			}

			key, err := processAndStoreImage(file, encryptedMeta, ownerUser.Id, mediaRepo)
			if err != nil {
				return nil, fmt.Errorf("upload: image%d: %w", i+1, err)
			}
			keys[i] = key
		}

		return event.UploadImageResponse{
			Key1: keys[0],
			Key2: keys[1],
			Key3: keys[2],
			Key4: keys[3],
		}, nil
	}
}

// buildEncryptedImageMeta fetches the owner user and produces the
// AES-encrypted {node,user,MAC} blob embedded into uploaded media EXIF.
// The encryption password is random and immediately discarded (see the
// file header), so the metadata stands as proof of ownership without
// leaking its contents. Shared by the image-upload and archive-import
// handlers.
func buildEncryptedImageMeta(
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

func processAndStoreImage(
	file string,
	encryptedMeta []byte,
	userId string,
	mediaRepo MediaStorer,
) (string, error) {
	parts := strings.SplitN(file, ",", 2) //nolint:mnd
	if len(parts) != 2 {                  //nolint:mnd
		return "", ErrInvalidBase64Signature
	}

	imgBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("base64 decoding: %w", err)
	}

	if size := binary.Size(imgBytes); size > units.MiB*50 {
		return "", ErrTooLargeImage
	}

	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if errors.Is(err, image.ErrFormat) {
		return "", warpnet.WarpError(
			"invalid image format: PNG, JPG, JPEG, GIF are only allowed", // TODO add more types
		)
	}
	if err != nil {
		return "", fmt.Errorf("image decoding: %w", err)
	}

	var imageBuf bytes.Buffer
	err = jpeg.Encode(&imageBuf, img, &jpeg.Options{Quality: 100}) //nolint:mnd
	if err != nil {
		return "", fmt.Errorf("JPEG encoding: %w", err)
	}

	amendedImg, err := amendExifMetadata(imageBuf.Bytes(), encryptedMeta)
	if err != nil {
		return "", fmt.Errorf("meta data amending: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(amendedImg)

	key, err := mediaRepo.SetImage(userId, database.Base64Image(imagePrefix+encoded))
	if err != nil {
		return "", fmt.Errorf("storing media: %w", err)
	}

	return string(key), nil
}

type VideoStorer interface {
	GetVideo(userId, key string) (database.Base64Video, error)
	SetVideo(userId string, video database.Base64Video) (_ database.VideoKey, err error)
	SetForeignVideoWithTTL(userId, key string, video database.Base64Video) error
}

func StreamUploadVideoHandler(
	info MediaNodeInformer,
	mediaRepo VideoStorer,
	userRepo MediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UploadVideoEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, err
		}
		if ev.Video == "" {
			return nil, ErrNoVideoProvided
		}

		encryptedMeta, ownerUser, err := buildEncryptedImageMeta(info, userRepo)
		if err != nil {
			return nil, err
		}

		key, err := processAndStoreVideo(ev.Video, encryptedMeta, ownerUser.Id, mediaRepo)
		if err != nil {
			return nil, fmt.Errorf("upload: video: %w", err)
		}

		return event.UploadVideoResponse{Key: key}, nil
	}
}

// processAndStoreVideo validates the container, stamps the encrypted
// ownership blob into it and stores the result. Unlike the image path there
// is no re-encode: the node has no codec, so the uploaded bytes are kept
// verbatim and playback is left to the client's system codecs.
func processAndStoreVideo(
	file string,
	encryptedMeta []byte,
	userId string,
	mediaRepo VideoStorer,
) (string, error) {
	parts := strings.SplitN(file, ",", 2) //nolint:mnd
	if len(parts) != 2 {                  //nolint:mnd
		return "", ErrInvalidBase64Signature
	}

	// The stored prefix is echoed straight back into the client's player, so
	// it is rebuilt from a fixed allow-list rather than trusted from the
	// caller. It also must not be normalised to mp4: nothing is transcoded
	// here, so the declared type has to keep matching the actual bytes.
	prefix, ok := videoDataPrefix(parts[0])
	if !ok {
		return "", ErrUnsupportedVideo
	}

	videoBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("base64 decoding: %w", err)
	}

	if len(videoBytes) > maxVideoSize {
		return "", ErrTooLargeVideo
	}

	if !isISOBaseMediaFile(videoBytes) {
		return "", ErrUnsupportedVideo
	}

	amendedVideo, err := amendVideoMetadata(videoBytes, encryptedMeta)
	if err != nil {
		return "", fmt.Errorf("meta data amending: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(amendedVideo)

	key, err := mediaRepo.SetVideo(userId, database.Base64Video(prefix+encoded))
	if err != nil {
		return "", fmt.Errorf("storing media: %w", err)
	}

	return string(key), nil
}

func StreamGetVideoHandler(
	streamer MediaStreamer,
	mediaRepo VideoStorer,
	userRepo MediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetVideoEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get video: unmarshalling event: %w", err)
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get video: %w", ErrEmptyVideoKey)
		}

		ownNodeInfo := streamer.NodeInfo()
		ownerId := ownNodeInfo.OwnerId
		if ev.UserId == "" {
			ev.UserId = ownerId
		}

		isOwnVideoRequest := ownerId == ev.UserId

		if isOwnVideoRequest {
			video, err := mediaRepo.GetVideo(ev.UserId, ev.Key)
			if errors.Is(err, database.ErrMediaNotFound) || video == "" {
				log.Warnf("get video: key not found: %s", ev.Key)
				return event.GetVideoResponse{File: ""}, nil
			}
			if err != nil {
				return nil, fmt.Errorf("get video: fetching media: %w", err)
			}
			return newVideoResponse(video, ev.Deferred), nil
		}

		u, err := userRepo.Get(ev.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			video, _ := mediaRepo.GetVideo(ev.UserId, ev.Key)
			return newVideoResponse(video, ev.Deferred), nil
		}
		if err != nil {
			return nil, fmt.Errorf("get video: fetching user: %w", err)
		}

		isOwnAlias := ownNodeInfo.ID.String() == u.NodeId
		if isOwnAlias {
			return event.GetVideoResponse{File: ""}, nil
		}

		// Serve the persisted copy first so a video already pulled from its
		// owner doesn't need another round-trip on every view.
		if cached, cErr := mediaRepo.GetVideo(ev.UserId, ev.Key); cErr == nil && cached != "" {
			return newVideoResponse(cached, ev.Deferred), nil
		}

		// A deferred caller only wants to know the video exists; skip the
		// (potentially large) remote fetch entirely rather than pulling the
		// payload just to drop it.
		if ev.Deferred {
			return event.GetVideoResponse{File: "", Deferred: true}, nil
		}

		resp, err := streamer.GenericStream(u.NodeId, event.PUBLIC_GET_VIDEO, ev)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.GetVideoResponse{File: ""}, nil
		}
		if err != nil {
			return nil, err
		}

		var videoResp event.GetVideoResponse
		if err := json.Unmarshal(resp, &videoResp); err != nil {
			return nil, fmt.Errorf("get video: unmarshalling response: %w", err)
		}

		if videoResp.File != "" {
			if err := mediaRepo.SetForeignVideoWithTTL(
				u.Id, ev.Key, database.Base64Video(videoResp.File),
			); err != nil {
				log.Errorf("get video: storing foreign video: %v", err)
			}
		}

		return videoResp, nil
	}
}

// newVideoResponse reports the payload size either way, so a deferred caller
// can show the download cost before committing to it.
func newVideoResponse(video database.Base64Video, deferred bool) event.GetVideoResponse {
	if deferred {
		return event.GetVideoResponse{
			File:     "",
			Size:     int64(len(video)),
			Deferred: true,
		}
	}
	return event.GetVideoResponse{File: string(video), Size: int64(len(video))}
}

type MediaStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
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
		if cached, cErr := mediaRepo.GetImage(ev.UserId, ev.Key); cErr == nil && cached != "" {
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

		if err := mediaRepo.SetForeignImageWithTTL(u.Id, ev.Key, database.Base64Image(imgResp.File)); err != nil {
			log.Errorf("get image: storing foreign image: %v", err)
		}

		return resp, nil
	}
}

// warpnetMetaUUID identifies the top-level ISO-BMFF `uuid` box carrying the
// encrypted ownership blob — the video counterpart of the EXIF
// ImageDescription stamp. Readers that don't recognise the UUID skip the box,
// so the file stays playable everywhere.
var warpnetMetaUUID = [16]byte{
	0x77, 0x61, 0x72, 0x70, 0x6e, 0x65, 0x74, 0x00, // "warpnet\0"
	0x6d, 0x65, 0x74, 0x61, 0x00, 0x00, 0x00, 0x01, // "meta\0\0\0\1"
}

const (
	boxHeaderSize = 8
	boxUUIDSize   = 16
)

// acceptedVideoPrefixes maps the MIME types the node accepts to the data-URL
// prefix it stores. Rebuilding the prefix from this table keeps a caller from
// smuggling an arbitrary data URL into every viewer's player.
var acceptedVideoPrefixes = map[string]string{
	"video/mp4":       "data:video/mp4;base64,",
	"video/quicktime": "data:video/quicktime;base64,",
	"video/x-m4v":     "data:video/x-m4v;base64,",
}

// videoDataPrefix validates the "data:<mime>;base64" header of an uploaded
// data URL and returns the prefix to store the payload under.
func videoDataPrefix(header string) (string, bool) {
	header = strings.TrimPrefix(header, "data:")
	mime, params, _ := strings.Cut(header, ";")
	if params != "base64" {
		return "", false
	}
	prefix, ok := acceptedVideoPrefixes[strings.ToLower(strings.TrimSpace(mime))]
	return prefix, ok
}

// isISOBaseMediaFile reports whether b looks like an ISO base media file
// (MP4 / M4V / QuickTime MOV). Only the container is checked: per the
// project's stance, decoding is the client system's job, so an exotic codec
// inside a valid MP4 is accepted and simply won't play without codecs.
func isISOBaseMediaFile(b []byte) bool {
	// Walk the leading top-level boxes looking for `ftyp`. Some muxers emit
	// a `wide`, `free` or `skip` box before it.
	for offset := 0; offset+boxHeaderSize <= len(b); {
		size := int(binary.BigEndian.Uint32(b[offset : offset+4]))
		boxType := string(b[offset+4 : offset+boxHeaderSize])

		if boxType == "ftyp" {
			return true
		}
		// Only these may legally precede ftyp; anything else means this is
		// not an ISO base media file.
		if boxType != "wide" && boxType != "free" && boxType != "skip" {
			return false
		}
		if size < boxHeaderSize {
			return false
		}
		offset += size
	}
	return false
}

// amendVideoMetadata appends a top-level `uuid` box holding the encrypted
// {node,user,MAC} blob. Appending is non-destructive: unlike the image path
// there is no re-encode, so the original stream is preserved byte for byte.
func amendVideoMetadata(videoBytes, metadata []byte) ([]byte, error) {
	encodedMetadata := base64.StdEncoding.EncodeToString(metadata)

	boxSize := boxHeaderSize + boxUUIDSize + len(encodedMetadata)
	if boxSize > math.MaxUint32 {
		return nil, warpnet.WarpError("amend video meta: metadata box too large")
	}

	buf := bytes.NewBuffer(make([]byte, 0, len(videoBytes)+boxSize))
	buf.Write(videoBytes)

	header := make([]byte, boxHeaderSize)
	binary.BigEndian.PutUint32(header, uint32(boxSize)) //nolint:gosec
	copy(header[4:], "uuid")

	buf.Write(header)
	buf.Write(warpnetMetaUUID[:])
	buf.WriteString(encodedMetadata)

	return buf.Bytes(), nil
}

func amendExifMetadata(imageBytes, metadata []byte) ([]byte, error) {
	parser := jis.NewJpegMediaParser()

	intfc, err := parser.ParseBytes(imageBytes)
	if err != nil {
		return nil, fmt.Errorf("amend EXIF: parse bytes: %w", err)
	}

	sl, ok := intfc.(*jis.SegmentList)
	if !ok {
		return nil, fmt.Errorf("amend EXIF: %w", ErrInvalidEXIF)
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
