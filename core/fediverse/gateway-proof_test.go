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
	"io"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/require"
)

func seededHost(t *testing.T, seed string) (host.Host, ed25519.PrivateKey) {
	t.Helper()
	priv, err := security.GenerateKeyFromSeed([]byte(seed))
	require.NoError(t, err)
	key, err := p2pcrypto.UnmarshalEd25519PrivateKey(priv)
	require.NoError(t, err)
	h, err := libp2p.New(libp2p.Identity(key), libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), libp2p.DisableRelay())
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h, priv
}

func rsaKeyPEM(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// serveProof makes h answer the proof route like the gateway does, signing for
// clientOf(remote peer) with key.
func serveProof(h host.Host, key *rsa.PrivateKey, clientOf func(peer.ID) string) {
	h.SetStreamHandler(protocol.ID(event.PUBLIC_GET_GATEWAY_PROOF), func(s network.Stream) {
		defer func() { _ = s.Close() }()
		data, _ := io.ReadAll(s)
		var msg event.Message
		var ev event.GatewayProofEvent
		if json.Unmarshal(data, &msg) != nil || json.Unmarshal(msg.Body, &ev) != nil {
			return
		}
		nonce, _ := base64.StdEncoding.DecodeString(ev.Nonce)
		digest := sha256.Sum256(GatewayProofBytes(nonce, clientOf(s.Conn().RemotePeer()), h.ID().String()))
		sig, _ := rsa.SignPKCS1v15(nil, key, crypto.SHA256, digest[:])
		out, _ := json.Marshal(event.GatewayProofResponse{Signature: base64.StdEncoding.EncodeToString(sig)})
		_, _ = s.Write(out)
	})
}

func connect(t *testing.T, from, to host.Host) network.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, from.Connect(ctx, peer.AddrInfo{ID: to.ID(), Addrs: to.Addrs()}))
	conns := from.Network().ConnsToPeer(to.ID())
	require.NotEmpty(t, conns)
	return conns[0]
}

func TestGatewayProver(t *testing.T) {
	gwKey, gwPEM := rsaKeyPEM(t)
	otherKey, _ := rsaKeyPEM(t)
	member, memberPriv := seededHost(t, "member-"+t.Name())
	prover, err := NewGatewayProver(memberPriv, gwPEM)
	require.NoError(t, err)
	asIs := func(p peer.ID) string { return p.String() }

	prove := func(gw host.Host) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return prover(ctx, connect(t, member, gw))
	}

	t.Run("the real gateway proves itself", func(t *testing.T) {
		gw, _ := seededHost(t, "public-gateway-seed")
		serveProof(gw, gwKey, asIs)
		require.NoError(t, prove(gw))
	})

	// Same public seed, so the same peer id, but not the gateway's RSA key.
	t.Run("an impostor with the gateway's peer id fails", func(t *testing.T) {
		gw, _ := seededHost(t, "public-gateway-seed")
		serveProof(gw, otherKey, asIs)
		require.Error(t, prove(gw))
	})

	// A proof the real gateway made for another client must not be replayable.
	t.Run("a proof made for another node fails", func(t *testing.T) {
		gw, _ := seededHost(t, "public-gateway-seed")
		serveProof(gw, gwKey, func(peer.ID) string { return "12D3KooWSomeOtherNode" })
		require.Error(t, prove(gw))
	})

	t.Run("a peer without the proof route fails", func(t *testing.T) {
		gw, _ := seededHost(t, "public-gateway-seed")
		require.Error(t, prove(gw))
	})

	t.Run("a bad public key is refused", func(t *testing.T) {
		_, err := NewGatewayProver(memberPriv, "not a key")
		require.Error(t, err)
	})
}

// VerifyAuthorship must trust the gateway's peer id only over a connection
// that proved the gateway's RSA key, and a failed connection is closed.
func TestVerifyAuthorshipRequiresGatewayProof(t *testing.T) {
	gwKey, gwPEM := rsaKeyPEM(t)
	otherKey, _ := rsaKeyPEM(t)
	t.Cleanup(func() { warpnet.RequireConnProof("", nil) })

	authorship := func(t *testing.T, rsaKey *rsa.PrivateKey) (error, func() bool) {
		member, memberPriv := seededHost(t, "member-"+t.Name())
		gw, _ := seededHost(t, "public-gateway-seed")
		serveProof(gw, rsaKey, func(p peer.ID) string { return p.String() })
		prover, err := NewGatewayProver(memberPriv, gwPEM)
		require.NoError(t, err)
		warpnet.RequireConnProof(gw.ID(), prover)

		result := make(chan error, 1)
		member.SetStreamHandler("/test/push", func(s network.Stream) {
			result <- warpnet.VerifyAuthorship(s, gw.ID().String())
			_ = s.Close()
		})
		connect(t, gw, member)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s, err := gw.NewStream(ctx, member.ID(), "/test/push")
		require.NoError(t, err)
		_ = s.CloseWrite()
		var got error
		select {
		case got = <-result:
		case <-ctx.Done():
			t.Fatal("no authorship verdict")
		}
		_ = s.Close()
		return got, func() bool { return len(member.Network().ConnsToPeer(gw.ID())) > 0 }
	}

	t.Run("the real gateway is trusted", func(t *testing.T) {
		err, connected := authorship(t, gwKey)
		require.NoError(t, err)
		require.True(t, connected())
	})

	t.Run("an impostor is refused and disconnected", func(t *testing.T) {
		err, connected := authorship(t, otherKey)
		require.ErrorIs(t, err, warpnet.ErrForeignAuthor)
		require.Eventually(t, func() bool { return !connected() }, time.Second, 10*time.Millisecond)
	})

	// Requests the node sends to the gateway's peer id are refused too when the
	// connection reaches an impostor.
	t.Run("a request to an impostor is refused", func(t *testing.T) {
		member, memberPriv := seededHost(t, "member-"+t.Name())
		gw, _ := seededHost(t, "public-gateway-seed")
		serveProof(gw, otherKey, func(p peer.ID) string { return p.String() })
		gw.SetStreamHandler(protocol.ID(event.PUBLIC_GET_USER), func(s network.Stream) {
			_, _ = io.ReadAll(s)
			_, _ = s.Write([]byte(`{"id":"alice@mastodon.social"}`))
			_ = s.Close()
		})
		prover, err := NewGatewayProver(memberPriv, gwPEM)
		require.NoError(t, err)
		warpnet.RequireConnProof(gw.ID(), prover)
		pool, err := stream.NewStreamPool(context.Background(), member)
		require.NoError(t, err)
		member.Peerstore().AddAddrs(gw.ID(), gw.Addrs(), time.Minute)

		_, err = pool.Send(peer.AddrInfo{ID: gw.ID()}, stream.WarpRoute(event.PUBLIC_GET_USER), []byte(`{}`))
		require.ErrorIs(t, err, warpnet.ErrConnNotProven)
	})

	t.Run("the default key parses", func(t *testing.T) {
		_, err := parseRSAPublicKey(DefaultGatewayPublicKeyPEM)
		require.NoError(t, err)
	})
}

// Must stay byte-identical to the gateway's gatewayProofBytes.
func TestGatewayProofBytes(t *testing.T) {
	require.Equal(t, "warpnet-gateway-proof\x00\x01\x02\x00client\x00gateway",
		string(GatewayProofBytes([]byte{1, 2}, "client", "gateway")))
}
