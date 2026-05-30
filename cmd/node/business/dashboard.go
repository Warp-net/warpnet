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

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"

	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const sessionCookieName = "warpnet_business_session"

// errShortWSFrame is a package-level sentinel so the dynamic-error linter is
// satisfied (and callers could errors.Is it if they ever needed to).
var errShortWSFrame = errors.New("business: ws frame too short")

// session holds per-login state. aesKey is sha256(password): the preshared
// secret for the encrypted WS channel is the user's own password, so there is
// no extra secret to manage.
type session struct {
	aesKey []byte
}

type sessionStore struct {
	mu sync.RWMutex
	m  map[string]*session
}

func newSessionStore() *sessionStore {
	return &sessionStore{m: make(map[string]*session)}
}

func (s *sessionStore) create(aesKey []byte) string {
	id := randomHex(32)
	s.mu.Lock()
	s.m[id] = &session{aesKey: aesKey}
	s.mu.Unlock()
	return id
}

func (s *sessionStore) get(id string) (*session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.m[id]
	return sess, ok
}

func (s *sessionStore) delete(id string) {
	s.mu.Lock()
	delete(s.m, id)
	s.mu.Unlock()
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func deriveKey(password string) []byte {
	sum := sha256.Sum256([]byte(password))
	return sum[:]
}

func (s *Server) handleIsFirstRun(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.isFirstRun())
}

// handleCall is the HTTP boundary the frontend transport bridge talks to.
// Login and logout are open (login establishes the session, logout tears it
// down); every other path requires a session cookie — the gate backed by the
// PRIVATE_POST_LOGIN handler.
func (s *Server) handleCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg AppMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if !isOpenPath(msg.Path) && !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	resp, password := s.dispatch(msg)

	switch msg.Path {
	case event.PRIVATE_POST_LOGIN:
		if password != "" { // non-empty only when AuthLogin succeeded
			id := s.sessions.create(deriveKey(password))
			http.SetCookie(w, sessionCookie(id, false))
		}
	case event.PRIVATE_POST_LOGOUT:
		if c, err := r.Cookie(sessionCookieName); err == nil {
			s.sessions.delete(c.Value)
		}
		http.SetCookie(w, sessionCookie("", true))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleWS is the AES-256-GCM channel. Frames are base64(nonce||ciphertext)
// sealed with the session key (= sha256(password)); the decrypted payload is
// the same AppMessage envelope handleCall accepts and is dispatched the same way.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     func(_ *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("business: ws upgrade: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	for {
		_, frame, err := conn.ReadMessage()
		if err != nil {
			return
		}
		plain, err := aesOpen(sess.aesKey, frame)
		if err != nil {
			log.Warnf("business: ws decrypt: %v", err)
			continue
		}

		var msg AppMessage
		if err := json.Unmarshal(plain, &msg); err != nil {
			log.Warnf("business: ws envelope: %v", err)
			continue
		}
		resp, _ := s.dispatch(msg)

		out, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		sealed, err := aesSeal(sess.aesKey, out)
		if err != nil {
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, sealed); err != nil {
			return
		}
	}
}

func isOpenPath(path string) bool {
	return path == event.PRIVATE_POST_LOGIN || path == event.PRIVATE_POST_LOGOUT
}

func (s *Server) authorized(r *http.Request) bool {
	_, ok := s.session(r)
	return ok
}

func (s *Server) session(r *http.Request) (*session, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, false
	}
	return s.sessions.get(c.Value)
}

func sessionCookie(value string, expire bool) *http.Cookie {
	c := &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	if expire {
		c.MaxAge = -1
	}
	return c
}

func aesSeal(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nonce, nonce, plaintext, nil)
	enc := make([]byte, base64.StdEncoding.EncodedLen(len(ct)))
	base64.StdEncoding.Encode(enc, ct)
	return enc, nil
}

func aesOpen(key, b64 []byte) ([]byte, error) {
	data := make([]byte, base64.StdEncoding.DecodedLen(len(b64)))
	n, err := base64.StdEncoding.Decode(data, b64)
	if err != nil {
		return nil, err
	}
	data = data[:n]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errShortWSFrame
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}
