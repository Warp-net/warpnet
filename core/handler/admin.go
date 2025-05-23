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
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/Warp-net/warpnet/core/consensus"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"io/fs"
)

type AdminStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
}

type AdminStateCommitter interface {
	CommitState(newState consensus.KVState) (_ *consensus.KVState, err error)
}

type ConsensusResetter interface {
	Reset() error
}

func StreamVerifyHandler(state AdminStateCommitter) middleware.WarpHandler {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		if state == nil {
			return nil, nil
		}
		var newState map[string]string
		err := json.JSON.Unmarshal(buf, &newState)
		if err != nil {
			return nil, err
		}

		updatedState, err := state.CommitState(newState)
		if err != nil {
			return nil, err
		}

		return updatedState, nil
	}
}

func StreamConsensusResetHandler(consRepo ConsensusResetter) middleware.WarpHandler {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		if consRepo == nil {
			return nil, nil
		}

		return event.Accepted, consRepo.Reset()
	}
}

type FileSystem interface {
	ReadDir(name string) ([]fs.DirEntry, error)
	ReadFile(name string) ([]byte, error)
	Open(name string) (fs.File, error)
}

// TODO nonce cache check
func StreamChallengeHandler(fs FileSystem, privateKey warpnet.WarpPrivateKey) middleware.WarpHandler {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		if fs == nil {
			panic("challenge handler called with nil file system")
		}
		var req event.GetChallengeEvent
		err := json.JSON.Unmarshal(buf, &req)
		if err != nil {
			return nil, err
		}

		codeHash, err := security.GetCodebaseHash(fs)
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

		edKey, err := privateKey.Raw()
		if err != nil {
			return nil, fmt.Errorf("challenge handler failed to get raw ed25519 key: %v", err)
		}

		sig := ed25519.Sign(edKey, challenge)

		return event.GetChallengeResponse{
			Challenge: hex.EncodeToString(challenge),
			CodeHash:  hex.EncodeToString(codeHash),
			Signature: base64.StdEncoding.EncodeToString(sig),
		}, nil
	}
}
