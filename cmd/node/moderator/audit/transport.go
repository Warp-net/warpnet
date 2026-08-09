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

WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package audit

import (
	"fmt"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

const ErrChallengeRefused warpnet.WarpError = "audit: peer refused the challenge"

// Streamer is the slice of the moderator node this transport dials with.
type Streamer interface {
	GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error)
}

// StreamChallenger is the wire side of an audit: it carries a challenge
// over a Warpnet stream and decodes the answer. It is the only place in
// this package that knows about routes, streams or encoding — the audit
// logic sees nothing but Challenger.
type StreamChallenger struct {
	node Streamer
}

func NewStreamChallenger(node Streamer) *StreamChallenger {
	return &StreamChallenger{node: node}
}

func (c *StreamChallenger) Ask(peer string, ch Challenge) (ChallengeResponse, error) {
	data, err := c.node.GenericStream(peer, ChallengeRoute, ch)
	if err != nil {
		return ChallengeResponse{}, err
	}
	// A handler that rejected the challenge answers with an error
	// envelope, which must never be parsed as an answer.
	var respErr event.ResponseError
	if json.Unmarshal(data, &respErr) == nil && respErr.Message != "" {
		return ChallengeResponse{}, fmt.Errorf("%w: %s", ErrChallengeRefused, respErr.Message)
	}

	var resp ChallengeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return ChallengeResponse{}, err
	}
	return resp, nil
}
