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

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
)

const rsaKeyBits = 2048

var errNotPEMFile = errors.New("keys: not a PEM file")

// loadOrCreateKey loads an RSA private key from path, generating and
// persisting a new 2048-bit key on first run. Mastodon verifies HTTP
// signatures against an RSA public key, so the gateway signs with RSA even
// though Warpnet node identities are Ed25519. The key must be stable across
// restarts: if it changes, remote followers can no longer verify our posts.
func loadOrCreateKey(path string) (*rsa.PrivateKey, error) {
	bt, err := os.ReadFile(path) //#nosec G304 -- path is operator-provided
	switch {
	case err == nil:
		block, _ := pem.Decode(bt)
		if block == nil {
			return nil, fmt.Errorf("keys: %s: %w", path, errNotPEMFile)
		}
		key, perr := x509.ParsePKCS1PrivateKey(block.Bytes)
		if perr != nil {
			return nil, fmt.Errorf("keys: parse %s: %w", path, perr)
		}
		return key, nil
	case !os.IsNotExist(err):
		return nil, fmt.Errorf("keys: read %s: %w", path, err)
	}

	log.Infof("keys: generating new RSA-%d key at %s", rsaKeyBits, path)
	key, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, fmt.Errorf("keys: generate: %w", err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return nil, fmt.Errorf("keys: write %s: %w", path, err)
	}
	return key, nil
}

// publicKeyPEM renders the public half as the PEM string Mastodon expects in
// the actor document's publicKey.publicKeyPem field.
func publicKeyPEM(key *rsa.PrivateKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", fmt.Errorf("keys: marshal public: %w", err)
	}
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: der}
	return string(pem.EncodeToMemory(block)), nil
}
