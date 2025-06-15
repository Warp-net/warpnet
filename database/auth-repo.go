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

package database

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/database/storage"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"math/big"
	"strings"
	"time"
)

const (
	AuthRepoName    = "/AUTH"
	PassSubName     = "PASS" // TODO pass restore functionality
	DefaultOwnerKey = "OWNER"
)

var ErrNilAuthRepo = storage.DBError("auth repo is nil")

type AuthStorer interface {
	Run(username, password string) (err error)
	Set(key storage.DatabaseKey, value []byte) error
	Get(key storage.DatabaseKey) ([]byte, error)
	NewTxn() (storage.WarpTransactioner, error)
}

type AuthRepo struct {
	db           AuthStorer
	owner        domain.Owner
	sessionToken string
	privateKey   ed25519.PrivateKey
}

func NewAuthRepo(db AuthStorer) *AuthRepo {
	return &AuthRepo{db: db, privateKey: nil}
}

func (repo *AuthRepo) Authenticate(username, password string) (err error) {
	if repo == nil {
		return ErrNilAuthRepo
	}
	if repo.db == nil {
		return storage.ErrNotRunning
	}

	err = repo.db.Run(username, password)
	if err != nil {
		return err
	}

	repo.sessionToken, repo.privateKey, err = repo.generateSecrets(username, password)
	return err
}

func (repo *AuthRepo) generateSecrets(username, password string) (token string, pk ed25519.PrivateKey, err error) {
	n, err := rand.Int(rand.Reader, big.NewInt(127))
	if err != nil {
		return "", nil, err
	}
	randChar := string(uint8(n.Uint64())) //#nosec
	tokenSeed := []byte(username + "@" + password + "@" + randChar + "@" + time.Now().String())
	token = base64.StdEncoding.EncodeToString(security.ConvertToSHA256(tokenSeed))

	pkSeed := base64.StdEncoding.EncodeToString(
		security.ConvertToSHA256(
			[]byte(username + "@" + password + "@" + strings.Repeat("@", len(password))), // no random - private key must be determined
		),
	)
	privateKey, err := security.GenerateKeyFromSeed([]byte(pkSeed))
	if err != nil {
		return "", nil, fmt.Errorf("generate private key from seed: %w", err)
	}

	return token, privateKey, nil
}

func (repo *AuthRepo) SessionToken() string {
	return repo.sessionToken
}

func (repo *AuthRepo) PrivateKey() ed25519.PrivateKey {
	if repo == nil {
		return nil
	}
	if repo.privateKey == nil {
		panic("private key is nil")
	}
	return repo.privateKey
}

func (repo *AuthRepo) GetOwner() domain.Owner {
	if repo == nil {
		panic(ErrNilAuthRepo)
	}
	if repo.owner.UserId != "" {
		return repo.owner
	}

	ownerKey := storage.NewPrefixBuilder(AuthRepoName).
		AddRootID(DefaultOwnerKey).
		Build()

	data, err := repo.db.Get(ownerKey)
	if err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		panic(err)
	}
	if len(data) == 0 {
		return domain.Owner{}
	}

	var owner domain.Owner
	err = json.JSON.Unmarshal(data, &owner)
	if err != nil {
		panic(err)
	}
	return owner

}

func (repo *AuthRepo) SetOwner(o domain.Owner) (_ domain.Owner, err error) {
	ownerKey := storage.NewPrefixBuilder(AuthRepoName).
		AddRootID(DefaultOwnerKey).
		Build()

	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now()
	}
	data, err := json.JSON.Marshal(o)
	if err != nil {
		return o, err
	}

	if err = repo.db.Set(ownerKey, data); err != nil {
		return o, err
	}
	repo.owner = o
	return o, nil
}
