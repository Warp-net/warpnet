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

//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type stubPairAuth struct{ token string }

func (s *stubPairAuth) SessionToken() string { return s.token }

type stubDeviceRepo struct {
	saved       int
	setDeviceFn func(ownerNodeId string, device domain.Device) error
}

func (s *stubDeviceRepo) SetDevice(ownerNodeId string, device domain.Device) error {
	if s.setDeviceFn == nil {
		s.saved++
		return nil
	}
	if err := s.setDeviceFn(ownerNodeId, device); err != nil {
		return err
	}
	s.saved++
	return nil
}

type stubNodeAddresser struct {
	addrs []warpnet.WarpAddress
}

func (s stubNodeAddresser) PublicAddrs() []warpnet.WarpAddress { return s.addrs }

// stubPairConn embeds network.Conn so only the methods the handler calls
// (LocalPeer/RemotePeer) need to be implemented.
type stubPairConn struct {
	network.Conn
	localPeerID  peer.ID
	remotePeerID peer.ID
}

func (c stubPairConn) LocalPeer() peer.ID  { return c.localPeerID }
func (c stubPairConn) RemotePeer() peer.ID { return c.remotePeerID }

// stubPairStream embeds network.Stream so only Conn() needs implementing.
type stubPairStream struct {
	network.Stream
	conn stubPairConn
}

func (s stubPairStream) Conn() network.Conn { return s.conn }

func TestStreamNodesPairingHandler(t *testing.T) {
	const serverToken = "server-secret-token"

	localPeer, _ := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	remotePeer, _ := peer.Decode("QmcEPrat8ShnCph8WjkREzt5CPXF2RwhYxYBALDcLC1iV6")

	stream := stubPairStream{conn: stubPairConn{
		localPeerID:  localPeer,
		remotePeerID: remotePeer,
	}}

	t.Run("invalid payload", func(t *testing.T) {
		auth := &stubPairAuth{token: serverToken}
		h := StreamNodesPairingHandler(auth, &stubDeviceRepo{}, stubNodeAddresser{})
		_, err := h([]byte("{"), stream)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty token", func(t *testing.T) {
		auth := &stubPairAuth{token: serverToken}
		h := StreamNodesPairingHandler(auth, &stubDeviceRepo{}, stubNodeAddresser{})
		_, err := h(marshal(t, domain.AuthNodeInfo{Token: ""}), stream)
		if err == nil || err.Error() != "empty token" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("token mismatch", func(t *testing.T) {
		auth := &stubPairAuth{token: serverToken}
		h := StreamNodesPairingHandler(auth, &stubDeviceRepo{}, stubNodeAddresser{})
		_, err := h(marshal(t, domain.AuthNodeInfo{Token: "wrong-token"}), stream)
		if err == nil || err.Error() != "token mismatch" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("device repo error", func(t *testing.T) {
		repoErr := errors.New("db down")
		auth := &stubPairAuth{token: serverToken}
		h := StreamNodesPairingHandler(auth, &stubDeviceRepo{
			setDeviceFn: func(ownerNodeId string, device domain.Device) error {
				return repoErr
			},
		}, stubNodeAddresser{})
		_, err := h(marshal(t, domain.AuthNodeInfo{Token: serverToken}), stream)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got: %v", err)
		}
	})

	t.Run("successful pairing", func(t *testing.T) {
		addr, _ := ma.NewMultiaddr("/ip4/1.2.3.4/tcp/4001")
		var capturedOwner string
		var capturedDevice domain.Device
		repo := &stubDeviceRepo{
			setDeviceFn: func(ownerNodeId string, device domain.Device) error {
				capturedOwner = ownerNodeId
				capturedDevice = device
				return nil
			},
		}
		auth := &stubPairAuth{token: serverToken}
		h := StreamNodesPairingHandler(auth, repo, stubNodeAddresser{addrs: []warpnet.WarpAddress{addr}})

		resp, err := h(marshal(t, domain.AuthNodeInfo{Token: serverToken}), stream)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedOwner != localPeer.String() {
			t.Fatalf("expected owner %q, got %q", localPeer.String(), capturedOwner)
		}
		if capturedDevice.NodeId != remotePeer {
			t.Fatalf("expected device node id %q, got %q", remotePeer, capturedDevice.NodeId)
		}
		if capturedDevice.Token != serverToken {
			t.Fatalf("expected device token %q, got %q", serverToken, capturedDevice.Token)
		}
		addrs, ok := resp.([]string)
		if !ok {
			t.Fatalf("expected []string response, got %T", resp)
		}
		if len(addrs) != 1 || addrs[0] != addr.String() {
			t.Fatalf("expected [%s], got %v", addr.String(), addrs)
		}
	})
}

func TestStreamNodesPairingHandler_ReadsLiveToken(t *testing.T) {
	auth := &stubPairAuth{token: "tokenA"}
	devices := &stubDeviceRepo{}
	h := StreamNodesPairingHandler(auth, devices, stubNodeAddresser{})

	peerID, _ := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	stream := stubPairStream{conn: stubPairConn{localPeerID: peerID, remotePeerID: peerID}}
	body := marshal(t, domain.AuthNodeInfo{Token: "tokenB"})

	if _, err := h(body, stream); err == nil {
		t.Fatal("expected token mismatch while session token is tokenA")
	}

	auth.token = "tokenB"
	if _, err := h(body, stream); err != nil {
		t.Fatalf("expected pairing to succeed after the session token rotated to tokenB, got: %v", err)
	}
	if devices.saved != 1 {
		t.Fatalf("expected device saved once, got %d", devices.saved)
	}
}
