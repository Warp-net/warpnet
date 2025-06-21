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
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/config"
	dht "github.com/Warp-net/warpnet/core/dht"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/node/base"
	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"strings"
	"time"
)

type ModeratorNode struct {
	*base.WarpNode

	discService       DiscoveryHandler
	pubsubService     PubSubProvider
	dHashTable        DistributedHashTableCloser
	memoryStoreCloseF func() error
	psk               security.PSK
	selfHashHex       string
}

func NewModeratorNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	store DistributedStorer,
	selfHashHex string,
) (_ *ModeratorNode, err error) {
	modelPath, err := ensureModelPresence(store)
	if err != nil {
		return nil, err
	}

	log.Infof("moderator: LLM model path: %s", modelPath)

	discService := discovery.NewBootstrapDiscoveryService(ctx)
	pubsubService := pubsub.NewPubSub(
		ctx, nil, warpnet.ModeratorOwner, discService.DefaultDiscoveryHandler,
	)
	memoryStore, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, fmt.Errorf("moderator: fail creating memory peerstore: %w", err)
	}
	mapStore := datastore.NewMapDatastore()

	closeF := func() error {
		_ = memoryStore.Close()
		return mapStore.Close()
	}

	dHashTable := dht.NewDHTable(
		ctx, mapStore,
		nil, discService.DefaultDiscoveryHandler,
	)

	node, err := base.NewWarpNode(
		ctx,
		privKey,
		memoryStore,
		psk,
		nil,
		[]string{
			fmt.Sprintf("/ip6/%s/tcp/%s", config.Config().Node.HostV6, config.Config().Node.Port),
			fmt.Sprintf("/ip4/%s/tcp/%s", config.Config().Node.HostV4, config.Config().Node.Port),
		},
		dHashTable.StartRouting,
	)
	if err != nil {
		return nil, fmt.Errorf("moderator: failed to init node: %v", err)
	}

	bn := &ModeratorNode{
		WarpNode:          node,
		discService:       discService,
		pubsubService:     pubsubService,
		dHashTable:        dHashTable,
		memoryStoreCloseF: closeF,
		psk:               psk,
		selfHashHex:       selfHashHex,
	}

	mw := middleware.NewWarpMiddleware()
	logMw := mw.LoggingMiddleware
	bn.SetStreamHandler(
		event.PUBLIC_GET_INFO,
		logMw(handler.StreamGetInfoHandler(bn, discService.DefaultDiscoveryHandler)),
	)
	bn.SetStreamHandler(
		event.PUBLIC_POST_NODE_CHALLENGE,
		logMw(mw.UnwrapStreamMiddleware(handler.StreamChallengeHandler(root.GetCodeBase(), privKey))),
	)
	return bn, nil
}

func ensureModelPresence(store DistributedStorer) (string, error) {
	var (
		fileExists bool
		path       = config.Config().Node.Moderator.Path
		cid        = config.Config().Node.Moderator.CID
	)

	f, err := os.Open(path)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to read model path: %v", err)
	}
	if f != nil {
		defer f.Close()
	}

	fileExists = !os.IsNotExist(err)

	if fileExists {
		// send to background
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			cid, err := store.PutStream(ctx, f)
			if err != nil {
				log.Errorf("failed to put file in IPFS: %v", err)
				return
			}
			log.Info("CID:", cid)
		}()

		return path, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	reader, err := store.GetStream(ctx, cid)
	if err != nil {
		return "", fmt.Errorf("failed to get stream in IPFS: %v", err)
	}
	defer reader.Close()

	path = "llama-2-7b-chat.Q8_0.gguf.tmp"
	file, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("creating file: %v", err)
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		return "", fmt.Errorf("writing to file: %v", err)
	}
	if err = os.Rename(path, strings.TrimSuffix(path, ".tmp")); err != nil {
		log.Errorf("renaming file: %v", err)
	}

	return path, nil
}

func (bn *ModeratorNode) NodeInfo() warpnet.NodeInfo {
	bi := bn.BaseNodeInfo()
	bi.OwnerId = warpnet.ModeratorOwner
	return bi
}

func (bn *ModeratorNode) Start() error {
	if bn == nil {
		return errors.New("moderator: nil node")
	}
	bn.pubsubService.Run(bn, nil)
	if err := bn.discService.Run(bn); err != nil {
		return err
	}

	nodeInfo := bn.NodeInfo()

	println()
	fmt.Printf(
		"\033[1mMODERATOR NODE STARTED WITH ID %s AND ADDRESSES %v\033[0m\n",
		nodeInfo.ID.String(), nodeInfo.Addresses,
	)
	println()
	return nil
}

func (bn *ModeratorNode) GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error) {
	if bn == nil {
		return
	}
	nodeId := warpnet.FromStringToPeerID(nodeIdStr)
	bt, err := bn.Stream(nodeId, path, data)
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		return bt, nil
	}
	return bt, err
}

func (bn *ModeratorNode) Stop() {
	if bn == nil {
		return
	}
	if bn.discService != nil {
		bn.discService.Close()
	}

	if bn.pubsubService != nil {
		if err := bn.pubsubService.Close(); err != nil {
			log.Errorf("moderator: failed to close pubsub: %v", err)
		}
	}

	if bn.dHashTable != nil {
		bn.dHashTable.Close()
	}

	if bn.memoryStoreCloseF != nil {
		if err := bn.memoryStoreCloseF(); err != nil {
			log.Errorf("moderator: failed to close memory store: %v", err)
		}
	}

	bn.WarpNode.StopNode()
}
