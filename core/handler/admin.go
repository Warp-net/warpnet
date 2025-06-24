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
	"encoding/hex"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
	"io/fs"
)

type FileSystem interface {
	ReadDir(name string) ([]fs.DirEntry, error)
	ReadFile(name string) ([]byte, error)
	Open(name string) (fs.File, error)
}

type AdminConsensusServicer interface {
	Validate(data []byte, _ warpnet.WarpStream) (any, error)
	ValidationResult(data []byte, s warpnet.WarpStream) (any, error)
}

// TODO nonce cache check
func StreamChallengeHandler(fs FileSystem, privateKey ed25519.PrivateKey) middleware.WarpHandler {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		if fs == nil {
			panic("challenge handler called with nil file system")
		}
		var req event.GetChallengeEvent
		err := json.JSON.Unmarshal(buf, &req)
		if err != nil {
			return nil, err
		}

		challenge, err := security.ResolveChallenge(
			fs,
			security.SampleLocation{
				DirStack:  req.DirStack,
				FileStack: req.FileStack,
			},
			req.Nonce,
		)
		if err != nil {
			return nil, err
		}

		sig := ed25519.Sign(privateKey, challenge)

		return event.GetChallengeResponse{
			Challenge: hex.EncodeToString(challenge),
			Signature: base64.StdEncoding.EncodeToString(sig),
		}, nil
	}
}

func StreamValidateHandler(svc AdminConsensusServicer) middleware.WarpHandler {
	if svc == nil {
		return nil
	}
	log.Infoln("StreamValidateHandler event")
	return svc.Validate
}

func StreamValidationResponseHandler(svc AdminConsensusServicer) middleware.WarpHandler {
	if svc == nil {
		return nil
	}
	log.Infoln("StreamValidationResponseHandler event")

	return svc.ValidationResult
}
