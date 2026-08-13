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
	"crypto/ed25519"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
)

const (
	metaVersion = 1

	signDomain = "warpnet/media-meta/v1"

	ErrNoMetadata        warpnet.WarpError = "media carries no warpnet metadata"
	ErrForgedMetadata    warpnet.WarpError = "media metadata does not match the file or its signer"
	ErrAmbiguousMetadata warpnet.WarpError = "media carries more than one warpnet metadata block"
	ErrNoSigningKey      warpnet.WarpError = "media meta: no signing key"
	ErrNoSigningIdentity warpnet.WarpError = "media meta: no signing identity"
)

type signedWatermark struct {
	Version       uint8     `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	EncryptedMeta []byte    `json:"encrypted_meta"`
	Signature     string    `json:"signature"`
}

type Watermark struct {
	PrivKey       ed25519.PrivateKey
	NodeId        string
	OwnerId       string
	EncryptedMeta []byte
}

func (w Watermark) Sign(rawHash []byte) ([]byte, error) {
	if err := w.validate(); err != nil {
		return nil, err
	}

	signed := signedWatermark{
		Version:       metaVersion,
		CreatedAt:     time.Now().UTC(),
		EncryptedMeta: w.EncryptedMeta,
	}
	signed.Signature = security.Sign(
		w.PrivKey,
		signingBytes(signed.Version, signed.CreatedAt, w.NodeId, w.OwnerId, rawHash, signed.EncryptedMeta),
	)
	return json.Marshal(signed)
}

func verify(watermarkBytes, rawHash []byte, nodeId, ownerId string) error {
	signed, err := parseSignedWatermark(watermarkBytes)
	if err != nil {
		return err
	}
	if nodeId == "" || ownerId == "" {
		return ErrForgedMetadata
	}

	pubKey := warpnet.FromIDToPubKey(warpnet.FromStringToPeerID(nodeId))
	if len(pubKey) == 0 {
		return ErrForgedMetadata
	}

	body := signingBytes(signed.Version, signed.CreatedAt, nodeId, ownerId, rawHash, signed.EncryptedMeta)
	if err := security.VerifySignature(pubKey, body, signed.Signature); err != nil {
		return ErrForgedMetadata
	}
	return nil
}

func parseSignedWatermark(b []byte) (signedWatermark, error) {
	var signed signedWatermark
	if len(b) == 0 {
		return signed, ErrNoMetadata
	}
	if err := json.Unmarshal(b, &signed); err != nil {
		return signed, ErrNoMetadata
	}
	if signed.Version != metaVersion || signed.Signature == "" || len(signed.EncryptedMeta) == 0 {
		return signed, ErrNoMetadata
	}
	return signed, nil
}

func signingBytes(
	blockVersion uint8,
	createdAt time.Time,
	nodeId, ownerId string,
	rawHash, encryptedMeta []byte,
) []byte {
	return []byte(strings.Join([]string{
		signDomain,
		strconv.Itoa(int(blockVersion)),
		nodeId,
		ownerId,
		hex.EncodeToString(rawHash),
		strconv.FormatInt(createdAt.UTC().UnixNano(), 10),
		hex.EncodeToString(security.ConvertToSHA256(encryptedMeta)),
	}, "\x00"))
}

func (w Watermark) validate() error {
	if len(w.PrivKey) != ed25519.PrivateKeySize {
		return ErrNoSigningKey
	}
	if w.NodeId == "" || w.OwnerId == "" {
		return ErrNoSigningIdentity
	}
	return nil
}
