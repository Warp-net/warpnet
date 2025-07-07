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

package stream

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	"github.com/libp2p/go-libp2p/core/network"
	log "github.com/sirupsen/logrus"
	"io"
	"time"
)

type NodeStreamer interface {
	NewStream(ctx context.Context, p warpnet.WarpPeerID, pids ...warpnet.WarpProtocolID) (warpnet.WarpStream, error)
	Network() network.Network
	ID() warpnet.WarpPeerID
}

type streamPool struct {
	ctx          context.Context
	n            NodeStreamer
	privKey      ed25519.PrivateKey
	clientPeerID warpnet.WarpPeerID
}

func NewStreamPool(
	ctx context.Context,
	n NodeStreamer,
) (*streamPool, error) {
	privKey, err := n.Network().Peerstore().PrivKey(n.ID()).Raw()
	if err != nil {
		return nil, err
	}

	return &streamPool{ctx: ctx, n: n, privKey: privKey}, nil
}

func (p *streamPool) Send(peerAddr warpnet.WarpAddrInfo, r WarpRoute, data []byte) ([]byte, error) {
	if p == nil {
		return nil, warpnet.WarpError("nil stream pool")
	}
	if p.ctx.Err() != nil {
		return nil, p.ctx.Err()
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
	return p.send(ctx, peerAddr, r, data)
}

func (p *streamPool) send(
	ctx context.Context, serverInfo warpnet.WarpAddrInfo, r WarpRoute, bodyBytes []byte,
) ([]byte, error) {
	if p.n == nil || serverInfo.String() == "" || r == "" {
		return nil, warpnet.WarpError("stream: parameters improperly configured")
	}

	if len(serverInfo.ID) > 52 {
		return nil, fmt.Errorf("stream: node id is too long: %v", serverInfo.ID)
	}

	if err := serverInfo.ID.Validate(); err != nil {
		return nil, err
	}

	stream, err := p.n.NewStream(ctx, serverInfo.ID, r.ProtocolID())
	if err != nil {
		log.Debugf("stream: new: failed to create stream: %v", err)
		if errors.Is(err, warpnet.ErrAllDialsFailed) {
			err = warpnet.ErrAllDialsFailed
		}
		return nil, fmt.Errorf("stream: new: %v", err)
	}
	defer closeStream(stream)

	body := jsoniter.RawMessage(bodyBytes)
	msg := event.Message{
		Body:        &body,
		MessageId:   uuid.New().String(),
		NodeId:      p.n.ID().String(),
		Destination: r.String(),
		Timestamp:   time.Now(),
		Version:     "0.0.0",
		Signature:   security.Sign(p.privKey, body),
	}
	data, _ := json.JSON.Marshal(msg)

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
	_, err = buf.ReadFrom(rw)
	if err != nil && !errors.Is(err, io.EOF) {
		log.Debugf("stream: reading response from %s: %v", serverInfo.ID.String(), err)
		return nil, fmt.Errorf("stream: reading response from %s: %w", serverInfo.ID.String(), err)
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
