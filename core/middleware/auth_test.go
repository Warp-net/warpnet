// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
)

func TestIsFresh(t *testing.T) {
	p := &WarpMiddleware{freshnessWindow: 5 * time.Minute}
	now := time.Now()

	cases := []struct {
		name string
		ts   time.Time
		want bool
	}{
		{"now", now, true},
		{"within past", now.Add(-2 * time.Minute), true},
		{"within future", now.Add(2 * time.Minute), true},
		{"stale", now.Add(-10 * time.Minute), false},
		{"future skew", now.Add(10 * time.Minute), false},
		{"zero", time.Time{}, false},
	}
	for _, c := range cases {
		if got := p.isFresh(c.ts); got != c.want {
			t.Errorf("%s: isFresh=%v want %v", c.name, got, c.want)
		}
	}
}

// A zero freshnessWindow must fall back to the package default.
func TestIsFresh_DefaultsWindow(t *testing.T) {
	p := &WarpMiddleware{}
	if !p.isFresh(time.Now()) {
		t.Fatal("expected now to be fresh with default window")
	}
	if p.isFresh(time.Now().Add(-messageFreshnessWindow - time.Minute)) {
		t.Fatal("expected stale message to fail with default window")
	}
}

// An over-limit payload used to deadlock: the middleware stopped reading at
// the cap and then blocked writing an error to a peer that was itself still
// blocked writing its payload. Both sides sat until their deadlines, so the
// caller saw nothing at all. The middleware must instead reset the stream,
// which is what unblocks the peer's write.
func TestAuthMiddleware_OversizedPayloadDoesNotDeadlock(t *testing.T) {
	mw := NewWarpMiddleware("peer1")
	defer mw.Close()

	var handlerCalled bool
	handler := mw.AuthMiddleware(func(s warpnet.WarpStream) {
		handlerCalled = true
		_, _ = s.Write([]byte(`{"ok":true}`))
	})

	client, server := stream.NewLoopbackStream(
		"peer1", warpnet.WarpProtocolID(event.PUBLIC_GET_USER),
	)
	go handler(server)

	// Substantially over the cap, so the middleware stops reading with a
	// large amount still outstanding — that is the shape that deadlocks. A
	// payload of exactly limit+1 is fully drained by the sentinel read and
	// would not reproduce it. Uses a default-limit route to keep the
	// allocation small while exercising identical logic.
	limit := RouteMaxLimit(event.PUBLIC_GET_USER)
	payload := bytes.Repeat([]byte("A"), int(limit)*3)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = client.SetDeadline(time.Now().Add(30 * time.Second))
		_, _ = client.Write(payload)
		_ = client.CloseWrite()
		_, _ = io.ReadAll(client)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("oversized payload deadlocked the caller")
	}

	if handlerCalled {
		t.Error("an over-limit payload must never reach the wrapped handler")
	}
}

// A payload that fits must still be processed normally, so the +1 sentinel
// read doesn't cost a legitimate upload at exactly the ceiling.
func TestAuthMiddleware_PayloadAtLimitIsNotRejectedForSize(t *testing.T) {
	mw := NewWarpMiddleware("peer1")
	defer mw.Close()

	limit := RouteMaxLimit(event.PRIVATE_POST_UPLOAD_VIDEO)
	client, server := stream.NewLoopbackStream(
		"peer1", warpnet.WarpProtocolID(event.PRIVATE_POST_UPLOAD_VIDEO),
	)
	go mw.AuthMiddleware(func(s warpnet.WarpStream) {})(server)

	// Exactly at the ceiling: rejected later for being invalid JSON, but it
	// must be read in full rather than tripping the size guard.
	payload := bytes.Repeat([]byte("A"), int(limit))

	done := make(chan struct{})
	var resp []byte
	go func() {
		defer close(done)
		_ = client.SetDeadline(time.Now().Add(30 * time.Second))
		_, _ = client.Write(payload)
		_ = client.CloseWrite()
		resp, _ = io.ReadAll(client)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("payload at the limit deadlocked")
	}

	if len(resp) == 0 {
		t.Error("a payload at the ceiling must still get a response")
	}
}

func TestRouteMaxLimit(t *testing.T) {
	if got := RouteMaxLimit(event.PRIVATE_POST_UPLOAD_VIDEO); got != int64(VideoMaxLimit) {
		t.Errorf("video route limit = %d, want %d", got, VideoMaxLimit)
	}
	if got := RouteMaxLimit(event.PRIVATE_POST_IMPORT_TWITTER_TWEET); got != int64(ImportTweetMaxLimit) {
		t.Errorf("import route limit = %d, want %d", got, ImportTweetMaxLimit)
	}
	if got := RouteMaxLimit(event.PUBLIC_GET_USER); got != int64(MaxLimit) {
		t.Errorf("default route limit = %d, want %d", got, MaxLimit)
	}
}

// The node's decoded-video ceiling has to stay reachable: base64 inflates by
// 4/3, so the envelope cap must exceed that or the handler's clear
// "video is too large" error is replaced by a stream reset.
func TestVideoLimitLeavesRoomForBase64(t *testing.T) {
	const maxDecodedVideo = 50 * 1024 * 1024 // handler.maxVideoSize
	base64Size := int64(maxDecodedVideo) * 4 / 3
	if int64(VideoMaxLimit) <= base64Size {
		t.Fatalf(
			"VideoMaxLimit %d must exceed the base64 size %d of a maximum video",
			VideoMaxLimit, base64Size,
		)
	}
}
