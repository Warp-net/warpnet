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
	"bytes"
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

func streamRequest(
	t *testing.T, sh warpnet.StreamHandler, proto warpnet.WarpProtocolID, request []byte,
) []byte {
	t.Helper()

	client, server := stream.NewLoopbackStream("peer1", "peer1", proto)
	go sh(server)

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
		t.Error("unwrap deadlocked")
	}
	return resp
}

func roundTrip(t *testing.T, handler warpnet.WarpHandlerFunc, request []byte) []byte {
	t.Helper()
	return streamRequest(t, unwrapHandler(handler), testProto, request)
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
	assert.Equal(t, "empty response", out.Message)
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

func TestUnwrap_PanickingHandlerClosesTheStream(t *testing.T) {
	resp := roundTrip(t, func([]byte, warpnet.WarpStream) (any, error) {
		panic("handler exploded")
	}, []byte(`{}`))

	assert.Empty(t, resp, "a panic must not leak a half-written response")
}

func TestUnwrap_OversizedPayloadDoesNotDeadlock(t *testing.T) {
	var handlerCalled atomic.Bool
	sh := unwrapHandler(func([]byte, warpnet.WarpStream) (any, error) {
		handlerCalled.Store(true)
		return event.Accepted, nil
	})

	payload := bytes.Repeat([]byte("A"), int(stream.MaxControlSize)+4096)
	streamRequest(t, sh, testProto, payload)

	assert.False(t, handlerCalled.Load(), "an over-limit payload must never reach the handler")
}

func TestUnwrap_PayloadAtLimitIsNotRejectedForSize(t *testing.T) {
	sh := unwrapHandler(func([]byte, warpnet.WarpStream) (any, error) {
		return event.Accepted, nil
	})

	payload := bytes.Repeat([]byte("A"), int(stream.MaxControlSize))
	resp := streamRequest(t, sh, testProto, payload)

	assert.NotEmpty(t, resp, "a payload at the ceiling must still get a response")
}

func injectBody(messageID string) StreamMiddleware {
	return func(next warpnet.WarpHandlerFunc) warpnet.WarpHandlerFunc {
		return func(data []byte, s warpnet.WarpStream) (any, error) {
			return next(data, &warpnet.WarpStreamBody{
				WarpStream: s,
				MessageId:  messageID,
			})
		}
	}
}

func idempotentChain(t *testing.T, messageID string, handler warpnet.WarpHandlerFunc) warpnet.StreamHandler {
	t.Helper()
	mw := middleware.NewWarpMiddleware("peer1", nil, nil)
	t.Cleanup(mw.Close)
	return unwrapHandler(injectBody(messageID)(mw.IdempotencyMiddleware(handler)))
}

func TestIdempotency_ReplayRunsHandlerOnce(t *testing.T) {
	var calls int64
	chain := idempotentChain(t, "same-message-id", func([]byte, warpnet.WarpStream) (any, error) {
		atomic.AddInt64(&calls, 1)
		return []byte(`{"created":true}`), nil
	})

	for i := 0; i < 3; i++ {
		resp := streamRequest(t, chain, testProto, []byte(`{"text":"hi"}`))
		assert.Equal(t, `{"created":true}`, string(resp), "replay %d", i)
	}

	assert.Equal(t, int64(1), atomic.LoadInt64(&calls),
		"a replayed message id must be served from cache")
}

func TestIdempotency_AcceptedResponseIsCached(t *testing.T) {
	var calls int64
	chain := idempotentChain(t, "accepted-id", func([]byte, warpnet.WarpStream) (any, error) {
		atomic.AddInt64(&calls, 1)
		return event.Accepted, nil
	})

	for i := 0; i < 2; i++ {
		resp := streamRequest(t, chain, testProto, []byte(`{}`))
		assert.Equal(t, event.Accepted, string(resp), "replay %d", i)
	}

	assert.Equal(t, int64(1), atomic.LoadInt64(&calls),
		"an Accepted ack is a successful reply and must be replayed from cache")
}

func TestIdempotency_FailedResponseIsNotCached(t *testing.T) {
	var calls int64
	chain := idempotentChain(t, "retryable-id", func([]byte, warpnet.WarpStream) (any, error) {
		atomic.AddInt64(&calls, 1)
		return nil, errors.New("transient")
	})

	for i := 0; i < 2; i++ {
		streamRequest(t, chain, testProto, []byte(`{}`))
	}

	assert.Equal(t, int64(2), atomic.LoadInt64(&calls),
		"a transient failure must not be pinned for the whole TTL")
}

func TestIdempotency_ConcurrentReplaysShareOneHandlerRun(t *testing.T) {
	var calls int64
	release := make(chan struct{})
	chain := idempotentChain(t, "concurrent-id", func([]byte, warpnet.WarpStream) (any, error) {
		atomic.AddInt64(&calls, 1)
		<-release
		return []byte(`{"ok":true}`), nil
	})

	const n = 4
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			streamRequest(t, chain, testProto, []byte(`{}`))
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
	chain := idempotentChain(t, "an-id", func([]byte, warpnet.WarpStream) (any, error) {
		atomic.AddInt64(&calls, 1)
		return []byte(`{}`), nil
	})

	proto := warpnet.WarpProtocolID("/public/get/user/0.0.0")
	for i := 0; i < 2; i++ {
		streamRequest(t, chain, proto, []byte(`{}`))
	}

	assert.Equal(t, int64(2), atomic.LoadInt64(&calls),
		"reads are not deduplicated — they must always see current state")
}
