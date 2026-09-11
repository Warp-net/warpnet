// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	watcher  = warpnet.WarpPeerID("peer-local")
	watched  = warpnet.WarpPeerID("peer-remote")
	testSize = 4096
)

// watchingNode is a node that reports what it sees, with no libp2p under it.
func watchingNode() *WarpNode {
	return &WarpNode{events: warpnet.NewPeerEmitter()}
}

// reportAbout runs one request through unwrap between two different peers
// and returns what the node reported about the remote one.
func reportAbout(
	t *testing.T, n *WarpNode, handler warpnet.WarpHandlerFunc, request []byte,
) []warpnet.PeerEvent {
	t.Helper()

	client, server := stream.NewLoopbackStream(watcher, watched, testProto)
	go n.unwrap(handler)(server)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = client.SetDeadline(time.Now().Add(20 * time.Second))
		_, _ = client.Write(request)
		_ = client.CloseWrite()
		_, _ = io.ReadAll(client)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("unwrap deadlocked")
	}

	var out []warpnet.PeerEvent
	for {
		select {
		case ev := <-n.Event():
			out = append(out, ev)
		case <-time.After(time.Second):
			return out
		}
	}
}

func TestUnwrapReportsAnOversizePayload(t *testing.T) {
	n := watchingNode()

	events := reportAbout(t, n, func([]byte, warpnet.WarpStream) (any, error) {
		return event.Accepted, nil
	}, bytes.Repeat([]byte("A"), int(stream.MaxControlSize)+testSize))

	require.Len(t, events, 1)
	assert.Equal(t, warpnet.PeerOversizePayload, events[0].Type)
	assert.Equal(t, watched.String(), events[0].PeerID, "the peer that sent it is the one charged")
	assert.Equal(t, string(testProto), events[0].Route)
}

func TestUnwrapReportsAnEventFromTheWrongAuthor(t *testing.T) {
	n := watchingNode()

	events := reportAbout(t, n, func([]byte, warpnet.WarpStream) (any, error) {
		return nil, warpnet.ErrForeignAuthor
	}, []byte(`{}`))

	require.Len(t, events, 1)
	assert.Equal(t, warpnet.PeerForeignAuthorship, events[0].Type)
	assert.Equal(t, watched.String(), events[0].PeerID)
}

func TestUnwrapReportsNothingForAnOrdinaryHandlerError(t *testing.T) {
	n := watchingNode()

	events := reportAbout(t, n, func([]byte, warpnet.WarpStream) (any, error) {
		return nil, errors.New("the handler simply failed")
	}, []byte(`{}`))

	assert.Empty(t, events, "a handler that fails says nothing about the peer that called it")
}

func TestUnwrapReportsNothingAboutASelfStream(t *testing.T) {
	n := watchingNode()

	client, server := stream.NewLoopbackStream(watcher, watcher, testProto)
	go n.unwrap(func([]byte, warpnet.WarpStream) (any, error) {
		return nil, warpnet.ErrForeignAuthor
	})(server)

	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = client.Write([]byte(`{}`))
	_ = client.CloseWrite()
	_, _ = io.ReadAll(client)

	select {
	case ev := <-n.Event():
		t.Fatalf("a node reported itself: %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestEmitterlessNodeIsSafe(t *testing.T) {
	n := &WarpNode{} // a zero node has nobody to report to
	client, server := stream.NewLoopbackStream(watcher, watched, testProto)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	assert.NotPanics(t, func() {
		n.emitStream(server, warpnet.PeerOversizePayload)
	})
}
