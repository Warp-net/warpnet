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

package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/retrier"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	log "github.com/sirupsen/logrus"
	"io"
	"sync/atomic"
	"time"
)

type ClientStreamer interface {
	Send(peerAddr warpnet.WarpAddrInfo, r stream.WarpRoute, data []byte) ([]byte, error)
}

type WarpClientNode struct {
	ctx            context.Context
	clientNode     warpnet.P2PNode
	streamer       ClientStreamer
	retrier        retrier.Retrier
	serverNodeAddr string
	psk            security.PSK
	isRunning      *atomic.Bool
}

func NewClientNode(ctx context.Context, psk security.PSK) (_ *WarpClientNode, err error) {
	serverNodeAddrDefault := fmt.Sprintf("/ip4/127.0.0.1/tcp/%s/p2p/", config.Config().Node.Port)

	n := &WarpClientNode{
		ctx:            ctx,
		clientNode:     nil,
		retrier:        retrier.New(time.Second*5, 10, retrier.ExponentialBackoff),
		serverNodeAddr: serverNodeAddrDefault,
		isRunning:      new(atomic.Bool),
		psk:            psk,
	}

	return n, nil
}

func (n *WarpClientNode) Pair(serverInfo domain.AuthNodeInfo) error {
	if n == nil {
		return warpnet.WarpError("client: not initialized")
	}

	if serverInfo.NodeInfo.ID.String() == "" {
		return warpnet.WarpError("client node: server node ID is empty")
	}

	serverAddr := n.serverNodeAddr + serverInfo.NodeInfo.ID.String()
	maddr, err := warpnet.NewMultiaddr(serverAddr)
	if err != nil {
		return fmt.Errorf("client: parsing server address: %s", err)
	}

	peerInfo, err := warpnet.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return fmt.Errorf("client: creating address info: %s", err)
	}
	client, err := libp2p.New(
		libp2p.RandomIdentity,
		libp2p.NoListenAddrs,
		libp2p.DisableMetrics(),
		libp2p.DisableRelay(),
		libp2p.DisableIdentifyAddressDiscovery(),
		libp2p.Security(warpnet.NoiseID, warpnet.NewNoise),
		libp2p.Transport(warpnet.NewTCPTransport),
		libp2p.PrivateNetwork(warpnet.PSK(n.psk)),
		libp2p.UserAgent("warpnet-client"),
	)
	if err != nil {
		return fmt.Errorf("client: init %s", err)
	}

	n.clientNode = client
	client.Peerstore().AddAddrs(peerInfo.ID, peerInfo.Addrs, warpnet.PermanentTTL)
	if len(client.Addrs()) != 0 {
		return warpnet.WarpError("client: must have no addresses")
	}

	n.streamer = stream.NewStreamPool(n.ctx, n.clientNode)

	err = n.pairNodes(peerInfo.ID.String(), serverInfo)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	log.Infoln("client-server nodes paired")
	log.Infoln("client node created:", n.clientNode.ID())

	n.isRunning.Store(true)
	return nil
}

func (n *WarpClientNode) pairNodes(nodeId string, serverInfo domain.AuthNodeInfo) error {
	if n == nil {
		log.Errorln("client: must not be nil")
		return warpnet.WarpError("client: must not be nil")
	}

	var resp []byte
	err := n.retrier.Try(n.ctx, func() (err error) {
		resp, err = n.ClientStream(nodeId, event.PRIVATE_POST_PAIR, serverInfo)
		return err
	})
	if err != nil {
		return err
	}

	var errResp event.ErrorResponse
	if _ = json.JSON.Unmarshal(resp, &errResp); errResp.Message != "" {
		return errResp
	}

	return nil
}

func (n *WarpClientNode) IsRunning() bool {
	if n == nil {
		return false
	}
	return n.isRunning.Load()
}

func (n *WarpClientNode) ClientStream(nodeId string, path string, data any) (_ []byte, err error) {
	if n == nil || n.clientNode == nil {
		return nil, warpnet.WarpError("client: not initialized")
	}
	var bt []byte
	if data != nil {
		var ok bool
		bt, ok = data.([]byte)
		if !ok {
			bt, err = json.JSON.Marshal(data)
			if err != nil {
				return nil, err
			}
		}
	}

	addrInfo, err := warpnet.AddrInfoFromString(n.serverNodeAddr + nodeId)
	if err != nil {
		return nil, err
	}

	return n.streamer.Send(*addrInfo, stream.WarpRoute(path), bt)
}

func (n *WarpClientNode) Stop() {
	if n == nil || n.clientNode == nil {
		return
	}
	if err := n.clientNode.Close(); err != nil {
		log.Errorf("client: stop fail: %v", err)
	}
	n.clientNode = nil
	n.isRunning.Store(false)
}
