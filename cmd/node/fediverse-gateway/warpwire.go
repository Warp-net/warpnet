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

package main

// The Warpnet wire essentials — PSK derivation, message signing, bootstrap
// peers, and the stream request/response framing — reimplemented here (copied
// from warpnet's security/config/stream) so the gateway speaks the protocol
// with only native libp2p, no warpnet packages.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const defaultWarpnetNetwork = "warpnet"

// warpnetNetworkMajor must match the MAJOR version of the Warpnet network (the
// repo `version` file): the PSK is keyed on it, so a mismatch fails to connect.
const warpnetNetworkMajor = "0"

// bootstrapByNetwork lists the public entry nodes per network (copied from the
// warpnet config).
var bootstrapByNetwork = map[string][]string{
	defaultWarpnetNetwork: {
		"/ip4/207.154.221.44/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j",
		"/ip4/207.154.221.44/tcp/4002/p2p/12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU",
		"/ip4/207.154.221.44/tcp/4003/p2p/12D3KooWNXSGyfTuYc3JznW48jay73BtQgHszWfPpyF581EWcpGJ",
		"/ip4/130.94.88.38/tcp/4011/p2p/12D3KooWNW7nbLpbsEVJ86JN6c1zXRDKGCbqmLfhitFCPccRv2YW",
	},
	"testnet": {
		"/ip4/207.154.221.44/tcp/4011/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j",
		"/ip4/207.154.221.44/tcp/4022/p2p/12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU",
		"/ip4/207.154.221.44/tcp/4033/p2p/12D3KooWNXSGyfTuYc3JznW48jay73BtQgHszWfPpyF581EWcpGJ",
	},
}

// spbFounding anchors the PSK entropy (copied verbatim from warpnet security).
const spbFounding = -((int64(133129) << 16) + 51200)

func generateAnchoredEntropy() []byte {
	input := []byte(strconv.FormatInt(spbFounding, 10))
	for range 10 {
		sum := sha256.Sum256(input)
		input = sum[:]
	}
	return input
}

// generatePSK derives the 32-byte private-network key for the given network,
// byte-identical to warpnet's security.GeneratePSK.
func generatePSK(network string) []byte {
	if network == "mainnet" {
		network = defaultWarpnetNetwork
	}
	seed := append([]byte(network), []byte(warpnetNetworkMajor)...)
	seed = append(seed, generateAnchoredEntropy()...)
	sum := sha256.Sum256(seed)
	return sum[:]
}

// signBody signs the request body with the node's ed25519 key (warpnet's
// security.Sign): base64(ed25519 signature).
func signBody(priv ed25519.PrivateKey, body []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, body))
}

// streamSend opens a libp2p stream on the route's protocol ID, writes the signed
// message envelope, and reads the full response — warpnet's stream framing.
func streamSend(ctx context.Context, h host.Host, p peer.ID, priv ed25519.PrivateKey, route string, payload any) ([]byte, error) {
	var body []byte
	if payload != nil {
		if b, ok := payload.([]byte); ok {
			body = b
		} else {
			var err error
			if body, err = json.Marshal(payload); err != nil {
				return nil, fmt.Errorf("stream: marshal payload: %w", err)
			}
		}
	}

	s, err := h.NewStream(ctx, p, protocol.ID(route))
	if err != nil {
		return nil, fmt.Errorf("stream: new: %w", err)
	}
	defer func() { _ = s.Close() }()

	data, err := json.Marshal(message{
		Body:        json.RawMessage(body),
		MessageId:   uuid.New().String(),
		NodeId:      h.ID().String(),
		Destination: route,
		Timestamp:   time.Now(),
		Version:     "0.0.0",
		Signature:   signBody(priv, body),
	})
	if err != nil {
		return nil, fmt.Errorf("stream: marshal envelope: %w", err)
	}

	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
	if _, err := rw.Write(data); err != nil {
		return nil, fmt.Errorf("stream: write: %w", err)
	}
	if err := rw.Flush(); err != nil {
		return nil, fmt.Errorf("stream: flush: %w", err)
	}
	_ = s.CloseWrite()

	buf := bytes.NewBuffer(nil)
	if _, err := buf.ReadFrom(rw); err != nil {
		return nil, fmt.Errorf("stream: read: %w", err)
	}
	return buf.Bytes(), nil
}
