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

package media_meta

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/docker/go-units"
)

const (
	nodeMetaKey = "node"
	userMetaKey = "user"
	macMetaKey  = "MAC"

	ImagePrefix = "data:image/jpeg;base64,"

	ErrTooLargeImage warpnet.WarpError = "image is too large"
)

func BuildMetadata(
	nodeInfo warpnet.NodeInfo,
	privKey ed25519.PrivateKey,
	owner domain.User,
) (Metadata, error) {
	metaData := map[string]any{
		nodeMetaKey: nodeInfo, userMetaKey: owner, macMetaKey: warpnet.GetMacAddr(),
	}
	metaBytes, err := json.Marshal(metaData)
	if err != nil {
		return Metadata{}, fmt.Errorf("image meta: marshalling meta data: %w", err)
	}

	password, err := security.NewWeakPassword()
	if err != nil {
		return Metadata{}, fmt.Errorf("image meta: weak password: %w", err)
	}
	defer security.Wipe(password)

	encryptedMeta, err := security.EncryptAES(metaBytes, password)
	if err != nil {
		return Metadata{}, fmt.Errorf("image meta: AES encrypting: %w", err)
	}

	return Metadata{
		PrivKey:       privKey,
		NodeId:        nodeInfo.ID.String(),
		OwnerId:       owner.Id,
		EncryptedMeta: encryptedMeta,
	}, nil
}

func SignUploadedImage(file string, metadata Metadata) (domain.Base64Image, error) {
	_, imgBytes, err := SplitDataURI(file)
	if err != nil {
		return "", err
	}

	jpegBytes, err := transcodeToJPEG(imgBytes)
	if err != nil {
		return "", err
	}

	signed, err := signJPEG(jpegBytes, metadata)
	if err != nil {
		return "", err
	}

	encoded := base64.StdEncoding.EncodeToString(signed)
	return domain.Base64Image(ImagePrefix + encoded), nil
}

func transcodeToJPEG(imgBytes []byte) ([]byte, error) {
	if size := binary.Size(imgBytes); size > units.MiB*50 {
		return nil, ErrTooLargeImage
	}

	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if errors.Is(err, image.ErrFormat) {
		return nil, warpnet.WarpError(
			"invalid image format: PNG, JPG, JPEG, GIF are only allowed",
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

func signJPEG(jpegBytes []byte, metadata Metadata) ([]byte, error) {
	metadataBytes, err := metadata.Sign(security.ConvertToSHA256(jpegBytes))
	if err != nil {
		return nil, fmt.Errorf("meta data signing: %w", err)
	}

	signed, err := EmbedInJPEG(jpegBytes, metadataBytes)
	if err != nil {
		return nil, fmt.Errorf("meta data amending: %w", err)
	}

	if err := VerifyImage(signed, metadata.NodeId, metadata.OwnerId); err != nil {
		return nil, fmt.Errorf("meta data self check: %w", err)
	}
	return signed, nil
}
