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

package stream

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p/core/network"
	log "github.com/sirupsen/logrus"
	"time"
)

type MastodonPseudoStreamer interface {
	ID() warpnet.WarpPeerID
	IsMastodonID(id warpnet.WarpPeerID) bool
	Addrs() []warpnet.WarpAddress
	Route(r WarpRoute, data []byte) (_ []byte, err error)
}

type NodeStreamer interface {
	NewStream(ctx context.Context, p warpnet.WarpPeerID, pids ...warpnet.WarpProtocolID) (warpnet.WarpStream, error)
	Network() network.Network
}

type streamPool struct {
	ctx                context.Context
	n                  NodeStreamer
	clientPeerID       warpnet.WarpPeerID
	mastodonPseudoNode MastodonPseudoStreamer
}

func NewStreamPool(
	ctx context.Context,
	n NodeStreamer,
	mastodonPseudoNode MastodonPseudoStreamer,
) *streamPool {
	pool := &streamPool{ctx: ctx, n: n, mastodonPseudoNode: mastodonPseudoNode}

	return pool
}

func (p *streamPool) Send(peerAddr warpnet.WarpAddrInfo, r WarpRoute, data []byte) ([]byte, error) {
	if p == nil {
		return nil, warpnet.WarpError("nil stream pool")
	}
	if p.ctx.Err() != nil {
		return nil, p.ctx.Err()
	}

	if p.mastodonPseudoNode != nil && p.mastodonPseudoNode.IsMastodonID(peerAddr.ID) {
		return p.mastodonPseudoNode.Route(r, data)
	}
	// long-long wait in case of p2p-circuit stream
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	connectedness := p.n.Network().Connectedness(peerAddr.ID)
	switch connectedness {
	case network.Limited:
		log.Debugf("stream: peer %s has limited connection", peerAddr.ID.String())
		ctx = network.WithAllowLimitedConn(ctx, warpnet.WarpnetName)
	default:
	}
	return send(ctx, p.n, peerAddr, r, data)
}

func send(
	ctx context.Context, n NodeStreamer,
	serverInfo warpnet.WarpAddrInfo, r WarpRoute, data []byte,
) ([]byte, error) {
	if n == nil || serverInfo.String() == "" || r == "" {
		return nil, warpnet.WarpError("stream: parameters improperly configured")
	}

	if len(serverInfo.ID) > 52 {
		return nil, fmt.Errorf("stream: node id is too long: %v", serverInfo.ID)
	}

	if err := serverInfo.ID.Validate(); err != nil {
		return nil, err
	}

	stream, err := n.NewStream(ctx, serverInfo.ID, r.ProtocolID())
	if err != nil {
		log.Debugf("stream: new: failed to create stream: %v", err)
		if errors.Is(err, warpnet.ErrAllDialsFailed) {
			err = warpnet.ErrAllDialsFailed
		}
		return nil, fmt.Errorf("stream: new: %v", err)
	}
	defer closeStream(stream)

	var rw = bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	if data != nil {
		log.Debugf("stream: sent to %s data with size %d\n", r, len(data))
		_, err = rw.Write(data)
	}
	flush(rw)
	closeWrite(stream)
	if err != nil {
		log.Errorf("stream: writing: %v", err)
		return nil, fmt.Errorf("stream: writing: %s", err)
	}

	buf := bytes.NewBuffer(nil)
	num, err := buf.ReadFrom(rw)
	if err != nil {
		log.Debugf("stream: reading response from %s: %v", serverInfo.ID.String(), err)
		return nil, fmt.Errorf("stream: reading response from %s: %w", serverInfo.ID.String(), err)
	}

	if num == 0 {
		return nil, fmt.Errorf(
			"stream: protocol %s, peer ID %s, addresses %v: empty response",
			r.ProtocolID(), serverInfo.ID.String(), serverInfo.Addrs,
		)
	}
	return buf.Bytes(), nil
}

func closeStream(stream warpnet.WarpStream) {
	if err := stream.Close(); err != nil {
		log.Errorf("stream: closing: %s", err)
	}
}

func flush(rw *bufio.ReadWriter) {
	if err := rw.Flush(); err != nil {
		log.Errorf("stream: flush: %s", err)
	}
}

func closeWrite(s warpnet.WarpStream) {
	if err := s.CloseWrite(); err != nil {
		log.Errorf("stream: close write: %s", err)
	}
}
