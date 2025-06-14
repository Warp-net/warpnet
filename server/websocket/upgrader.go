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

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package websocket

import (
	"encoding/base64"
	"errors"
	"github.com/Warp-net/warpnet/security"
	ws "github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"net/http"
	"sync"
	"time"
)

type EncryptedUpgrader struct {
	upgrader        ws.Upgrader
	conn            *ws.Conn
	readCallback    func(msg []byte) ([]byte, error)
	encrypter       *security.DiffieHellmanEncrypter
	mx              *sync.Mutex
	isAuthenticated bool
	externalPubKey  []byte
	isSaltRenewed   bool
	salt            []byte
}

func NewEncryptedUpgrader() *EncryptedUpgrader {
	e, err := security.NewDiffieHellmanEncrypter()
	if err != nil {
		log.Fatalln(err)
	}

	return &EncryptedUpgrader{
		upgrader: ws.Upgrader{
			CheckOrigin: func(r *http.Request) bool { // TODO
				//origin := r.Header.Get("Origin")
				//addr, err := url.Parse(origin)
				//if err != nil {
				//	return false
				//}
				//if addr.Hostname() != "warp.net" {
				//	return false
				//}
				return true
			},
		},
		encrypter: e,
		mx:        new(sync.Mutex),
		salt:      getCurrentDate(),
	}
}

func (s *EncryptedUpgrader) UpgradeConnection(w http.ResponseWriter, r *http.Request) (err error) {
	s.conn, err = s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}

	return s.readLoop()
}

func (s *EncryptedUpgrader) IsCloseError(err error, codes ...int) bool {
	return ws.IsCloseError(err, codes...)
}

func (s *EncryptedUpgrader) Close() {
	if s == nil || s.conn == nil {
		return
	}
	_ = s.conn.Close()
	s.conn = nil
	s.salt = nil
	return
}

func (s *EncryptedUpgrader) OnMessage(fn func(msg []byte) ([]byte, error)) {
	s.readCallback = fn
}

func (s *EncryptedUpgrader) readLoop() error {
	if s == nil || s.conn == nil {
		return errors.New("websocket connection is closed")
	}
	for {
		if s.conn == nil {
			return errors.New("websocket connection is down")
		}
		messageType, message, err := s.conn.ReadMessage()
		if err != nil {
			return err
		}

		if messageType != ws.TextMessage {
			_ = s.SendPlain("message type must be a text")
			continue
		}

		if !s.isAuthenticated {
			log.Infoln("websocket: received client public key")
			pubKey, err := base64.StdEncoding.DecodeString(string(message))
			if err != nil {
				_ = s.SendPlain(err.Error())
				return err
			}
			s.externalPubKey = pubKey

			if err := s.encrypter.ComputeSharedSecret(s.externalPubKey, s.salt); err != nil {
				_ = s.SendPlain(err.Error())
				continue
			}
			log.Infoln("websocket: computed shared secret")

			// send ours public key
			encoded := base64.StdEncoding.EncodeToString(s.encrypter.PublicKey())
			err = s.SendPlain(encoded)
			if err != nil {
				log.Infof("websocket: error sending public key: %s", err)
				continue
			}
			log.Infoln("websocket: sent server public key")

			s.isAuthenticated = true
			log.Infoln("websocket: handshake complete")
			continue
		}

		if s.readCallback == nil {
			log.Infoln("websocket: no read callback provided")
			continue
		}
		decryptedMessage, err := s.encrypter.DecryptMessage(message)
		if err != nil {
			log.Errorf("websocket: failed to decrypt message: %v", err)
			return nil
		}
		response, err := s.readCallback(decryptedMessage)
		if err != nil {
			log.Errorf("websocket: read callback: %v", err)
		}
		if err = s.SendEncrypted(response); err != nil {
			log.Errorf("websocket: failed to send encrypted message: %v", err)
		}
		if err := s.renewSalt(); err != nil {
			log.Errorf("websocket: failed to renew salt: %v", err)
		}
	}
}
func (s *EncryptedUpgrader) SendPlain(msg string) error {
	s.mx.Lock()
	defer s.mx.Unlock()

	return s.conn.WriteMessage(ws.TextMessage, []byte(msg))
}
func (s *EncryptedUpgrader) SetNewSalt(salt string) {
	if s == nil {
		return
	}
	if salt == "" {
		return
	}
	s.mx.Lock()
	defer s.mx.Unlock()
	s.salt = []byte(salt)
	s.isSaltRenewed = false
}

func (s *EncryptedUpgrader) renewSalt() error {
	s.mx.Lock()
	defer s.mx.Unlock()

	if s.isSaltRenewed {
		return nil
	}
	err := s.encrypter.ComputeSharedSecret(s.externalPubKey, s.salt)
	if err != nil {
		return err
	}
	s.isSaltRenewed = true
	log.Infoln("websocket: secret renewed")
	return nil
}

func (s *EncryptedUpgrader) SendEncrypted(msg []byte) error {
	if len(msg) == 0 {
		return nil
	}
	encryptedMessage, err := s.encrypter.EncryptMessage(msg)
	if err != nil {
		return err
	}

	s.mx.Lock()
	defer s.mx.Unlock()

	return s.conn.WriteMessage(ws.TextMessage, encryptedMessage)
}

func getCurrentDate() []byte {
	return []byte(time.Now().UTC().Format("2006-01-02"))
}
