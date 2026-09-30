// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"os"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/assert"
)

// brokenStream is a stream whose request never arrives whole: reading it
// fails the way a reset connection or a silent peer does.
type brokenStream struct {
	warpnet.WarpStream

	cause error
}

func (s brokenStream) Read([]byte) (int, error)    { return 0, s.cause }
func (s brokenStream) Write(p []byte) (int, error) { return len(p), nil }

// A request cut short by the network is not a malformed frame: the peer
// sent nothing wrong, and charging it 120 a time floors a peer on a bad
// link within a handful of requests.
func TestUnwrapDoesNotBlameAPeerForTheNetwork(t *testing.T) {
	for name, cause := range map[string]error{
		"read deadline":     os.ErrDeadlineExceeded,
		"connection closed": os.ErrClosed,
	} {
		t.Run(name, func(t *testing.T) {
			n := watchingNode()
			_, server := stream.NewLoopbackStream(watcher, watched, testProto)

			n.unwrap(func([]byte, warpnet.WarpStream) (any, error) {
				return event.Accepted, nil
			})(brokenStream{WarpStream: server, cause: cause})

			select {
			case ev := <-n.Event():
				assert.Failf(t, "the peer was charged for the network", "%s: %s", name, ev.Type)
			case <-time.After(200 * time.Millisecond):
			}
		})
	}
}
