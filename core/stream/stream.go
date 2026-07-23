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
	"hash/fnv"
	"io"
	"strconv"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/retrier"
	"github.com/Warp-net/warpnet/security"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/oklog/ulid/v2"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

const (
	sendTimeout    = 10 * time.Second
	retryBudget    = 10 * time.Second
	maxSendRetries = 5

	// offlineCacheTTL is how long a peer stays short-circuited as offline
	// before Send probes it again (half-open backstop when no reconnect
	// event arrives). offlineCacheSize bounds the number of tracked peers.
	offlineCacheTTL  = 30 * time.Second
	offlineCacheSize = 1024
)

const ErrResponseRead = warpnet.WarpError("stream: response read failed after request delivered")

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
	sf           singleflight.Group
	retrier      retrier.Retrier
	offline      *expirable.LRU[string, struct{}]
}

func NewStreamPool(
	ctx context.Context,
	n NodeStreamer,
) (*streamPool, error) {
	privKey, err := n.Network().Peerstore().PrivKey(n.ID()).Raw()
	if err != nil {
		return nil, err
	}

	return &streamPool{
		ctx:     ctx,
		n:       n,
		privKey: privKey,
		retrier: retrier.New(time.Second, maxSendRetries, retrier.FixedBackoff),
		offline: expirable.NewLRU[string, struct{}](offlineCacheSize, nil, offlineCacheTTL),
	}, nil
}

func (p *streamPool) Send(peerAddr warpnet.WarpAddrInfo, r WarpRoute, data []byte) ([]byte, error) {
	if p == nil {
		return nil, warpnet.WarpError("nil stream pool")
	}
	if p.ctx.Err() != nil {
		return nil, p.ctx.Err()
	}

	// Short-circuit peers already known to be offline: skip the dial and its
	// sendTimeout wait until the peer is seen online again (a successful send
	// or a reconnect via SetOnline) or the cached mark expires.
	if p.isOffline(peerAddr.ID) {
		return nil, warpnet.ErrNodeIsOffline
	}

	key := string(r) + "\x00" + peerAddr.ID.String() + "\x00" + hashBody(data)
	v, err, _ := p.sf.Do(key, func() (any, error) {
		return p.sendWithRetry(peerAddr, r, data)
	})
	switch {
	case errors.Is(err, warpnet.ErrNodeIsOffline):
		p.markOffline(peerAddr.ID)
	case err == nil, errors.Is(err, ErrResponseRead):
		// ErrResponseRead means the request reached the peer, so it is online.
		p.SetOnline(peerAddr.ID)
	}
	if err != nil {
		return nil, err
	}
	bt, _ := v.([]byte)
	return bt, nil
}

func (p *streamPool) markOffline(id warpnet.WarpPeerID) {
	if p == nil || p.offline == nil {
		return
	}
	p.offline.Add(id.String(), struct{}{})
}

// SetOnline clears any cached offline mark so the next Send dials the peer
// again instead of short-circuiting. Called when the peer is seen to reconnect.
func (p *streamPool) SetOnline(id warpnet.WarpPeerID) {
	if p == nil || p.offline == nil {
		return
	}
	p.offline.Remove(id.String())
}

func (p *streamPool) isOffline(id warpnet.WarpPeerID) bool {
	if p == nil || p.offline == nil {
		return false
	}
	return p.offline.Contains(id.String())
}

func (p *streamPool) sendWithRetry(serverInfo warpnet.WarpAddrInfo, r WarpRoute, bodyBytes []byte) ([]byte, error) {
	msgID := ulid.Make().String()

	bt, err := p.send(serverInfo, r, bodyBytes, msgID)
	if err == nil || errors.Is(err, warpnet.ErrNodeIsOffline) || errors.Is(err, ErrResponseRead) {
		return bt, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), retryBudget)
	defer cancel()
	_ = p.retrier.Try(ctx, func() error {
		bt, err = p.send(serverInfo, r, bodyBytes, msgID)
		if errors.Is(err, warpnet.ErrNodeIsOffline) || errors.Is(err, ErrResponseRead) {
			return fmt.Errorf("%w: %w", err, retrier.ErrStopTrying)
		}
		return err
	})
	return bt, err
}

func hashBody(data []byte) string {
	h := fnv.New64a()
	_, _ = h.Write(data)
	return strconv.FormatUint(h.Sum64(), 16)
}

func (p *streamPool) send(
	serverInfo warpnet.WarpAddrInfo, r WarpRoute, bodyBytes []byte, msgID string,
) ([]byte, error) {
	if p.n == nil || serverInfo.String() == "" || r == "" {
		return nil, warpnet.WarpError("stream: parameters improperly configured")
	}

	if len(serverInfo.ID) > 52 {
		return nil, fmt.Errorf(
			"stream: %w: node id is too long: %s", warpnet.ErrMalformedNodeId, serverInfo.ID.String(),
		)
	}

	if err := serverInfo.ID.Validate(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	if netw := p.n.Network(); netw != nil && netw.Connectedness(serverInfo.ID) == network.Limited {
		log.Debugf("stream: peer %s has limited connection", serverInfo.ID.String())
		ctx = network.WithAllowLimitedConn(ctx, warpnet.WarpnetName)
	}

	stream, err := p.n.NewStream(ctx, serverInfo.ID, r.ProtocolID())
	// No known addresses (routing.ErrNotFound), every dial failed
	// (swarm.ErrAllDialsFailed), or the dial hanging past sendTimeout
	// (context.DeadlineExceeded) all mean the peer is unreachable — offline.
	if warpnet.IsNoAddressesError(err) || errors.Is(err, warpnet.ErrAllDialsFailed) ||
		errors.Is(err, context.DeadlineExceeded) {
		return nil, warpnet.ErrNodeIsOffline
	}
	if err != nil {
		log.Debugf("stream: new: failed to create stream: %v", err)
		return nil, fmt.Errorf("stream: new: %w", err)
	}
	defer closeStream(stream)

	if msgID == "" {
		msgID = ulid.Make().String()
	}
	body := json.RawMessage(bodyBytes)
	msg := event.Message{
		Body:        body,
		MessageId:   msgID,
		NodeId:      p.n.ID().String(),
		Destination: r.String(),
		Timestamp:   time.Now().UTC(),
		Version:     "0.0.0", // TODO event message version
	}
	msg.Signature = security.Sign(p.privKey, msg.SigningBytes())
	data, _ := json.Marshal(msg)

	var rw = bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	if data != nil {
		log.Debugf("stream: sent to %s data with size %d\n", r, len(data))
		_, err = rw.Write(data)
	}
	flush(rw)
	closeWrite(stream)
	if err != nil {
		log.Errorf("stream: writing: %v", err)
		return nil, fmt.Errorf("stream: writing: %w", err)
	}

	buf := bytes.NewBuffer(nil)
	_, err = buf.ReadFrom(rw)
	if err != nil && !errors.Is(err, io.EOF) {
		log.Debugf("stream: reading response from %s: %v", serverInfo.ID.String(), err)
		return nil, fmt.Errorf("%w: from %s: %w", ErrResponseRead, serverInfo.ID.String(), err)
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
