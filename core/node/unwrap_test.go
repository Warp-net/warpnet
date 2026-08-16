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
package node

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProto = warpnet.WarpProtocolID("/public/post/tweet/0.0.0")

func unwrapHandler(handler warpnet.WarpHandlerFunc) warpnet.StreamHandler {
	return (&WarpNode{}).unwrap(handler)
}

// idempotentChain mirrors the production composition: the idempotency
// middleware wrapping the node's unwrap adapter.
func idempotentChain(t *testing.T, handler warpnet.WarpHandlerFunc) warpnet.StreamHandler {
	t.Helper()
	mw := middleware.NewWarpMiddleware("peer1", nil)
	t.Cleanup(mw.Close)
	return mw.IdempotencyMiddleware(unwrapHandler(handler))
}

func roundTrip(t *testing.T, handler warpnet.WarpHandlerFunc, request []byte) []byte {
	t.Helper()

	client, server := stream.NewLoopbackStream("peer1", "peer1", testProto)
	go unwrapHandler(handler)(server)

	var resp []byte
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = client.SetDeadline(time.Now().Add(20 * time.Second))
		_, _ = client.Write(request)
		_ = client.CloseWrite()
		resp, _ = io.ReadAll(client)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("unwrap deadlocked")
	}
	return resp
}

func TestUnwrap_StructResponseIsJSONEncoded(t *testing.T) {
	resp := roundTrip(t, func(msg []byte, s warpnet.WarpStream) (any, error) {
		return event.Accepted, nil
	}, []byte(`{"hello":"world"}`))

	assert.NotEmpty(t, resp)
	var decoded any
	assert.NoError(t, json.Unmarshal(resp, &decoded), "clients parse the response as JSON")
}

func TestUnwrap_ByteAndStringResponsesGoOutVerbatim(t *testing.T) {
	raw := roundTrip(t, func([]byte, warpnet.WarpStream) (any, error) {
		return []byte(`{"raw":true}`), nil
	}, []byte(`{}`))
	assert.Equal(t, `{"raw":true}`, string(raw), "a []byte answer must not be re-encoded")

	str := roundTrip(t, func([]byte, warpnet.WarpStream) (any, error) {
		return `{"str":true}`, nil
	}, []byte(`{}`))
	assert.Equal(t, `{"str":true}`, string(str))
}

func TestUnwrap_HandlerSeesExactRequestBytes(t *testing.T) {
	payload := `{"text":"кириллица 🔥 <script>","n":42}`
	var seen string
	roundTrip(t, func(msg []byte, s warpnet.WarpStream) (any, error) {
		seen = string(msg)
		return event.Accepted, nil
	}, []byte(payload))

	assert.Equal(t, payload, seen)
}

func TestUnwrap_NilResponseBecomesAnErrorEnvelope(t *testing.T) {
	resp := roundTrip(t, func([]byte, warpnet.WarpStream) (any, error) {
		return nil, nil
	}, []byte(`{}`))

	var out event.ResponseError
	require.NoError(t, json.Unmarshal(resp, &out))
	assert.Equal(t, middleware.EmptyResponseMessage, out.Message)
}

func TestUnwrap_HandlerErrorBecomesResponseError(t *testing.T) {
	resp := roundTrip(t, func([]byte, warpnet.WarpStream) (any, error) {
		return nil, errors.New("handler exploded")
	}, []byte(`{}`))

	var out event.ResponseError
	require.NoError(t, json.Unmarshal(resp, &out))
	assert.Equal(t, middleware.InternalNodeErrorCode, out.Code)
	assert.Contains(t, out.Message, "handler exploded")
}

func TestUnwrap_OfflineErrorKeepsHandlerResponse(t *testing.T) {
	resp := roundTrip(t, func([]byte, warpnet.WarpStream) (any, error) {
		return []byte(`{"degraded":true}`), warpnet.ErrNodeIsOffline
	}, []byte(`{}`))

	assert.Equal(t, `{"degraded":true}`, string(resp),
		"an offline peer must not be rewritten into an internal error")
}

func TestUnwrap_UnencodableResponseWritesNothing(t *testing.T) {
	resp := roundTrip(t, func([]byte, warpnet.WarpStream) (any, error) {
		return make(chan int), nil
	}, []byte(`{}`))

	assert.Empty(t, resp)
}

func bodyStream(t *testing.T, proto warpnet.WarpProtocolID, body []byte, messageID string) (*warpnet.WarpStreamBody, warpnet.WarpStream) {
	t.Helper()
	client, server := stream.NewLoopbackStream("peer1", "peer1", proto)
	return &warpnet.WarpStreamBody{
		WarpStream: server,
		Body:       body,
		MessageId:  messageID,
	}, client
}

func drain(t *testing.T, client warpnet.WarpStream) []byte {
	t.Helper()
	_ = client.SetDeadline(time.Now().Add(20 * time.Second))
	out, _ := io.ReadAll(client)
	return out
}

func TestUnwrap_BodyStreamUsesPreReadPayload(t *testing.T) {
	body, client := bodyStream(t, testProto, []byte(`{"pre":"read"}`), "")

	var seen string
	go unwrapHandler(func(msg []byte, s warpnet.WarpStream) (any, error) {
		seen = string(msg)
		return []byte(`ok`), nil
	})(body)

	assert.Equal(t, "ok", string(drain(t, client)))
	assert.Equal(t, `{"pre":"read"}`, seen, "a pre-read body must not be re-read from the wire")
}

func TestIdempotency_ReplayRunsHandlerOnce(t *testing.T) {
	var calls int64
	chain := idempotentChain(t, func([]byte, warpnet.WarpStream) (any, error) {
		atomic.AddInt64(&calls, 1)
		return []byte(`{"created":true}`), nil
	})

	for i := 0; i < 3; i++ {
		body, client := bodyStream(t, testProto, []byte(`{"text":"hi"}`), "same-message-id")
		go chain(body)
		assert.Equal(t, `{"created":true}`, string(drain(t, client)), "replay %d", i)
	}

	assert.Equal(t, int64(1), atomic.LoadInt64(&calls),
		"a replayed message id must be served from cache")
}

func TestIdempotency_AcceptedResponseIsCached(t *testing.T) {
	var calls int64
	chain := idempotentChain(t, func([]byte, warpnet.WarpStream) (any, error) {
		atomic.AddInt64(&calls, 1)
		return event.Accepted, nil
	})

	for i := 0; i < 2; i++ {
		body, client := bodyStream(t, testProto, []byte(`{}`), "accepted-id")
		go chain(body)
		assert.Equal(t, event.Accepted, string(drain(t, client)), "replay %d", i)
	}

	assert.Equal(t, int64(1), atomic.LoadInt64(&calls),
		"an Accepted ack is a successful reply and must be replayed from cache")
}

func TestIdempotency_FailedResponseIsNotCached(t *testing.T) {
	var calls int64
	chain := idempotentChain(t, func([]byte, warpnet.WarpStream) (any, error) {
		atomic.AddInt64(&calls, 1)
		return nil, errors.New("transient")
	})

	for i := 0; i < 2; i++ {
		body, client := bodyStream(t, testProto, []byte(`{}`), "retryable-id")
		go chain(body)
		drain(t, client)
	}

	assert.Equal(t, int64(2), atomic.LoadInt64(&calls),
		"a transient failure must not be pinned for the whole TTL")
}

func TestIdempotency_ConcurrentReplaysShareOneHandlerRun(t *testing.T) {
	var calls int64
	release := make(chan struct{})
	chain := idempotentChain(t, func([]byte, warpnet.WarpStream) (any, error) {
		atomic.AddInt64(&calls, 1)
		<-release
		return []byte(`{"ok":true}`), nil
	})

	const n = 4
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		body, client := bodyStream(t, testProto, []byte(`{}`), "concurrent-id")
		go chain(body)
		go func() {
			defer wg.Done()
			drain(t, client)
		}()
	}

	time.Sleep(200 * time.Millisecond)
	close(release)
	wg.Wait()

	assert.Equal(t, int64(1), atomic.LoadInt64(&calls),
		"in-flight duplicates must share one handler invocation")
}

func TestIdempotency_NonIdempotentProtocolAlwaysRunsHandler(t *testing.T) {
	var calls int64
	chain := idempotentChain(t, func([]byte, warpnet.WarpStream) (any, error) {
		atomic.AddInt64(&calls, 1)
		return []byte(`{}`), nil
	})

	proto := warpnet.WarpProtocolID("/public/get/user/0.0.0")
	for i := 0; i < 2; i++ {
		body, client := bodyStream(t, proto, []byte(`{}`), "an-id")
		go chain(body)
		drain(t, client)
	}

	assert.Equal(t, int64(2), atomic.LoadInt64(&calls),
		"reads are not deduplicated — they must always see current state")
}
