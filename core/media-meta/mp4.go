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
	"encoding/base64"

	"github.com/Warp-net/warpnet/core/video"
	"github.com/Warp-net/warpnet/security"
)

var warpnetUUID = [16]byte{
	0x77, 0x61, 0x72, 0x70, 0x6e, 0x65, 0x74, 0x00, // "warpnet\0"
	0x6d, 0x65, 0x74, 0x61, 0x00, 0x00, 0x00, 0x01, // "meta\0\0\0\1"
}

func EmbedInVideo(videoBytes, metadataBytes []byte) ([]byte, error) {
	return video.AppendUUIDBox(videoBytes, warpnetUUID, []byte(base64.StdEncoding.EncodeToString(metadataBytes)))
}

func SplitVideo(b []byte) (raw, metadataBytes []byte, err error) {
	raw, payloads, err := video.CutUUIDBoxes(b, warpnetUUID)
	if err != nil {
		return nil, nil, err
	}
	switch len(payloads) {
	case 0:
		return raw, nil, nil
	case 1:
	default:
		return nil, nil, ErrAmbiguousMetadata
	}
	metadataBytes, err = base64.StdEncoding.DecodeString(string(payloads[0]))
	if err != nil {
		return nil, nil, ErrNoMetadata
	}
	return raw, metadataBytes, nil
}
func EmbedOrderInVideo(videoBytes, encryptedOrder []byte) ([]byte, error) {
	raw, metadataBytes, err := SplitVideo(videoBytes)
	if err != nil {
		return nil, err
	}
	marked, err := addOrder(metadataBytes, encryptedOrder)
	if err != nil {
		return nil, err
	}
	return EmbedInVideo(raw, marked)
}

func (m Metadata) SignChangedVideo(original, changed []byte) ([]byte, error) {
	_, metadataBytes, err := SplitVideo(original)
	if err != nil {
		return nil, err
	}
	signed, err := parseSignedMetadata(metadataBytes)
	if err != nil {
		return nil, err
	}
	m.EncryptedMeta = signed.EncryptedMeta
	block, err := m.signAt(security.ConvertToSHA256(changed), signed.CreatedAt)
	if err != nil {
		return nil, err
	}
	return EmbedInVideo(changed, block)
}

func VerifyVideo(videoBytes []byte, nodeId, ownerId string) error {
	raw, metadataBytes, err := SplitVideo(videoBytes)
	if err != nil {
		return err
	}
	return verify(metadataBytes, security.ConvertToSHA256(raw), nodeId, ownerId)
}
