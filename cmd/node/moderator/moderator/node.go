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

package moderator

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Masterminds/semver/v3"
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/dht"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database/ipfs"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	log "github.com/sirupsen/logrus"
)

// build constrained
var (
	moderator          Moderator
	moderatorReadyChan = make(chan struct{}, 1)
)

type ModeratorNode struct {
	ctx context.Context

	node    *node.WarpNode
	options []libp2p.Option

	store DistributedStorer

	dHashTable DistributedHashTableCloser

	memoryStoreCloseF func() error

	cache *moderationCache

	version     *semver.Version
	psk         security.PSK
	privKey     ed25519.PrivateKey
	selfHashHex string

	isClosed *atomic.Bool
}

func NewModeratorNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	selfHashHex string,
) (_ *ModeratorNode, err error) {
	memoryStore, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, fmt.Errorf("moderator: fail creating memory peerstore: %w", err)
	}
	mapStore := datastore.NewMapDatastore()

	closeF := func() error {
		_ = memoryStore.Close()
		return mapStore.Close()
	}

	infos, err := config.Config().Node.AddrInfos()
	if err != nil {
		return nil, err
	}

	dHashTable := dht.NewDHTable(
		ctx,
		dht.RoutingStore(mapStore),
		dht.BootstrapNodes(infos...),
	)

	currentNodeID, err := warpnet.IDFromPublicKey(privKey.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, err
	}

	p2pPrivKey, err := p2pCrypto.UnmarshalEd25519PrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	mn := &ModeratorNode{
		ctx:               ctx,
		dHashTable:        dHashTable,
		cache:             newModerationCache(),
		memoryStoreCloseF: closeF,
		psk:               psk,
		privKey:           privKey,
		selfHashHex:       selfHashHex,
		version:           config.Config().Version,
		options: []libp2p.Option{
			libp2p.ListenAddrStrings(
				[]string{
					fmt.Sprintf("/ip6/%s/tcp/%s", config.Config().Node.HostV6, config.Config().Node.Port),
					fmt.Sprintf("/ip4/%s/tcp/%s", config.Config().Node.HostV4, config.Config().Node.Port),
				}...,
			),
			libp2p.Transport(warpnet.NewTCPTransport),
			libp2p.Identity(p2pPrivKey),
			libp2p.Ping(false),
			libp2p.Security(warpnet.NoiseID, warpnet.NewNoise),
			libp2p.Peerstore(memoryStore),
			libp2p.PrivateNetwork(warpnet.PSK(psk)),
			libp2p.UserAgent(warpnet.WarpnetName + "-moderator"),
			libp2p.Routing(dHashTable.StartRouting),
			node.EnableAutoRelayWithStaticRelays(infos, currentNodeID)(),
		},
		isClosed: new(atomic.Bool),
	}

	return mn, nil
}

func (mn *ModeratorNode) Start() (err error) {
	if mn == nil {
		panic("moderator: nil node")
	}
	var (
		confModelPath = config.Config().Node.Moderator.Path
		cid           = config.Config().Node.Moderator.CID
	)
	modelFile, isModelExists := isModelInPath(confModelPath)

	mn.node, err = node.NewWarpNode(mn.ctx, mn.options...)
	if err != nil {
		return fmt.Errorf("node: failed to init node: %v", err)
	}

	mn.node.SetStreamHandlers(
		warpnet.WarpStreamHandler{
			event.PUBLIC_GET_INFO,
			handler.StreamGetInfoHandler(mn, nil),
		},
		warpnet.WarpStreamHandler{
			event.PUBLIC_POST_NODE_CHALLENGE,
			handler.StreamChallengeHandler(root.GetCodeBase(), mn.privKey),
		},
	)

	mn.store, err = ipfs.NewIPFS(mn.ctx, mn.node.Node())
	if err != nil {
		return fmt.Errorf("failed to init moderator IPFS node: %v", err)
	}

	if !isModelExists {
		log.Infof("moderator: LLM model not found, downloading from IPFS")
		if err = fetchModel(confModelPath, cid, mn.store); err != nil {
			return err
		}
	}

	moderatorReadyChan <- struct{}{}
	// wait until moderator set up
	<-moderatorReadyChan
	if moderator == nil {
		return errors.New("failed to init moderator engine")
	}

	if isModelExists {
		log.Infof("moderator: LLM model found, uploading to IPFS")
		go storeModel(modelFile, mn.store)
	}

	nodeInfo := mn.NodeInfo()

	go mn.lurkTweets()
	go mn.lurkUserDescriptions()

	println()
	fmt.Printf(
		"\033[1mMODERATOR NODE STARTED WITH ID %s AND ADDRESSES %v\033[0m\n",
		nodeInfo.ID.String(), nodeInfo.Addresses,
	)
	println()
	return nil
}

func (mn *ModeratorNode) lurkTweets() {
	if mn == nil {
		log.Fatalf("moderator: nil node")
	}
	if moderator == nil {
		log.Fatalf("moderator: nil moderator")
	}
	if mn.cache == nil {
		log.Fatalf("moderator: nil cache")
	}
	if mn.dHashTable == nil {
		log.Fatalf("moderator: nil DHT")
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if mn.isClosed.Load() {
			return
		}
		peers, err := mn.dHashTable.ClosestPeers()
		if err != nil {
			log.Errorf("moderator: failed to get closest peers: %v", err)
			continue
		}
		for _, peer := range peers {
			if mn.isClosed.Load() {
				return
			}
			if ok := mn.cache.IsModeratedAlready(peer); ok {
				continue
			}

			infoResp, err := mn.GenericStream(peer.String(), event.PUBLIC_GET_INFO, nil)
			if err != nil {
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
				continue
			}
			if info.OwnerId == "" {
				log.Errorf("moderator: node info %s has no owner", peer.String())
				continue
			}

			result := event.ModerationResultEvent{
				Type:   event.Tweet,
				Result: event.OK,
				NodeID: peer.String(),
				UserID: info.OwnerId,
			}

			err = mn.moderateTweet(peer, info.OwnerId)
			if err != nil && !errors.As(err, &errModerationFailure{}) {
				log.Errorf("moderator: moderation engine failure %s: %v", peer.String(), err)
				continue

			}
			if err != nil {
				var failure *errModerationFailure
				errors.As(err, &failure)

				result.ObjectID = failure.ObjectID
				result.Reason = &failure.Reason
				result.Result = event.FAIL
			}

			item := CacheEntry{Result: result}

			_, err = mn.GenericStream(
				peer.String(),
				event.PUBLIC_POST_MODERATION_RESULT,
				result,
			)
			if err != nil {
				log.Errorf("moderator: post moderation result: %v", err)
				continue
			}
			mn.cache.SetAsModerated(peer, item)
		}
	}
}

func (mn *ModeratorNode) lurkUserDescriptions() {

	// TODO
}

type errModerationFailure struct {
	UserID   string
	ObjectID *string
	Reason   string
}

func (e errModerationFailure) Error() string {
	return fmt.Sprintf("%+v", e)
}

// TODO
func (mn *ModeratorNode) moderateUser(peerID warpnet.WarpPeerID, userID string) func() error {
	return func() error {
		bt, err := mn.GenericStream(
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
		result, reason, err := moderator.Moderate(text)
		if err != nil {
			return err
		}

		if event.ModerationResult(result) == event.FAIL {
			return errModerationFailure{
				UserID:   userID,
				ObjectID: nil,
				Reason:   reason,
			}
		}

		return nil
	}
}

func (mn *ModeratorNode) moderateTweet(peerID warpnet.WarpPeerID, userID string) error {
	limit := uint64(20)

	tweetsResp, err := mn.GenericStream(
		peerID.String(),
		event.PUBLIC_GET_TWEETS,
		event.GetAllTweetsEvent{
			Limit:  &limit,
			UserId: userID,
		},
	)
	if err != nil {
		return fmt.Errorf("moderator: get tweets: %v", err)
	}

	var tweetsEvent event.TweetsResponse
	if err := json.Unmarshal(tweetsResp, &tweetsEvent); err != nil {
		return fmt.Errorf("moderator: failed to unmarshal tweets from new peer: %s %v", tweetsResp, err)
	}
	if len(tweetsEvent.Tweets) == 0 {
		return nil
	}

	randomTweet := tweetsEvent.Tweets[rand.Intn(len(tweetsEvent.Tweets))]
	if randomTweet.Moderation != nil && randomTweet.Moderation.IsOk {
		return nil
	}
	if randomTweet.Text == "" {
		return nil
	}

	result, reason, err := moderator.Moderate(randomTweet.Text)
	if err != nil {
		return err
	}

	if event.ModerationResult(result) == event.FAIL {
		return errModerationFailure{
			UserID:   userID,
			ObjectID: &randomTweet.Id,
			Reason:   reason,
		}
	}

	return nil
}

func (mn *ModeratorNode) NodeInfo() warpnet.NodeInfo {
	baseInfo := mn.node.BaseNodeInfo()
	baseInfo.OwnerId = warpnet.ModeratorOwner
	return baseInfo
}

func (mn *ModeratorNode) GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error) {
	nodeId := warpnet.FromStringToPeerID(nodeIdStr)
	return mn.node.Stream(nodeId, path, data)
}

func (mn *ModeratorNode) Stop() {
	defer func() { recover() }()
	if mn == nil {
		return
	}
	mn.isClosed.Store(true)

	if mn.dHashTable != nil {
		mn.dHashTable.Close()
	}

	if mn.memoryStoreCloseF != nil {
		if err := mn.memoryStoreCloseF(); err != nil {
			log.Errorf("moderator: failed to close memory store: %v", err)
		}
	}
	if mn.store != nil {
		_ = mn.store.Close()
	}

	mn.node.StopNode()
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
