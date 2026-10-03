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
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/security"
)

const (
	contentKeyLen = 64

	ErrInvalidBase64Signature warpnet.WarpError = "invalid base64 media data"
	ErrMediaKeyMismatch       warpnet.WarpError = "media content does not match the requested key"
)

func VerifyForeignImage(u domain.User, key, file string) error {
	return verifyForeignMedia(u, key, file, VerifyImage)
}

func VerifyForeignVideo(u domain.User, key, file string) error {
	return verifyForeignMedia(u, key, file, VerifyVideo)
}

func SplitDataURI(file string) (header string, data []byte, err error) {
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

func BuildContentKey(file string) string {
	return hex.EncodeToString(security.ConvertToSHA256([]byte(file)))
}

func verifyForeignMedia(
	u domain.User,
	key, file string,
	verifyMetadata func(raw []byte, nodeId, ownerId string) error,
) error {
	if file == "" || isForeignOriginMedia(u) {
		return nil
	}
	if err := verifyContentKey(key, file); err != nil {
		return err
	}

	_, raw, err := SplitDataURI(file)
	if err != nil {
		return err
	}
	return verifyMetadata(raw, u.NodeId, u.Id)
}

func isForeignOriginMedia(u domain.User) bool {
	return u.Network == fediverse.MastodonNetwork || u.NodeId == fediverse.GatewayNodeID()
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
	if BuildContentKey(file) != key {
		return ErrMediaKeyMismatch
	}
	return nil
}
