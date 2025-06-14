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
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/oklog/ulid/v2"
	log "github.com/sirupsen/logrus"
	"math"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

type UserPersistencyLayer interface {
	Create(user domain.User) (domain.User, error)
	Update(userId string, newUser domain.User) (domain.User, error)
}

type AuthPersistencyLayer interface {
	Authenticate(username, password string) error
	SessionToken() string
	GetOwner() domain.Owner
	SetOwner(domain.Owner) (domain.Owner, error)
}

type AuthService struct {
	isAuthenticated *atomic.Bool
	userPersistence UserPersistencyLayer
	authPersistence AuthPersistencyLayer
	interrupt       chan os.Signal
	authReady       chan domain.AuthNodeInfo
}

func NewAuthService(
	authRepo AuthPersistencyLayer,
	userRepo UserPersistencyLayer,
	interrupt chan os.Signal,
	authReady chan domain.AuthNodeInfo,
) *AuthService {
	return &AuthService{
		new(atomic.Bool),
		userRepo,
		authRepo,
		interrupt,
		authReady,
	}
}

func (as *AuthService) IsAuthenticated() bool {
	return as.isAuthenticated.Load()
}

func (as *AuthService) AuthLogin(message event.LoginEvent) (authInfo event.LoginResponse, err error) {
	if as.isAuthenticated.Load() {
		return event.LoginResponse{
			Identity: domain.Identity{
				Token: as.authPersistence.SessionToken(),
				Owner: as.authPersistence.GetOwner(),
			},
		}, nil
	}
	log.Infof("authenticating user %s", message.Username)

	message.Password = strings.TrimSpace(message.Password)

	if err := validatePassword(message.Password); err != nil {
		return authInfo, err
	}

	if err := as.authPersistence.Authenticate(message.Username, message.Password); err != nil {
		log.Errorf("authentication failed: %v", err)
		return authInfo, fmt.Errorf("authentication failed: %v", err)
	}
	token := as.authPersistence.SessionToken()
	owner := as.authPersistence.GetOwner()

	var user domain.User
	if owner.UserId == "" {
		id := ulid.Make().String()
		log.Infoln("creating new owner:", id)
		owner, err = as.authPersistence.SetOwner(domain.Owner{
			CreatedAt: time.Now(),
			Username:  message.Username,
			UserId:    id,
		})
		if err != nil {
			log.Errorf("new owner creation failed: %v", err)
			return authInfo, fmt.Errorf("create owner: %v", err)

		}
		user, err = as.userPersistence.Create(domain.User{
			CreatedAt: owner.CreatedAt,
			Id:        id,
			NodeId:    "None",
			Username:  owner.Username,
			Latency:   math.MaxInt64, // put your user at the end of a who-to-follow list
		})
		if err != nil {
			return authInfo, fmt.Errorf("new user creation failed: %v", err)
		}
	}

	if owner.Username != message.Username {
		log.Errorf("username mismatch: %s == %s", owner.Username, message.Username)
		return authInfo, fmt.Errorf("user %s doesn't exist", message.Username)
	}
	as.authReady <- domain.AuthNodeInfo{
		Identity: domain.Identity{Owner: owner, Token: token},
	}

	log.Infoln("OWNER USER ID:", owner.UserId)

	timer := time.NewTimer(time.Minute * 5)
	defer timer.Stop()
	select {
	case <-timer.C:
		log.Errorln("node startup failed: timeout")
		return authInfo, errors.New("node starting is timed out")
	case authInfo = <-as.authReady:
		user.Id = owner.UserId
		user.Username = owner.Username
		user.CreatedAt = owner.CreatedAt
		user.Latency = math.MaxInt64 // put your user at the end of a who-to-follow list
		user.NodeId = authInfo.Identity.Owner.NodeId
		owner.NodeId = authInfo.Identity.Owner.NodeId
	}

	if owner, err = as.authPersistence.SetOwner(owner); err != nil {
		log.Errorf("owner update failed: %v", err)
	}
	if _, err = as.userPersistence.Update(user.Id, user); err != nil {
		log.Errorf("user update failed: %v", err)
	}

	as.isAuthenticated.Store(true)

	return event.LoginResponse(authInfo), nil
}

func validatePassword(pw string) error {
	if pw == "" {
		return errors.New("empty password")
	}
	if len(pw) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(pw) > 32 {
		return errors.New("password must be less than 32 characters")
	}

	var (
		hasUpper   = regexp.MustCompile(`[A-Z]`).MatchString
		hasLower   = regexp.MustCompile(`[a-z]`).MatchString
		hasNumber  = regexp.MustCompile(`[0-9]`).MatchString
		hasSpecial = regexp.MustCompile(`[\W_]`).MatchString
	)

	switch {
	case !hasUpper(pw):
		return errors.New("password must have at least one uppercase letter")
	case !hasLower(pw):
		return errors.New("password must have at least one lowercase letter")
	case !hasNumber(pw):
		return errors.New("password must have at least one digit")
	case !hasSpecial(pw):
		return errors.New("password must have at least one special character")
	}
	return nil
}

func (as *AuthService) AuthLogout() error {
	as.interrupt <- os.Interrupt
	as.isAuthenticated.Store(false)
	return nil
}
