package stream

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/stretchr/testify/assert"
)

// fakeNewStreamNode is a NodeStreamer whose NewStream always fails with a
// configured error. Only NewStream is exercised by streamPool.send.
type fakeNewStreamNode struct{ err error }

func (f *fakeNewStreamNode) NewStream(_ context.Context, _ warpnet.WarpPeerID, _ ...warpnet.WarpProtocolID) (warpnet.WarpStream, error) {
	return nil, f.err
}
func (f *fakeNewStreamNode) Network() network.Network { return nil }
func (f *fakeNewStreamNode) ID() warpnet.WarpPeerID   { return "" }

// An offline peer whose addresses are still cached fails NewStream with
// swarm.ErrAllDialsFailed (not routing.ErrNotFound). send must report that as
// ErrNodeIsOffline so the offline-marking callers fire.
func TestSend_UnreachablePeerReportedOffline(t *testing.T) {
	pid := warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")
	serverInfo := warpnet.WarpAddrInfo{ID: pid}

	cases := []struct {
		name      string
		streamErr error
		offline   bool
	}{
		{"all dials failed", warpnet.ErrAllDialsFailed, true},
		{"wrapped all dials failed", fmt.Errorf("dial: %w", warpnet.ErrAllDialsFailed), true},
		{"unrelated error", errors.New("boom"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &streamPool{ctx: context.Background(), n: &fakeNewStreamNode{err: tc.streamErr}}
			_, err := p.send(serverInfo, WarpRoute("/test/route"), []byte("{}"), "")
			assert.Equal(t, tc.offline, errors.Is(err, warpnet.ErrNodeIsOffline))
		})
	}
}
