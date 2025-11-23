package moderator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/moderator/isolation"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	ErrNoTweetsForModeration warpnet.WarpError = "no tweets for moderation found"
	ErrModeratorInitFailed   warpnet.WarpError = "failed to init moderator engine"
)

type Engine interface {
	Moderate(content string) (bool, string, error)
	Close()
}

// build constrained
var (
	engine          Engine
	engineReadyChan = make(chan struct{}, 1)
)

type ModeratorNode interface {
	Start() error
	Stop()
	Node() warpnet.P2PNode
	ID() warpnet.WarpPeerID
	ClosestPeers() []warpnet.WarpPeerID
	NodeInfo() warpnet.NodeInfo
	GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error)
}

type Publisher interface {
	PublishUpdateToFollowers(ownerId, dest string, body any) (err error)
}

type Moderator struct {
	ctx        context.Context
	node       ModeratorNode
	tweetCache *tweetModerationCache
	isolation  *isolation.IsolationProtocol

	isClosed *atomic.Bool
}

func NewModerator(
	ctx context.Context,
	node ModeratorNode,
	pub Publisher,
) (_ *Moderator, err error) {
	mn := &Moderator{
		ctx:        ctx,
		tweetCache: newTweetModerationCache(),
		node:       node,
		isolation:  isolation.NewIsolationProtocol(node, pub),
		isClosed:   new(atomic.Bool),
	}

	return mn, nil
}

func (m *Moderator) Start() (err error) {
	if m == nil {
		panic("moderator: nil")
	}

	log.Infoln("moderator: wait engine init...")

	engineReadyChan <- struct{}{}
	// wait until moderator set up
	<-engineReadyChan
	if engine == nil {
		return ErrModeratorInitFailed
	}
	log.Infoln("moderator: engine is running")

	go m.runTweetsModeration()
	go m.lurkUserDescriptions()
	log.Infoln("moderator: started")

	return nil
}

func (m *Moderator) Close() {
	m.isClosed.Store(true)

	if engine != nil {
		engine.Close()
	}
}

func (m *Moderator) runTweetsModeration() {
	if m == nil || m.node == nil {
		log.Fatalf("moderator: nil node")
	}
	if engine == nil {
		log.Fatalf("moderator: nil moderator")
	}

	peerStore := m.node.Node().Peerstore()

	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for range ticker.C {
		if m.isClosed.Load() {
			return
		}
		peers := peerStore.PeersWithAddrs()
		if len(peers) == 0 {
			log.Warn("moderator: peers are not found")
			continue
		}
		for _, peer := range peers {
			if m.isClosed.Load() {
				return
			}
			if peer == m.node.ID() {
				continue
			}

			infoResp, err := m.node.GenericStream(peer.String(), event.PUBLIC_GET_INFO, nil)
			if err != nil {
				if strings.Contains(err.Error(), "protocols not supported") {
					continue
				}
				log.Errorf("moderator: no info response from new peer %s, %v", peer.String(), err)
				continue
			}
			if len(infoResp) == 0 {
				log.Errorf("moderator: no info response from new peer %s", peer.String())
				continue
			}

			var info warpnet.NodeInfo
			err = json.Unmarshal(infoResp, &info)
			if err != nil {
				log.Errorf("moderator: failed to unmarshal info from new peer: %s %v", infoResp, err)
				continue
			}
			if info.IsModerator() || info.IsBootstrap() {
				continue
			}
			if info.OwnerId == "" {
				log.Errorf("moderator: node info %s has no owner", peer.String())
				continue
			}
			log.Infof("moderator: checking peer: %s, owner: %s", peer.String(), info.OwnerId)

			tweet, err := m.pickTweet(peer, info.OwnerId)
			if errors.Is(err, ErrNoTweetsForModeration) {
				continue
			}
			if err != nil {
				log.Errorf("moderator: moderation engine failure %s: %v", peer.String(), err)
				continue
			}

			moderationResult, err := m.moderateTweet(tweet)
			if err != nil {
				log.Errorf("moderator: moderation engine failure %s: %v", peer.String(), err)
				continue
			}
			log.Infoln("moderator: isolate tweet protocol started")
			m.isolation.IsolateTweet(peer, tweet, moderationResult)
			log.Infoln("moderator: isolate tweet protocol finished")
		}
	}
}

const tweetsLimit uint64 = 20

func (m *Moderator) pickTweet(peerID warpnet.WarpPeerID, userID string) (*domain.Tweet, error) {
	var cursor *string

	for {
		data, err := m.node.GenericStream(
			peerID.String(),
			event.PUBLIC_GET_TWEETS,
			event.GetAllTweetsEvent{
				Limit:  func(l uint64) *uint64 { return &l }(tweetsLimit),
				UserId: userID,
				Cursor: cursor,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("moderator: get tweets: %w", err)
		}
		if len(data) == 0 {
			return nil, ErrNoTweetsForModeration
		}

		var tweetsResp event.TweetsResponse
		if err := json.Unmarshal(data, &tweetsResp); err != nil {
			return nil, fmt.Errorf("moderator: failed to unmarshal tweets from new peer: %s %w", string(data), err)
		}

		if len(tweetsResp.Tweets) == 0 {
			return nil, ErrNoTweetsForModeration
		}

		log.Infof("moderator: peers %s, got tweets: %d", peerID.String(), len(tweetsResp.Tweets))

		cursor = &tweetsResp.Cursor

		key := CacheKey{
			Type:   domain.ModerationTweetType,
			PeerId: peerID.String(),
			Cursor: cursor,
		}

		if m.tweetCache.IsModeratedAlready(key) {
			continue
		}

		m.tweetCache.SetAsModerated(key)

		for i := range tweetsResp.Tweets {
			tweet := tweetsResp.Tweets[i]

			if tweet.IsModerated() {
				continue
			}
			if tweet.Text == "" {
				continue
			}
			return &tweet, nil
		}
		if tweetsResp.Cursor == event.EndCursor || tweetsResp.Cursor == "" {
			return nil, ErrNoTweetsForModeration
		}
	}
}

func (m *Moderator) moderateTweet(tweet *domain.Tweet) (*domain.TweetModeration, error) {
	if tweet == nil {
		return nil, ErrNoTweetsForModeration
	}
	result, reason, err := engine.Moderate(tweet.Text)
	if err != nil {
		return nil, err
	}

	if !result {
		tweet.Text = ""
	}

	moderatorId := m.node.ID().String()
	return &domain.TweetModeration{
		ModeratorID: moderatorId,
		Model:       domain.LLAMA2,
		IsOk:        domain.ModerationResult(result) != domain.FAIL,
		Reason:      &reason,
		TimeAt:      time.Now(),
	}, nil
}
func (m *Moderator) lurkUserDescriptions() {
	// TODO
}

// TODO
func (m *Moderator) moderateUser(peerID warpnet.WarpPeerID, userID string) func() error {
	return func() error {
		bt, err := m.node.GenericStream(
			peerID.String(),
			event.PUBLIC_GET_USER,
			event.GetUserEvent{UserId: userID},
		)
		if err != nil {
			return err
		}

		var user domain.User
		if err := json.Unmarshal(bt, &user); err != nil {
			return err
		}

		text := fmt.Sprintf("%s: %s", user.Username, user.Bio)
		result, reason, err := engine.Moderate(text)
		if err != nil {
			return err
		}

		fmt.Printf("%s: %t %s\n", user.Username, result, reason)
		// TODO
		return nil
	}
}
