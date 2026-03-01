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

package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/Warp-net/warpnet/security"
	"math"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/oklog/ulid/v2"
	log "github.com/sirupsen/logrus"
)

const ErrUsernamesMismatch warpnet.WarpError = "username doesn't exist"

type UserPersistencyLayer interface {
	Create(user domain.User) (domain.User, error)
	Update(userId string, newUser domain.User) (domain.User, error)
}

type AuthPersistencyLayer interface {
	Authenticate(username, password string) error
	SessionToken() string
	GetOwner() domain.Owner
	SetOwner(domain.Owner) (domain.Owner, error)
	PrivateKey() ed25519.PrivateKey
	Logout()
}

type AuthService struct {
	ctx             context.Context
	isAuthenticated *atomic.Bool
	userPersistence UserPersistencyLayer
	authPersistence AuthPersistencyLayer
	authReady       chan domain.AuthNodeInfo
}

func NewAuthService(
	ctx context.Context,
	authRepo AuthPersistencyLayer,
	userRepo UserPersistencyLayer,
	authReady chan domain.AuthNodeInfo,
) *AuthService {
	return &AuthService{
		ctx,
		new(atomic.Bool),
		userRepo,
		authRepo,
		authReady,
	}
}

func (as *AuthService) Storage() AuthPersistencyLayer {
	return as.authPersistence
}

func (as *AuthService) PrivateKey() ed25519.PrivateKey {
	return as.authPersistence.PrivateKey()
}

func (as *AuthService) IsAuthenticated() bool {
	return as.isAuthenticated.Load()
}

func (as *AuthService) AuthLogin(message event.LoginEvent, psk security.PSK) (authInfo event.LoginResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("auth: panic:", r)
		}
	}()
	if as.isAuthenticated.Load() {
		return event.LoginResponse{
			Identity: domain.Identity{
				Token: as.authPersistence.SessionToken(),
				Owner: as.authPersistence.GetOwner(),
				PSK:   base64.StdEncoding.EncodeToString(psk),
			},
		}, nil
	}

	log.Infof("authenticating user '%s'", message.Username)

	message.Password = strings.TrimSpace(message.Password)

	if err := validatePassword(message.Password); err != nil {
		return authInfo, err
	}

	if err := as.authPersistence.Authenticate(message.Username, message.Password); err != nil {
		log.Errorf("authentication failed: %v", err)
		return authInfo, fmt.Errorf("authentication failed: %w", err)
	}
	token := as.authPersistence.SessionToken()
	owner := as.authPersistence.GetOwner()

	var user domain.User
	if owner.UserId == "" {
		id := ulid.Make().String()
		log.Infoln("creating new user:", id)
		owner, err = as.authPersistence.SetOwner(domain.Owner{
			CreatedAt:       time.Now(),
			Username:        message.Username,
			UserId:          id,
			RedundantUserID: id,
		})
		if err != nil {
			log.Errorf("new owner creation failed: %v", err)
			return authInfo, fmt.Errorf("create owner: %w", err)
		}
		user, err = as.userPersistence.Create(domain.User{
			CreatedAt:     owner.CreatedAt,
			Id:            id,
			NodeId:        "NotSet",
			Username:      owner.Username,
			RoundTripTime: math.MaxInt64, // put your user at the end of a who-to-follow list
		})
		if err != nil {
			return authInfo, fmt.Errorf("new user creation failed: %w", err)
		}
		log.Infof(
			"auth: user created: id: %s, name: '%s', node_id: %s, created_at: %s, latency: %d",
			user.Id,
			user.Username,
			user.NodeId,
			user.CreatedAt,
			user.RoundTripTime,
		)
	}

	if owner.Username != message.Username {
		log.Errorf("username mismatch: '%s' == '%s'", owner.Username, message.Username)
		return authInfo, fmt.Errorf("%w: %s", ErrUsernamesMismatch, message.Username)
	}
	as.authReady <- domain.AuthNodeInfo{
		Identity: domain.Identity{Owner: owner, Token: token, PSK: hex.EncodeToString(psk)},
	}

	select {
	case <-as.ctx.Done():
		log.Errorln("node startup cancelled")
		return authInfo, as.ctx.Err()
	case authInfo = <-as.authReady:
		if authInfo.NodeInfo.ID.String() == "" {
			panic("auth: node id missing")
		}

		user.Id = owner.UserId
		user.Username = owner.Username
		user.CreatedAt = owner.CreatedAt
		user.RoundTripTime = math.MaxInt64 // put your user at the end of a who-to-follow list
		user.NodeId = authInfo.NodeInfo.ID.String()
		owner.NodeId = authInfo.NodeInfo.ID.String()

		log.Infof(
			"auth: user authenticated: id: %s, name: '%s', node_id: %s, created_at: %s, latency: %d",
			user.Id,
			user.Username,
			user.NodeId,
			user.CreatedAt,
			user.RoundTripTime,
		)
	}

	if _, err = as.authPersistence.SetOwner(owner); err != nil {
		log.Errorf("owner update failed: %v", err)
	}
	updatedUser, err := as.userPersistence.Update(user.Id, user)
	if err != nil {
		log.Errorf("user update failed: %v", err)
	}

	as.isAuthenticated.Store(true)

	log.Infof(
		"auth: user updated: id: %s, name: '%s', node_id: %s, updated_at: %s, latency: %d",
		updatedUser.Id,
		updatedUser.Username,
		updatedUser.NodeId,
		updatedUser.UpdatedAt,
		updatedUser.RoundTripTime,
	)

	return event.LoginResponse(authInfo), nil
}

const (
	MinPasswordLength = 8
	MaxPasswordLength = 32

	ErrEmptyPassword             warpnet.WarpError = "empty password"
	ErrMinPasswordLength         warpnet.WarpError = "password must be at least 8 characters"
	ErrMaxPasswordLength         warpnet.WarpError = "password must be at least 32 characters"
	ErrPasswordUpperCaseRequired warpnet.WarpError = "password must have at least one uppercase letter"
	ErrPasswordLowerCaseRequired warpnet.WarpError = "password must have at least one lowercase letter"
	ErrPasswordDigitRequired     warpnet.WarpError = "password must have at least one digit"
	ErrPasswordSpecialRequired   warpnet.WarpError = "password must have at least one special character"
)

func validatePassword(pw string) error {
	if pw == "" {
		return ErrEmptyPassword
	}
	if len(pw) < MinPasswordLength {
		return ErrMinPasswordLength
	}
	if len(pw) > MaxPasswordLength {
		return ErrMaxPasswordLength
	}

	var (
		hasUpper   = regexp.MustCompile(`[A-Z]`).MatchString
		hasLower   = regexp.MustCompile(`[a-z]`).MatchString
		hasNumber  = regexp.MustCompile(`[0-9]`).MatchString
		hasSpecial = regexp.MustCompile(`[\W_]`).MatchString
	)

	switch {
	case !hasUpper(pw):
		return ErrPasswordUpperCaseRequired
	case !hasLower(pw):
		return ErrPasswordLowerCaseRequired
	case !hasNumber(pw):
		return ErrPasswordDigitRequired
	case !hasSpecial(pw):
		return ErrPasswordSpecialRequired
	}
	return nil
}
