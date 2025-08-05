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
	"encoding/hex"
	"errors"
	"fmt"
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
	Validate(ev event.ValidationEvent) error
	ValidationResult(ev event.ValidationResultEvent) error
}

// TODO nonce cache check
func StreamChallengeHandler(fs FileSystem, privateKey ed25519.PrivateKey) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		if fs == nil {
			panic("challenge handler called with nil file system")
		}
		var req event.ChallengeEvent
		err := json.Unmarshal(buf, &req)
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

		return event.ChallengeResponse{
			Challenge: hex.EncodeToString(challenge),
			Signature: security.Sign(privateKey, challenge),
		}, nil
	}
}

func StreamValidateHandler(svc AdminConsensusServicer) warpnet.WarpHandlerFunc {
	if svc == nil {
		panic("validate handler called with nil service")
	}

	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		if len(buf) == 0 {
			return nil, errors.New("gossip consensus: empty data")
		}

		var ev event.ValidationEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			log.Errorf("pubsub: failed to decode user update message: %v %s", err, buf)
			return nil, err
		}

		if ev.ValidatedNodeID == s.Conn().LocalPeer().String() { // no need to validate self
			return event.Accepted, nil
		}
		return event.Accepted, svc.Validate(ev)
	}
}

func StreamValidationResponseHandler(svc AdminConsensusServicer) warpnet.WarpHandlerFunc {
	if svc == nil {
		panic("validation result handler called with nil service")
	}
	return func(data []byte, s warpnet.WarpStream) (any, error) {
		if len(data) == 0 {
			fmt.Println("ValidationResult empty data")
			return nil, errors.New("validation result handler: empty data")
		}

		var ev event.ValidationResultEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			log.Errorf("validation result handler: failed to decode validation result: %v %s", err, data)
			return nil, err
		}
		ev.ValidatorID = s.Conn().RemotePeer().String()
		if ev.ValidatorID == s.Conn().LocalPeer().String() { // no need to validate self
			return event.Accepted, nil
		}
		if err := svc.ValidationResult(ev); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}
