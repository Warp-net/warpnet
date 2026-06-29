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

package handlers

import (
	"context"
	"crypto/ed25519"
	"github.com/Masterminds/semver/v3"
	bnode "github.com/Warp-net/warpnet/cmd/node/business/node"
	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/metrics"
	"github.com/Warp-net/warpnet/security"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const pathIsFirstRun = "is-first-run"

// maxInflightDispatches bounds the per-connection goroutines a slow client (or a
// burst of dashboard calls) can spawn, so one connection can't exhaust memory.
const maxInflightDispatches = 32

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     sameOrigin, // reject cross-site WebSocket hijacking
}

// sameOrigin permits only the dashboard served from this node: a request whose
// Origin host matches the Host it connects to. A missing Origin (non-browser
// clients, e.g. health probes) is allowed; any other origin is rejected so a
// malicious page can't open a /ws connection to this node.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host
}

type Codec interface {
	Decode(frame []byte) (plain []byte, encrypted bool)
	Encode(reply []byte, encrypted bool) ([]byte, error)
}

type Node interface {
	SelfStream(path stream.WarpRoute, data any) ([]byte, error)
}

// Authenticator is the slice of the auth service the dispatcher uses: log the
// owner in and out, sign self-stream requests with their key, and hand the
// node its owner storage.
type Authenticator interface {
	AuthLogin(message event.LoginEvent, psk security.PSK) (event.LoginResponse, error)
	AuthLogout()
	Reset()
	PrivateKey() ed25519.PrivateKey
	Storage() auth.AuthPersistencyLayer
}

// BridgeStorer is the store contract the bridge owns and hands to the business
// node. It is declared here (not imported from the member package) so the
// business layer depends on its own abstraction, not on member internals.
type BridgeStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
	NewReadTxn() (local_store.WarpTransactioner, error)
	Get(key local_store.DatabaseKey) ([]byte, error)
	GetExpiration(key local_store.DatabaseKey) (uint64, error)
	GetSize(key local_store.DatabaseKey) (int64, error)
	Sync() error
	IsClosed() bool
	InnerDB() *local_store.WarpDB
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
	Set(key local_store.DatabaseKey, value []byte) error
	Delete(key local_store.DatabaseKey) error
	Path() string
	Stats() map[string]string
	IsFirstRun() bool
	Close()
}

type BridgeHandler struct {
	ctx            context.Context
	codec          Codec
	auth           Authenticator
	db             BridgeStorer
	mx             sync.RWMutex
	node           Node
	dbDir          string
	version        *semver.Version
	readyChan      chan domain.AuthNodeInfo
	bootstrapPeers []string
	metricsGateway string
	network        string
	psk            security.PSK
}

func NewBridgeHandler(
	ctx context.Context,
	codec Codec,
	dbDir string,
	version *semver.Version,
	readyChan chan domain.AuthNodeInfo,
	bootstrapPeers []string,
	metricsGateway string,
) *BridgeHandler {
	return &BridgeHandler{
		ctx:            ctx,
		codec:          codec,
		dbDir:          dbDir,
		version:        version,
		readyChan:      readyChan,
		bootstrapPeers: bootstrapPeers,
		metricsGateway: metricsGateway,
	}
}

func (b *BridgeHandler) Handle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Errorf("business: ws upgrade: %v", err)
			return
		}

		defer func() { _ = conn.Close() }()

		// Dispatch each message in its own goroutine so a slow libp2p self-stream
		// can't head-of-line block every other dashboard call on the connection.
		// writeMx serializes WriteMessage (gorilla allows a single writer); sem
		// bounds in-flight work; the frontend matches replies by message_id, so
		// out-of-order responses are fine.
		var writeMx sync.Mutex
		var inflight sync.WaitGroup
		sem := make(chan struct{}, maxInflightDispatches)

		respond := func(req event.Message, encrypted bool) {
			out, err := json.Marshal(b.dispatch(req))
			if err != nil {
				log.Errorf("business: ws marshal: %v", err)
				return
			}
			if out, err = b.codec.Encode(out, encrypted); err != nil {
				log.Errorf("business: ws encode: %v", err)
				return
			}
			writeMx.Lock()
			defer writeMx.Unlock()
			if err := conn.WriteMessage(websocket.TextMessage, out); err != nil {
				_ = conn.Close() // unblock ReadMessage so the loop exits
			}
		}

		for {
			_, frame, err := conn.ReadMessage()
			if err != nil {
				inflight.Wait()
				return
			}

			plain, encrypted := b.codec.Decode(frame)
			var req event.Message
			if err := json.Unmarshal(plain, &req); err != nil {
				log.Warnf("business: ws envelope: %v", err)
				continue
			}

			// Login/logout are connection-wide state transitions (they open/close
			// the DB and drive a shared auth handshake), so they must not overlap
			// any in-flight call: drain first, then run synchronously as a barrier.
			if req.Destination == event.PRIVATE_POST_LOGIN || req.Destination == event.PRIVATE_POST_LOGOUT {
				inflight.Wait()
				respond(req, encrypted)
				continue
			}

			sem <- struct{}{}
			inflight.Go(func() {
				defer func() { <-sem }()
				respond(req, encrypted)
			})
		}
	}
}

func (b *BridgeHandler) AttachNode(n Node) {
	b.mx.Lock()
	b.node = n
	b.mx.Unlock()
}

func (b *BridgeHandler) dispatch(req event.Message) event.Message {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("business: dispatch panic: %v", r)
		}
	}()
	resp := event.Message{
		MessageId:   req.MessageId,
		Destination: req.Destination,
		Timestamp:   time.Now(),
		Version:     "0.0.0",
	}

	switch req.Destination {
	case pathIsFirstRun:
		resp.Body = b.isFirstRun(req.Body)
	case event.PRIVATE_POST_LOGIN:
		resp.Body = b.login(req.Body)
	case event.PRIVATE_POST_LOGOUT:
		b.auth.AuthLogout() // closes the database; the node keeps running
		b.auth.Reset()      // clear the auth guard so the next login can re-authenticate
		resp.Body = json.RawMessage(`["logged_out"]`)
	default:
		resp.Body = b.call(req)
	}

	if resp.Body == nil {
		resp.Body = newErrorResp("response body is empty")
	}
	return resp
}

func (b *BridgeHandler) login(body json.RawMessage) json.RawMessage {
	var ev event.LoginEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return newErrorResp(err.Error())
	}
	if err := b.setupAuth(ev.Network); err != nil {
		return newErrorResp(err.Error())
	}
	loginResp, err := b.auth.AuthLogin(ev, b.psk)
	if err != nil {
		log.Errorf("business: auth: %v", err)
		return newErrorResp(err.Error())
	}
	bt, err := json.Marshal(loginResp)
	if err != nil {
		return newErrorResp(err.Error())
	}
	return bt
}

// setupAuth opens the network-scoped database and wires the auth service for
// the login-supplied network. It does not authenticate — login does that next.
func (b *BridgeHandler) setupAuth(network string) error {
	psk, err := security.GeneratePSK(network, b.version)
	if err != nil {
		return err
	}
	dbPath := filepath.Join(local_store.GetAppPath(), strings.TrimSpace(network), strings.TrimSpace(b.dbDir))
	db, err := local_store.New(dbPath, local_store.DefaultOptions())
	if err != nil {
		return err
	}
	b.db = db
	b.network = network
	b.psk = psk
	userRepo := database.NewUserRepo(db)
	authRepo := database.NewAuthRepo(db, network)
	b.auth = auth.NewAuthService(b.ctx, authRepo, userRepo, b.readyChan)
	return nil
}

// isFirstRun answers the cleartext is-first-run probe (it precedes the channel
// key) from the network-scoped lock file, without opening the database.
func (b *BridgeHandler) isFirstRun(body json.RawMessage) json.RawMessage {
	var ev event.LoginEvent
	_ = json.Unmarshal(body, &ev)
	dbPath := filepath.Join(local_store.GetAppPath(), strings.TrimSpace(ev.Network), strings.TrimSpace(b.dbDir))
	out, _ := json.Marshal(local_store.IsFirstRunAt(dbPath))
	return out
}

// RunNode waits for the auth handshake on readyChan, starts the node once, and
// reports its info back. The network and PSK are stashed by login before the
// auth handshake fires, so the channel hand-off makes them visible here.
func (b *BridgeHandler) RunNode() {
	var node *bnode.BusinessNode
	defer func() {
		if node != nil {
			node.Stop()
		}
	}()

	for {
		var info domain.AuthNodeInfo
		select {
		case <-b.ctx.Done():
			return
		case info = <-b.readyChan:
			log.Infoln("business: database authentication passed")
		}

		if node == nil {
			privateKey := b.auth.PrivateKey()
			ownNodeId, err := warpnet.IDFromPublicKey(privateKey.Public().(ed25519.PublicKey))
			if err != nil {
				log.Errorf("business: node ID: %v", err)
				return
			}

			infos, err := addrInfos(b.bootstrapPeers)
			if err != nil {
				log.Errorf("business: bootstrap infos: %v", err)
				return
			}

			m := metrics.NewMetricsClient(b.metricsGateway, ownNodeId.String(), b.network)
			node, err = bnode.NewBusinessNode(
				b.ctx,
				privateKey,
				b.psk,
				ownNodeId,
				b.auth.Storage(),
				b.db,
				b.network,
				infos,
				m,
			)
			if err != nil {
				log.Errorf("business: init node: %v", err)
				return
			}

			if err := node.Start(); err != nil {
				log.Errorf("business: start node: %v", err)
				return
			}

			b.AttachNode(node)
		}

		ni := node.NodeInfo()
		info.ID = ni.ID.String()
		info.Network = b.network
		info.Addresses = ni.Addresses
		info.Role = ni.Type
		info.BootstrapPeers = b.bootstrapPeers
		b.readyChan <- info
	}
}

func (b *BridgeHandler) call(req event.Message) json.RawMessage {
	b.mx.RLock()
	n := b.node
	b.mx.RUnlock()
	if n == nil {
		return newErrorResp("not attached server node")
	}
	req.Signature = security.Sign(b.auth.PrivateKey(), req.Body)
	respData, err := n.SelfStream(stream.WarpRoute(req.Destination), req)
	if err != nil {
		return newErrorResp(err.Error())
	}
	return respData
}

// addrInfos parses bootstrap multiaddr strings into the AddrInfos the node
// needs; the same strings are reported verbatim as AuthNodeInfo.BootstrapPeers.
func addrInfos(peers []string) ([]warpnet.WarpAddrInfo, error) {
	infos := make([]warpnet.WarpAddrInfo, 0, len(peers))
	for _, p := range peers {
		maddr, err := warpnet.NewMultiaddr(p)
		if err != nil {
			return nil, err
		}
		info, err := warpnet.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			return nil, err
		}
		infos = append(infos, *info)
	}
	return infos, nil
}

func newErrorResp(msg string) json.RawMessage {
	bt, _ := json.Marshal(event.ResponseError{Code: http.StatusInternalServerError, Message: msg})
	return bt
}
