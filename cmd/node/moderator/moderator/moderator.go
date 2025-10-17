package moderator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/moderator/isolation"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
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

type DistributedStorer interface {
	GetStream(ctx context.Context, id string) (io.ReadCloser, error)
	PutStream(ctx context.Context, reader io.ReadCloser) (id string, _ error)
	Close() error
}

type ModeratorNode interface {
	Start() error
	Stop()
	Node() warpnet.P2PNode
	ID() warpnet.WarpPeerID
	ClosestPeers() ([]warpnet.WarpPeerID, error)
	NodeInfo() warpnet.NodeInfo
	GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error)
}

type Publisher interface {
	PublishUpdateToFollowers(ownerId, dest string, bt []byte) (err error)
}

type Moderator struct {
	ctx context.Context

	node      ModeratorNode
	store     DistributedStorer
	cache     *moderationCache
	isolation *isolation.IsolationProtocol

	isClosed *atomic.Bool
}

func NewModerator(
	ctx context.Context,
	node ModeratorNode,
	store DistributedStorer,
	pub Publisher,
) (_ *Moderator, err error) {

	mn := &Moderator{
		ctx:       ctx,
		cache:     newModerationCache(),
		node:      node,
		store:     store,
		isolation: isolation.NewIsolationProtocol(node, pub),
		isClosed:  new(atomic.Bool),
	}

	return mn, nil
}

func (m *Moderator) Start() (err error) {
	if m == nil {
		panic("moderator: nil")
	}

	confModelPath := config.Config().Node.Moderator.Path
	cid := config.Config().Node.Moderator.CID

	modelFile, isModelExists := isModelInPath(confModelPath)

	log.Infof("moderator: LLM model path: %s, CID: %s", confModelPath, cid)

	if !isModelExists {
		log.Infof("moderator: LLM model not found, downloading from IPFS")
		if err = fetchModel(confModelPath, cid, m.store); err != nil {
			return err
		}
	}

	log.Infoln("moderator: wait engine init...")

	engineReadyChan <- struct{}{}
	// wait until moderator set up
	<-engineReadyChan
	if engine == nil {
		return errors.New("failed to init moderator engine")
	}
	log.Infoln("moderator: engine is running")

	if isModelExists {
		log.Infof("moderator: LLM model found, uploading to IPFS")
		go storeModel(modelFile, m.store)
	}

	go m.lurkTweets()
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

func (m *Moderator) lurkTweets() {
	if m == nil || m.node == nil {
		log.Fatalf("moderator: nil node")
	}
	if engine == nil {
		log.Fatalf("moderator: nil moderator")
	}
	if m.cache == nil {
		log.Fatalf("moderator: nil cache")
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if m.isClosed.Load() {
			return
		}
		peers, err := m.node.ClosestPeers()
		if err != nil {
			log.Errorf("moderator: failed to get closest peers: %v", err)
			continue
		}
		if len(peers) == 0 {
			log.Warnf("moderator: no peers found")
			continue
		}
		for _, peer := range peers {
			if m.isClosed.Load() {
				return
			}
			if m.cache.IsModeratedAlready(peer) {
				continue
			}

			infoResp, err := m.node.GenericStream(peer.String(), event.PUBLIC_GET_INFO, nil)
			if err != nil {
				if strings.Contains(err.Error(), "protocols not supported") {
					continue
				}
				log.Errorf("moderator: get info: %v", err)
				continue
			}
			if infoResp == nil || len(infoResp) == 0 {
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
				m.cache.SetAsModerated(peer, CacheEntry{})
				continue
			}

			log.Infof("moderator: checking peer: %s, owner: %s", peer.String(), info.OwnerId)

			if info.OwnerId == "" {
				log.Errorf("moderator: node info %s has no owner", peer.String())
				continue
			}

			moderatedTweet, err := m.moderateRandomUserTweet(peer, info.OwnerId)
			if err != nil {
				log.Errorf("moderator: moderation engine failure %s: %v", peer.String(), err)
				continue
			}
			log.Infoln("moderator: isolate tweet protocol started")
			m.isolation.IsolateTweet(peer, moderatedTweet)

			m.cache.SetAsModerated(peer, CacheEntry{})
			log.Infoln("moderator: set as moderated", m.cache.IsModeratedAlready(peer))

		}
	}
}

func (m *Moderator) moderateRandomUserTweet(peerID warpnet.WarpPeerID, userID string) (tweet domain.Tweet, err error) {
	limit := uint64(20)

	tweetsResp, err := m.node.GenericStream(
		peerID.String(),
		event.PUBLIC_GET_TWEETS,
		event.GetAllTweetsEvent{
			Limit:  &limit,
			UserId: userID,
		},
	)
	if err != nil {
		return tweet, fmt.Errorf("moderator: get tweets: %v", err)
	}

	var tweetsEvent event.TweetsResponse
	if err := json.Unmarshal(tweetsResp, &tweetsEvent); err != nil {
		return tweet, fmt.Errorf("moderator: failed to unmarshal tweets from new peer: %s %v", tweetsResp, err)
	}
	if len(tweetsEvent.Tweets) == 0 {
		return tweet, nil
	}

	randomTweet := tweetsEvent.Tweets[rand.Intn(len(tweetsEvent.Tweets))]
	if randomTweet.Moderation != nil && randomTweet.Moderation.IsOk {
		return randomTweet, nil
	}
	if randomTweet.Text == "" {
		return randomTweet, nil
	}

	result, reason, err := engine.Moderate(randomTweet.Text)
	if err != nil {
		return randomTweet, err
	}

	if !result {
		randomTweet.Text = ""
	}

	randomTweet.Moderation = &domain.TweetModeration{
		IsModerated: true,
		ModeratorID: m.node.ID().String(),
		Model:       "llama2",
		IsOk:        domain.ModerationResult(result) != domain.FAIL,
		Reason:      &reason,
		TimeAt:      time.Now(),
	}

	return randomTweet, nil
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

func isModelInPath(path string) (*os.File, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	return f, true
}

func storeModel(f *os.File, store DistributedStorer) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour*8)
	defer cancel()

	cid, err := store.PutStream(ctx, f)
	if err != nil {
		log.Errorf("failed to put file in IPFS: %v", err)
		_ = f.Close()
		return
	}
	log.Infof("moderator: LLM model uploaded: CID: %s", cid)
	_ = f.Close()
	return
}

func fetchModel(path, cid string, store DistributedStorer) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour*8)
	defer cancel()

	reader, err := store.GetStream(ctx, cid)
	if err != nil {
		return fmt.Errorf("failed to get stream in IPFS: %v", err)
	}
	defer reader.Close()

	log.Infof("moderator: LLM model downloaded: CID: %s", cid)

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating file: %v", err)
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("writing to file: %v", err)
	}

	finalPath := strings.TrimSuffix(path, ".tmp")
	if err = os.Rename(path, finalPath); err != nil {
		log.Errorf("renaming file: %v", err)
	}

	return nil
}
