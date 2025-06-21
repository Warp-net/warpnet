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
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/node/base"
	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database/ipfs"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
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

type ModeratorNode struct {
	node warpnet.P2PNode

	ctx context.Context

	streamer          Streamer
	pubsubService     PubSubProvider
	dHashTable        DistributedHashTableCloser
	store             DistributedStorer
	memoryStoreCloseF func() error
	psk               security.PSK
	selfHashHex       string
}

func NewModeratorNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	selfHashHex string,
) (_ *ModeratorNode, err error) {
	pubsubService := pubsub.NewModeratorPubSub(ctx)
	memoryStore, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, fmt.Errorf("moderator: fail creating memory peerstore: %w", err)
	}
	mapStore := datastore.NewMapDatastore()

	closeF := func() error {
		_ = memoryStore.Close()
		return mapStore.Close()
	}

	dHashTable := dht.NewDHTable(ctx, mapStore, nil)

	limiter := warpnet.NewAutoScaledLimiter()

	manager, err := warpnet.NewConnManager(limiter)
	if err != nil {
		return nil, err
	}

	rm, err := warpnet.NewResourceManager(limiter)
	if err != nil {
		return nil, err
	}

	infos, err := config.Config().Node.AddrInfos()
	if err != nil {
		return nil, err
	}

	currentNodeID, err := warpnet.IDFromPublicKey(privKey.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, err
	}

	p2pPrivKey, err := p2pCrypto.UnmarshalEd25519PrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	warpNode, err := warpnet.NewP2PNode(
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
		libp2p.ResourceManager(rm),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
		libp2p.UserAgent(warpnet.WarpnetName+"-moderator"),
		libp2p.ConnectionManager(manager),
		libp2p.Routing(dHashTable.StartRouting),
		base.EnableAutoRelayWithStaticRelays(infos, currentNodeID)(),
	)
	if err != nil {
		return nil, fmt.Errorf("node: failed to init node: %v", err)
	}

	mn := &ModeratorNode{
		ctx:               ctx,
		node:              warpNode,
		pubsubService:     pubsubService,
		dHashTable:        dHashTable,
		memoryStoreCloseF: closeF,
		psk:               psk,
		selfHashHex:       selfHashHex,
	}

	mw := middleware.NewWarpMiddleware()
	logMw := mw.LoggingMiddleware
	mn.node.SetStreamHandler(
		event.PUBLIC_GET_INFO,
		logMw(handler.StreamGetInfoHandler(mn, nil)),
	)
	mn.node.SetStreamHandler(
		event.PUBLIC_POST_NODE_CHALLENGE,
		logMw(mw.UnwrapStreamMiddleware(handler.StreamChallengeHandler(root.GetCodeBase(), privKey))),
	)
	return mn, nil
}

func (mn *ModeratorNode) NodeInfo() warpnet.NodeInfo {
	addrs := make([]string, 0, len(mn.node.Addrs()))
	for _, addr := range mn.node.Addrs() {
		addrs = append(addrs, addr.String())
	}
	bi := warpnet.NodeInfo{
		OwnerId:        warpnet.ModeratorOwner,
		ID:             mn.node.ID(),
		Version:        nil,
		Addresses:      addrs,
		StartTime:      time.Time{},
		RelayState:     "off",
		BootstrapPeers: nil,
		Reachability:   warpnet.ReachabilityPrivate,
	}
	return bi
}

func (mn *ModeratorNode) Start() (err error) {
	if mn == nil || mn.node == nil {
		return errors.New("moderator: nil node")
	}

	mn.store, err = ipfs.NewIPFS(mn.ctx, mn.node)
	if err != nil {
		return fmt.Errorf("failed to init moderator IPFS node: %v", err)
	}

	modelPath, err := ensureModelPresence(mn.store)
	if err != nil {
		return err
	}

	log.Infof("moderator: LLM model path: %s", modelPath)

	mn.pubsubService.Run(mn)

	nodeInfo := mn.NodeInfo()

	println()
	fmt.Printf(
		"\033[1mMODERATOR NODE STARTED WITH ID %s AND ADDRESSES %v\033[0m\n",
		nodeInfo.ID.String(), nodeInfo.Addresses,
	)
	println()
	return nil
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

		return path, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	reader, err := store.GetStream(ctx, cid)
	if err != nil {
		return "", fmt.Errorf("failed to get stream in IPFS: %v", err)
	}
	defer reader.Close()

	log.Infoln("moderator: LLM model downloaded: CID: %s", cid)

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

	finalPath := strings.TrimSuffix(path, ".tmp")
	if err = os.Rename(path, finalPath); err != nil {
		log.Errorf("renaming file: %v", err)
	}

	return finalPath, nil
}

func (mn *ModeratorNode) GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error) {
	if mn == nil || mn.streamer == nil {
		return nil, warpnet.WarpError("node is not initialized")
	}
	nodeId := warpnet.FromStringToPeerID(nodeIdStr)

	if mn.node.ID() == nodeId {
		return nil, base.ErrSelfRequest
	}

	peerInfo := mn.node.Peerstore().PeerInfo(nodeId)
	if len(peerInfo.Addrs) == 0 {
		log.Warningf("node %v is offline", nodeId)
		return nil, warpnet.ErrNodeIsOffline
	}

	return mn.stream(peerInfo, path, data)
}

func (mn *ModeratorNode) stream(peerInfo warpnet.WarpAddrInfo, path stream.WarpRoute, data any) (_ []byte, err error) {
	var bt []byte
	if data != nil {
		var ok bool
		bt, ok = data.([]byte)
		if !ok {
			bt, err = json.JSON.Marshal(data)
			if err != nil {
				return nil, fmt.Errorf("node: generic stream: marshal data %v %s", err, data)
			}
		}
	}
	return mn.streamer.Send(peerInfo, path, bt) // TODO retrier
}

func (mn *ModeratorNode) SimpleConnect(pi warpnet.WarpAddrInfo) error {
	return mn.node.Connect(mn.ctx, pi)
}

func (mn *ModeratorNode) Peerstore() warpnet.WarpPeerstore {
	if mn == nil || mn.node == nil {
		return nil
	}
	return mn.node.Peerstore()
}

func (mn *ModeratorNode) Node() warpnet.P2PNode {
	if mn == nil || mn.node == nil {
		return nil
	}
	return mn.node
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

	_ = mn.node.Close()
}
