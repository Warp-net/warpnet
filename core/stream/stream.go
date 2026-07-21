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
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/oklog/ulid/v2"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

const (
	sendTimeout    = 10 * time.Second
	retryBudget    = 10 * time.Second
	maxSendRetries = 5
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
	}, nil
}

func (p *streamPool) Send(peerAddr warpnet.WarpAddrInfo, r WarpRoute, data []byte) ([]byte, error) {
	return p.SendWithID(peerAddr, r, data, "")
}

// SendWithID behaves like Send but pins the envelope message id instead of
// minting a fresh one. Redelivery from the offline outbox reuses the original
// id so the receiver's idempotency layer dedupes the replay.
func (p *streamPool) SendWithID(peerAddr warpnet.WarpAddrInfo, r WarpRoute, data []byte, msgID string) ([]byte, error) {
	if p == nil {
		return nil, warpnet.WarpError("nil stream pool")
	}
	if p.ctx.Err() != nil {
		return nil, p.ctx.Err()
	}

	key := string(r) + "\x00" + peerAddr.ID.String() + "\x00" + hashBody(data)
	v, err, _ := p.sf.Do(key, func() (any, error) {
		return p.sendWithRetry(peerAddr, r, data, msgID)
	})
	if err != nil {
		return nil, err
	}
	bt, _ := v.([]byte)
	return bt, nil
}

func (p *streamPool) sendWithRetry(serverInfo warpnet.WarpAddrInfo, r WarpRoute, bodyBytes []byte, msgID string) ([]byte, error) {
	if msgID == "" {
		msgID = ulid.Make().String()
	}

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
	// No known addresses (routing.ErrNotFound) or every dial failed
	// (swarm.ErrAllDialsFailed) both mean the peer is unreachable — offline.
	if warpnet.IsNoAddressesError(err) || errors.Is(err, warpnet.ErrAllDialsFailed) {
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
