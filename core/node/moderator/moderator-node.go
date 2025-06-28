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
	"github.com/Masterminds/semver/v3"
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/consensus"
	dht "github.com/Warp-net/warpnet/core/dht"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/node/base"
	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database/ipfs"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"strings"
	"time"
)

// build constrained
var (
	moderator Moderator
	readyChan = make(chan struct{}, 1)
)

type ModeratorNode struct {
	ctx context.Context

	node    *base.WarpNode
	options []libp2p.Option

	streamer Streamer
	store    DistributedStorer

	pubsubService PubSubProvider
	dHashTable    DistributedHashTableCloser

	consensusService  ConsensusServicer
	memoryStoreCloseF func() error

	version     *semver.Version
	psk         security.PSK
	privKey     ed25519.PrivateKey
	selfHashHex string
}

func NewModeratorNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	selfHashHex string,
	interruptChan chan os.Signal,
) (_ *ModeratorNode, err error) {
	pubsubService := pubsub.NewPubSubModerator(ctx)
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
		pubsubService:     pubsubService,
		dHashTable:        dHashTable,
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
			base.EnableAutoRelayWithStaticRelays(infos, currentNodeID)(),
		},
	}

	mn.consensusService = consensus.NewGossipConsensus(
		ctx, pubsubService, interruptChan, func(ev event.ValidationEvent) error {
			if len(selfHashHex) == 0 {
				return errors.New("empty codebase hash")
			}
			if selfHashHex == ev.SelfHashHex {
				return nil
			}
			return errors.New("self hash is not valid")
		},
	)

	return mn, nil
}

func (mn *ModeratorNode) Start() (err error) {
	if mn == nil {
		panic("moderator: nil node")
	}

	mn.node, err = base.NewWarpNode(mn.ctx, mn.options...)
	if err != nil {
		return fmt.Errorf("node: failed to init node: %v", err)
	}
	mn.setupHandlers()

	mn.pubsubService.Run(mn)

	nodeInfo := mn.NodeInfo()

	if err := mn.consensusService.Start(mn); err != nil {
		return err
	}
	mn.consensusService.AskValidation(event.ValidationEvent{nodeInfo.ID.String(), mn.selfHashHex, nil}) // blocking call

	mn.store, err = ipfs.NewIPFS(mn.ctx, mn.node.Node())
	if err != nil {
		return fmt.Errorf("failed to init moderator IPFS node: %v", err)
	}

	var (
		confModelPath = config.Config().Node.Moderator.Path
		cid           = config.Config().Node.Moderator.CID
	)
	if err = ensureModelPresence(confModelPath, cid, mn.store); err != nil {
		return err
	}

	readyChan <- struct{}{}

	// wait until moderator set up
	if err := mn.pubsubService.SubscribeModerationTopic(); err != nil {
		return err
	}

	println()
	fmt.Printf(
		"\033[1mMODERATOR NODE STARTED WITH ID %s AND ADDRESSES %v\033[0m\n",
		nodeInfo.ID.String(), nodeInfo.Addresses,
	)
	println()
	return nil
}

func (mn *ModeratorNode) NodeInfo() warpnet.NodeInfo {
	baseInfo := mn.node.BaseNodeInfo()
	baseInfo.OwnerId = warpnet.ModeratorOwner
	return baseInfo
}

func (mn *ModeratorNode) setupHandlers() {
	if mn.node == nil {
		panic("bootstrap: nil inner p2p node")
	}
	mw := middleware.NewWarpMiddleware()
	logMw := mw.LoggingMiddleware
	unwrapMw := mw.UnwrapStreamMiddleware

	mn.node.SetStreamHandlers(
		warpnet.WarpHandler{
			event.PRIVATE_POST_NODE_VALIDATE,
			logMw(unwrapMw(handler.StreamValidateHandler(mn.consensusService))),
		},
		warpnet.WarpHandler{
			event.PUBLIC_POST_NODE_VALIDATION_RESULT,
			logMw(unwrapMw(handler.StreamValidationResponseHandler(mn.consensusService))),
		},
		warpnet.WarpHandler{
			event.PUBLIC_GET_INFO,
			logMw(handler.StreamGetInfoHandler(mn, nil)),
		},
		warpnet.WarpHandler{
			event.PUBLIC_POST_NODE_CHALLENGE,
			logMw(mw.UnwrapStreamMiddleware(handler.StreamChallengeHandler(root.GetCodeBase(), mn.privKey))),
		},
		warpnet.WarpHandler{
			event.PRIVATE_POST_MODERATE, // TODO protect this endpoint
			logMw(mw.UnwrapStreamMiddleware(handler.StreamModerateHandler(mn, moderator))),
		},
	)
}

func ensureModelPresence(path, cid string, store DistributedStorer) error {
	var fileExists bool

	f, err := os.Open(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read model path: %v", err)
	}

	fileExists = !os.IsNotExist(err)

	if fileExists {
		// send it to background
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			cid, err = store.PutStream(ctx, f)
			if err != nil {
				log.Errorf("failed to put file in IPFS: %v", err)
				_ = f.Close()
				return
			}
			log.Infof("moderator: LLM model uploaded: CID: %s", cid)
			_ = f.Close()
		}()

		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	reader, err := store.GetStream(ctx, cid)
	if err != nil {
		return fmt.Errorf("failed to get stream in IPFS: %v", err)
	}
	defer reader.Close()

	log.Infof("moderator: LLM model downloaded: CID: %s", cid)

	path = "llama-2-7b-chat.Q8_0.gguf.tmp"
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

func (mn *ModeratorNode) SelfStream(path stream.WarpRoute, data any) (_ []byte, err error) {
	if mn == nil || mn.node == nil {
		return nil, nil
	}
	return mn.node.SelfStream(path, data)
}

func (mn *ModeratorNode) GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error) {
	nodeId := warpnet.FromStringToPeerID(nodeIdStr)
	return mn.node.Stream(nodeId, path, data)
}

func (mn *ModeratorNode) SimpleConnect(pi warpnet.WarpAddrInfo) error {
	return mn.node.Connect(pi)
}

func (mn *ModeratorNode) Peerstore() warpnet.WarpPeerstore {
	if mn == nil || mn.node == nil {
		return nil
	}
	return mn.node.Node().Peerstore()
}

func (mn *ModeratorNode) Node() warpnet.P2PNode {
	if mn == nil || mn.node == nil {
		return nil
	}
	return mn.node.Node()
}

func (mn *ModeratorNode) Stop() {
	if mn == nil {
		return
	}

	if mn.pubsubService != nil {
		if err := mn.pubsubService.Close(); err != nil {
			log.Errorf("moderator: failed to close pubsub: %v", err)
		}
	}

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

	if mn.consensusService != nil {
		mn.consensusService.Close()
	}

	mn.node.StopNode()
}
