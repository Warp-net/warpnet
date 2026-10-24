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

package remote

import (
	"crypto/ed25519"
	"errors"
	"github.com/Warp-net/warpnet/cmd/node/member/videostream"
	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/security"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
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

type Channel interface {
	Encrypt(plain []byte) ([]byte, error)
	Decrypt(frame []byte) ([]byte, error)
	RemoteStatic() []byte
}

type HandshakeFunc func(read func() ([]byte, error), write func([]byte) error) (Channel, error)

const handshakeTimeout = 10 * time.Second

type Node interface {
	SelfStream(from, to warpnet.WarpPeerID, path stream.WarpRoute, data any) ([]byte, error)
	OpenSelfStream(from, to warpnet.WarpPeerID, path stream.WarpRoute, data any) (warpnet.WarpStream, error)
}

type videoSessions interface {
	Start(id string, offset int64, s warpnet.WarpStream, sender videostream.FrameSender) (domain.VideoManifest, error)
	AddCredits(id string, count int)
	Stop(id string)
	StopAll()
}

// Authenticator is the slice of the auth service the dispatcher uses: log the
// owner in and out, and sign self-stream requests with their key.
type Authenticator interface {
	AuthLogin(message event.LoginEvent, psk security.PSK) (event.LoginResponse, error)
	AuthLogout()
	Reset()
	PrivateKey() ed25519.PrivateKey
	IsAuthenticated() bool
}

type BridgeHandler struct {
	handshake HandshakeFunc
	auth      Authenticator
	firstRun  func() bool
	psk       security.PSK

	mx      sync.RWMutex
	node    Node
	clients map[string]struct{}
}

func NewBridgeHandler(
	handshake HandshakeFunc,
	auth Authenticator,
	psk security.PSK,
	firstRun func() bool,
) *BridgeHandler {
	return &BridgeHandler{
		handshake: handshake,
		auth:      auth,
		psk:       psk,
		firstRun:  firstRun,
		clients:   make(map[string]struct{}),
	}
}

func (b *BridgeHandler) isEnrolled(pub []byte) bool {
	if len(pub) == 0 {
		return false
	}
	b.mx.RLock()
	defer b.mx.RUnlock()
	_, ok := b.clients[string(pub)]
	return ok
}

func (b *BridgeHandler) enroll(pub []byte) {
	if len(pub) == 0 {
		return
	}
	b.mx.Lock()
	b.clients[string(pub)] = struct{}{}
	b.mx.Unlock()
}

type clientConn struct {
	static     []byte
	authorized atomic.Bool
	videos     videoSessions

	conn    *websocket.Conn
	channel Channel
	writeMx *sync.Mutex
}

func (c *clientConn) write(msg event.Message) error {
	out, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.writeMx.Lock()
	defer c.writeMx.Unlock()
	sealed, err := c.channel.Encrypt(out)
	if err != nil {
		_ = c.conn.Close()
		return err
	}
	if err := c.conn.WriteMessage(websocket.BinaryMessage, sealed); err != nil {
		_ = c.conn.Close() // unblock ReadMessage so the loop exits
		return err
	}
	return nil
}

func (c *clientConn) SendFrame(streamId string, frame event.VideoFrame) error {
	body, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return c.write(event.Message{
		MessageId:   streamId,
		Destination: videostream.FramePath,
		Body:        body,
		Timestamp:   time.Now(),
		Version:     "0.0.0",
	})
}

func (b *BridgeHandler) Handle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Errorf("remote: ws upgrade: %v", err)
			return
		}

		defer func() { _ = conn.Close() }()

		_ = conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
		channel, err := b.handshake(
			func() ([]byte, error) {
				_, frame, err := conn.ReadMessage()
				return frame, err
			},
			func(msg []byte) error {
				return conn.WriteMessage(websocket.BinaryMessage, msg)
			},
		)
		if err != nil {
			log.Warnf("remote: ws handshake: %v", err)
			return
		}
		_ = conn.SetReadDeadline(time.Time{})

		// Dispatch each message in its own goroutine so a slow libp2p self-stream
		// can't head-of-line block every other dashboard call on the connection.
		// writeMx serializes Encrypt+WriteMessage (gorilla allows a single writer,
		// counter nonces require wire order to match encryption order); sem bounds
		// in-flight work; the frontend matches replies by message_id.
		var inflight sync.WaitGroup
		sem := make(chan struct{}, maxInflightDispatches)

		c := &clientConn{
			static:  channel.RemoteStatic(),
			videos:  videostream.NewSessions(),
			conn:    conn,
			channel: channel,
			writeMx: new(sync.Mutex),
		}
		c.authorized.Store(b.isEnrolled(c.static))
		defer c.videos.StopAll()

		respond := func(req event.Message) {
			if err := c.write(b.dispatch(req, c)); err != nil {
				log.Errorf("remote: ws write: %v", err)
			}
		}

		for {
			_, frame, err := conn.ReadMessage()
			if err != nil {
				inflight.Wait()
				return
			}

			plain, err := channel.Decrypt(frame)
			if err != nil {
				log.Warnf("remote: ws decrypt: %v", err)
				inflight.Wait()
				return
			}

			var req event.Message
			if err := json.Unmarshal(plain, &req); err != nil {
				log.Warnf("remote: ws envelope: %v", err)
				continue
			}

			// Login/logout are connection-wide state transitions (they open/close
			// the DB and drive a shared auth handshake), so they must not overlap
			// any in-flight call: drain first, then run synchronously as a barrier.
			if req.Destination == event.PRIVATE_POST_LOGIN || req.Destination == event.PRIVATE_POST_LOGOUT {
				inflight.Wait()
				respond(req)
				continue
			}

			sem <- struct{}{}
			inflight.Go(func() {
				defer func() { <-sem }()
				respond(req)
			})
		}
	}
}

func (b *BridgeHandler) AttachNode(n Node) {
	b.mx.Lock()
	b.node = n
	b.mx.Unlock()
}

func (b *BridgeHandler) dispatch(req event.Message, c *clientConn) event.Message {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("remote: dispatch panic: %v", r)
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
		body, _ := json.Marshal(b.firstRun())
		resp.Body = body
	case event.PRIVATE_POST_LOGIN:
		resp.Body = b.login(req.Body, c)
	case event.PRIVATE_GET_VIDEO, event.PRIVATE_GET_CHAT_VIDEO:
		if !b.isAuthorized(c) {
			resp.Body = newUnauthorizedResp()
			break
		}
		resp.Body = b.startVideo(req, c)
	case videostream.CreditPath, videostream.StopPath:
		if !b.isAuthorized(c) {
			resp.Body = newUnauthorizedResp()
			break
		}
		var ev event.VideoStreamEvent
		if err := json.Unmarshal(req.Body, &ev); err != nil {
			resp.Body = newErrorResp(err.Error())
			break
		}
		if req.Destination == videostream.StopPath {
			c.videos.Stop(ev.StreamId)
		} else {
			c.videos.AddCredits(ev.StreamId, ev.Count)
		}
		resp.Body = json.RawMessage(event.Accepted)
	case event.PRIVATE_POST_LOGOUT:
		if !b.isAuthorized(c) {
			resp.Body = newUnauthorizedResp()
			break
		}
		b.auth.AuthLogout() // closes the database; the node keeps running
		b.auth.Reset()      // clear the auth guard so the next login can re-authenticate
		resp.Body = json.RawMessage(`["logged_out"]`)
	default:
		if !b.isAuthorized(c) {
			resp.Body = newUnauthorizedResp()
			break
		}
		resp.Body = b.call(req)
	}

	if resp.Body == nil {
		resp.Body = newErrorResp("response body is empty")
	}
	return resp
}

func (b *BridgeHandler) isAuthorized(c *clientConn) bool {
	return c.authorized.Load() && b.auth.IsAuthenticated()
}

func (b *BridgeHandler) login(body json.RawMessage, c *clientConn) json.RawMessage {
	var ev event.LoginEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return newErrorResp(err.Error())
	}
	loginResp, err := b.auth.AuthLogin(ev, b.psk)
	if err != nil {
		log.Errorf("remote: auth: %v", err)
		return newErrorResp(err.Error())
	}
	bt, err := json.Marshal(loginResp)
	if err != nil {
		return newErrorResp(err.Error())
	}
	b.enroll(c.static)
	c.authorized.Store(true)
	return bt
}

func (b *BridgeHandler) call(req event.Message) json.RawMessage {
	b.mx.RLock()
	n := b.node
	b.mx.RUnlock()
	if n == nil {
		return newErrorResp("not attached server node")
	}
	req, ownNodeId, err := b.signRequest(req)
	if err != nil {
		return newErrorResp(err.Error())
	}
	respData, err := n.SelfStream(ownNodeId, ownNodeId, stream.WarpRoute(req.Destination), req)
	if err != nil {
		return newErrorResp(err.Error())
	}
	return respData
}

func (b *BridgeHandler) startVideo(req event.Message, c *clientConn) json.RawMessage {
	b.mx.RLock()
	n := b.node
	b.mx.RUnlock()
	if n == nil {
		return newErrorResp("not attached server node")
	}

	var ev event.GetVideoChunksEvent
	if err := json.Unmarshal(req.Body, &ev); err != nil {
		return newErrorResp(err.Error())
	}

	req, ownNodeId, err := b.signRequest(req)
	if err != nil {
		return newErrorResp(err.Error())
	}
	s, err := n.OpenSelfStream(ownNodeId, ownNodeId, stream.WarpRoute(req.Destination), req)
	if err != nil {
		return newErrorResp(err.Error())
	}
	manifest, err := c.videos.Start(req.MessageId, ev.Offset, s, c)
	if err != nil && !errors.Is(err, media_meta.ErrVideoUnavailable) {
		return newErrorResp(err.Error())
	}

	body, err := json.Marshal(event.GetVideoChunksResponse{Size: manifest.Size, Mime: manifest.Mime})
	if err != nil {
		return newErrorResp(err.Error())
	}
	return body
}

func (b *BridgeHandler) signRequest(req event.Message) (event.Message, warpnet.WarpPeerID, error) {
	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now()
	}
	req.Timestamp = req.Timestamp.UTC()
	privKey := b.auth.PrivateKey()
	ownNodeId, err := warpnet.IDFromPublicKey(privKey.Public().(ed25519.PublicKey))
	if err != nil {
		return req, "", err
	}
	req.Signature = security.Sign(privKey, req.SigningBytes())
	return req, ownNodeId, nil
}

func newErrorResp(msg string) json.RawMessage {
	bt, _ := json.Marshal(event.ResponseError{Code: http.StatusInternalServerError, Message: msg})
	return bt
}

func newUnauthorizedResp() json.RawMessage {
	bt, _ := json.Marshal(event.ResponseError{
		Code:    http.StatusUnauthorized,
		Message: "this connection is not signed in: log in on this node first",
	})
	return bt
}
