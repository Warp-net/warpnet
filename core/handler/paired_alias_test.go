//nolint:all
package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

func newAliasPeer(t *testing.T) warpnet.WarpPeerID {
	t.Helper()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	id, err := warpnet.IDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("derive node id: %v", err)
	}
	return id
}

// pairedDeviceStream mirrors what a paired mobile client produces: it dials
// this node, so the connection's remote peer is the device while the acting
// user's stored NodeId is this node itself.
func pairedDeviceStream(
	t *testing.T, route warpnet.WarpProtocolID, ownNodeId, device warpnet.WarpPeerID, paired bool,
) warpnet.WarpStream {
	t.Helper()

	_, server := stream.NewLoopbackStream(ownNodeId, device, route)
	return &warpnet.WarpStreamBody{
		WarpStream:  server,
		MessageId:   "message-1",
		PairedAlias: paired,
	}
}

func TestPairedDeviceAuthorship(t *testing.T) {
	var (
		ownNodeId = newAliasPeer(t)
		device    = newAliasPeer(t)
		owner     = "owner-1"
		author    = "author-1"
		tweetId   = "tweet-1"
	)

	users := stubReactionUserRepo{getFn: func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: ownNodeId.String(), Username: "owner"}, nil
	}}

	t.Run("view from a paired device is accepted", func(t *testing.T) {
		s := pairedDeviceStream(t, event.PUBLIC_POST_VIEW, ownNodeId, device, true)
		h := StreamViewHandler(
			stubViewRepo{recordFn: func(tweetId, viewerId string) (uint64, error) { return 7, nil }},
			users,
			stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}},
		)

		resp, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: owner}), s)
		if err != nil {
			t.Fatalf("paired device view rejected: %v", err)
		}
		if resp.(event.ViewsCountResponse).Count != 7 {
			t.Fatalf("unexpected response: %v", resp)
		}
	})

	t.Run("view from an unpaired peer is still rejected", func(t *testing.T) {
		s := pairedDeviceStream(t, event.PUBLIC_POST_VIEW, ownNodeId, device, false)
		h := StreamViewHandler(stubViewRepo{}, users, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: author},
		})

		_, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: owner}), s)
		if !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("expected %v, got %v", warpnet.ErrForeignAuthor, err)
		}
	})

	t.Run("a paired device may not author for a foreign node", func(t *testing.T) {
		foreignUsers := stubReactionUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{Id: userId, NodeId: newAliasPeer(t).String()}, nil
		}}
		s := pairedDeviceStream(t, event.PUBLIC_POST_VIEW, ownNodeId, device, true)
		h := StreamViewHandler(stubViewRepo{}, foreignUsers, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: author},
		})

		_, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: owner}), s)
		if !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("expected %v, got %v", warpnet.ErrForeignAuthor, err)
		}
	})

	t.Run("reaction from a paired device is accepted", func(t *testing.T) {
		s := pairedDeviceStream(t, event.PUBLIC_POST_REACT, ownNodeId, device, true)
		h := StreamReactionHandler(
			stubReactionRepo{},
			users,
			stubModerationNotifier{},
			stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}},
		)

		if _, err := h(marshal(t, event.ReactionEvent{
			TweetId: tweetId, UserId: author, OwnerId: owner, Emoji: "👍",
		}), s); err != nil {
			t.Fatalf("paired device reaction rejected: %v", err)
		}
	})
}
