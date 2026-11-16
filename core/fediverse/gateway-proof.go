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

package fediverse

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multistream"
	"github.com/oklog/ulid/v2"
)

// DefaultGatewayPublicKeyPEM is the default gateway's RSA HTTP-signature key.
// Its libp2p key comes from a public seed, so only this key proves that a
// connection to DefaultGatewayNodeID reaches the real gateway.
const DefaultGatewayPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAx2NS63Vs4zK244Pao8zU
CA2ERMFJj2xNFPVfqQ+SYGRZfXWdrTQOy73cqwdCxdn0/xazReCBG/YJ2zsYRa4c
CslZri0LaQAMl7CvbbSGKfGJeideCMAxIcVWOAdW/Ml7BS3ii1QteFehSKcq/Tf0
kbon/+OmFrNdHivJE9Fm7ao7/ce4SOMTuJTWR3DmGhrE9piE/5ECNcxb2kuV59ZE
kkIL/eqTxdMiNDo0h0npHuyd0cbgxdBK8OLP9LwCI+LyDme9ZG8k+n/mtsm63hIz
bmjRh/OeyRHim5VlT0hqr0LgBS63jBisfSGYWxwI5Ik77kJwu6Kw8di4GsRLoaFC
DQIDAQAB
-----END PUBLIC KEY-----
`

const (
	gatewayProofNonceSize = 32
	maxGatewayProofSize   = 4096
)

var errBadGatewayKey = errors.New("fediverse: gateway public key is not an RSA PEM key")

// GatewayProofBytes is what the gateway signs: the nonce bound to both ends of
// the connection, so a proof cannot be relayed to another node.
func GatewayProofBytes(nonce []byte, clientID, gatewayID string) []byte {
	bt := []byte("warpnet-gateway-proof\x00")
	bt = append(bt, nonce...)
	bt = append(bt, 0)
	bt = append(bt, clientID...)
	bt = append(bt, 0)
	return append(bt, gatewayID...)
}

func parseRSAPublicKey(pemKey string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, errBadGatewayKey
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errBadGatewayKey
	}
	return rsaPub, nil
}

// NewGatewayProver challenges a connection to the gateway: it sends a fresh
// nonce on that very connection, signed like any request with the node's own
// key, and checks the gateway's RSA signature over GatewayProofBytes.
func NewGatewayProver(privKey ed25519.PrivateKey, publicKeyPEM string) (warpnet.ConnProver, error) {
	pub, err := parseRSAPublicKey(publicKeyPEM)
	if err != nil {
		return nil, err
	}
	route := protocol.ID(event.PUBLIC_GET_GATEWAY_PROOF)
	return func(ctx context.Context, c warpnet.WarpConn) error {
		nonce := make([]byte, gatewayProofNonceSize)
		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		body, err := json.Marshal(event.GatewayProofEvent{Nonce: base64.StdEncoding.EncodeToString(nonce)})
		if err != nil {
			return err
		}
		msg := event.Message{
			Body:        body,
			MessageId:   ulid.Make().String(),
			NodeId:      c.LocalPeer().String(),
			Destination: string(route),
			Timestamp:   time.Now().UTC(),
			Version:     "0.0.0",
		}
		msg.Signature = security.Sign(privKey, msg.SigningBytes())
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}

		// The gateway is reachable only through relays.
		s, err := c.NewStream(network.WithAllowLimitedConn(ctx, string(route)))
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		if deadline, ok := ctx.Deadline(); ok {
			_ = s.SetDeadline(deadline)
		}
		if err := s.SetProtocol(route); err != nil {
			return err
		}
		if err := multistream.SelectProtoOrFail(route, s); err != nil {
			return err
		}
		if _, err := s.Write(data); err != nil {
			return err
		}
		if err := s.CloseWrite(); err != nil {
			return err
		}
		resp, err := io.ReadAll(io.LimitReader(s, maxGatewayProofSize))
		if err != nil {
			return err
		}
		var proof event.GatewayProofResponse
		if err := json.Unmarshal(resp, &proof); err != nil {
			return err
		}
		sig, err := base64.StdEncoding.DecodeString(proof.Signature)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(GatewayProofBytes(nonce, c.LocalPeer().String(), c.RemotePeer().String()))
		return rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig)
	}, nil
}
