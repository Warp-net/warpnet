//nolint:all
package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

// Replying to or favouriting a post without following its author first is an
// ordinary Fediverse interaction, so a bridged actor reaches a node that has
// never stored them - only a follow used to store one. The authorship gate then
// has no node id to compare, and the interaction is dropped. These tests pin
// the first contact: the actor is resolved from the node that delivered the
// event, and the gate still refuses anyone that node does not host.
func TestFirstContactActor(t *testing.T) {
	const (
		owner       = "owner-1"
		parentUser  = "parent-user"
		bridged     = "someone@mastodon.social"
		tweetId     = "tweet-1"
		ownerNode   = warpnet.WarpPeerID("owner-node")
		gatewayNode = warpnet.WarpPeerID("gateway-node")
	)

	ownInfo := warpnet.NodeInfo{OwnerId: owner, ID: ownerNode}
	_, fromGateway := stream.NewLoopbackStream(ownerNode, gatewayNode, "/test/route/0.0.0")

	// What the gateway answers PUBLIC_GET_USER with: the bridged actor, homed
	// on the gateway itself.
	bridgedUser := func(nodeId string) domain.User {
		return domain.User{Id: bridged, Username: "someone", NodeId: nodeId, Network: "mastodon"}
	}

	// unknownActor knows every local user but has never heard of the bridged one.
	unknownActor := func(created *domain.User) stubReplyUserRepo {
		return stubReplyUserRepo{
			getFn: func(userId string) (domain.User, error) {
				if userId == bridged {
					return domain.User{}, database.ErrUserNotFound
				}
				return domain.User{Id: userId, NodeId: ownerNode.String()}, nil
			},
			createFn: func(u domain.User) (domain.User, error) {
				*created = u
				return u, nil
			},
		}
	}

	reply := func() event.NewTweetEvent {
		pu, pid := parentUser, tweetId
		return event.NewTweetEvent{
			CreatedAt:    time.Now(),
			Id:           "reply-1",
			ParentId:     &pid,
			ParentUserId: &pu,
			RootId:       tweetId,
			Text:         "a reply from mastodon",
			UserId:       bridged,
			Username:     "someone",
		}
	}

	t.Run("reply: the delivering node resolves an actor nobody here knows", func(t *testing.T) {
		var created domain.User
		var routes []stream.WarpRoute
		var askedNode string

		streamer := stubStreamer{
			nodeInfo: ownInfo,
			genericStreamFn: func(nodeId string, path stream.WarpRoute, _ any) ([]byte, error) {
				routes = append(routes, path)
				askedNode = nodeId
				return marshal(t, bridgedUser(gatewayNode.String())), nil
			},
		}
		h := StreamNewReplyHandler(stubTweetRepo{}, unknownActor(&created), stubModerationNotifier{}, streamer)

		resp, err := h(marshal(t, reply()), fromGateway)
		if err != nil {
			t.Fatalf("first-contact reply rejected: %v", err)
		}
		if resp.(domain.Tweet).Text != "a reply from mastodon" {
			t.Fatalf("expected the reply to be stored, got %+v", resp)
		}
		if len(routes) != 1 || routes[0] != event.PUBLIC_GET_USER {
			t.Fatalf("expected a single %s call, got %v", event.PUBLIC_GET_USER, routes)
		}
		if askedNode != gatewayNode.String() {
			t.Fatalf("expected the actor resolved from the delivering node %q, got %q", gatewayNode.String(), askedNode)
		}
		if created.Id != bridged || created.NodeId != gatewayNode.String() {
			t.Fatalf("expected the resolved actor to be stored, got %+v", created)
		}
	})

	t.Run("reply: an actor the delivering node does not host is still refused", func(t *testing.T) {
		var created domain.User
		streamer := stubStreamer{
			nodeInfo: ownInfo,
			genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
				return marshal(t, bridgedUser("somebody-elses-node")), nil
			},
		}
		h := StreamNewReplyHandler(stubTweetRepo{}, unknownActor(&created), stubModerationNotifier{}, streamer)

		if _, err := h(marshal(t, reply()), fromGateway); !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("expected ErrForeignAuthor, got %v", err)
		}
	})

	t.Run("reply: an actor nobody vouches for is still refused", func(t *testing.T) {
		var created domain.User
		streamer := stubStreamer{
			nodeInfo: ownInfo,
			genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
				return nil, errors.New("no such user")
			},
		}
		h := StreamNewReplyHandler(stubTweetRepo{}, unknownActor(&created), stubModerationNotifier{}, streamer)

		if _, err := h(marshal(t, reply()), fromGateway); !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("expected ErrForeignAuthor, got %v", err)
		}
	})

	// The favourite path: same first contact, and the resolved profile is what
	// gives the notification a name instead of a raw handle.
	unknownReactor := func(created *domain.User) stubReactionUserRepo {
		return stubReactionUserRepo{
			getFn: func(userId string) (domain.User, error) {
				if userId == bridged {
					return domain.User{}, database.ErrUserNotFound
				}
				return domain.User{Id: userId, NodeId: ownerNode.String()}, nil
			},
			createFn: func(u domain.User) (domain.User, error) {
				*created = u
				return u, nil
			},
		}
	}

	reaction := event.ReactionEvent{TweetId: tweetId, UserId: owner, OwnerId: bridged, Emoji: "❤️"}

	t.Run("reaction: the delivering node resolves an actor nobody here knows", func(t *testing.T) {
		var created domain.User
		var notification domain.Notification

		streamer := stubStreamer{
			nodeInfo: ownInfo,
			genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
				return marshal(t, bridgedUser(gatewayNode.String())), nil
			},
		}
		notifier := stubModerationNotifier{addFn: func(not domain.Notification) error {
			notification = not
			return nil
		}}
		h := StreamReactionHandler(stubReactionRepo{}, unknownReactor(&created), notifier, streamer)

		resp, err := h(marshal(t, reaction), fromGateway)
		if err != nil {
			t.Fatalf("first-contact reaction rejected: %v", err)
		}
		if resp.(event.ReactionsCountResponse).Count != 1 {
			t.Fatalf("expected the reaction to be counted, got %+v", resp)
		}
		if created.Id != bridged || created.NodeId != gatewayNode.String() {
			t.Fatalf("expected the resolved actor to be stored, got %+v", created)
		}
		if notification.ActorId != bridged || notification.Text != "someone reacted your tweet" {
			t.Fatalf("expected the resolved username in the notification, got %+v", notification)
		}
	})

	t.Run("reaction: an actor the delivering node does not host is still refused", func(t *testing.T) {
		var created domain.User
		streamer := stubStreamer{
			nodeInfo: ownInfo,
			genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
				return marshal(t, bridgedUser("somebody-elses-node")), nil
			},
		}
		h := StreamReactionHandler(stubReactionRepo{}, unknownReactor(&created), stubModerationNotifier{}, streamer)

		if _, err := h(marshal(t, reaction), fromGateway); !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("expected ErrForeignAuthor, got %v", err)
		}
	})
}
