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

//nolint:all
package auth

import (
	"context"
	"crypto/ed25519"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testNodeID = "12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"

type fakeAuthRepo struct {
	mu sync.Mutex

	authErr   error
	authCalls []string

	owner    domain.Owner
	setErr   error
	setOwner []domain.Owner

	token      string
	privKey    ed25519.PrivateKey
	logoutSeen int
}

func newFakeAuthRepo() *fakeAuthRepo {
	return &fakeAuthRepo{token: "session-token"}
}

func (f *fakeAuthRepo) Authenticate(username, password string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authCalls = append(f.authCalls, username+":"+password)
	return f.authErr
}

func (f *fakeAuthRepo) SessionToken() string { return f.token }

func (f *fakeAuthRepo) GetOwner() domain.Owner {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.owner
}

func (f *fakeAuthRepo) SetOwner(o domain.Owner) (domain.Owner, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setOwner = append(f.setOwner, o)
	if f.setErr != nil {
		return domain.Owner{}, f.setErr
	}
	f.owner = o
	return o, nil
}

func (f *fakeAuthRepo) PrivateKey() ed25519.PrivateKey { return f.privKey }

func (f *fakeAuthRepo) Logout() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logoutSeen++
}

type fakeUserRepo struct {
	mu sync.Mutex

	createErr error
	created   []domain.User
	updated   []domain.User
	updateErr error
}

func (f *fakeUserRepo) Create(user domain.User) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, user)
	if f.createErr != nil {
		return domain.User{}, f.createErr
	}
	return user, nil
}

func (f *fakeUserRepo) Update(userId string, newUser domain.User) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updated = append(f.updated, newUser)
	return newUser, f.updateErr
}

func (f *fakeUserRepo) snapshot() ([]domain.User, []domain.User) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.User(nil), f.created...), append([]domain.User(nil), f.updated...)
}

func newService(t *testing.T, authRepo *fakeAuthRepo, userRepo *fakeUserRepo, reply *domain.AuthNodeInfo) *AuthService {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ready := make(chan domain.AuthNodeInfo)
	as := NewAuthService(ctx, authRepo, userRepo, ready)

	if reply != nil {
		go func() {
			select {
			case <-ctx.Done():
				return
			case <-ready: // the request the service publishes
			}
			select {
			case <-ctx.Done():
			case ready <- *reply:
			}
		}()
	}
	return as
}

func TestValidatePassword_RejectsWeakSecrets(t *testing.T) {
	cases := []struct {
		pw   string
		want error
	}{
		{"", ErrEmptyPassword},
		{"Ab1!", ErrMinPasswordLength},
		{"Abcdefg1", ErrPasswordSpecialRequired},
		{"abcdefg1!", ErrPasswordUpperCaseRequired},
		{"ABCDEFG1!", ErrPasswordLowerCaseRequired},
		{"Abcdefgh!", ErrPasswordDigitRequired},
		{strings.Repeat("Aa1!", 9), ErrMaxPasswordLength},
		{"ПАРОЛЬ1A!", ErrPasswordLowerCaseRequired},
		{"пароль1a!", ErrPasswordUpperCaseRequired},
	}

	for _, c := range cases {
		assert.ErrorIsf(t, validatePassword(c.pw), c.want, "password %q", c.pw)
	}
}

func TestValidatePassword_AcceptsStrongSecrets(t *testing.T) {
	for _, pw := range []string{
		"Claude1234$",
		"Aa1!aaaa",
		"Пароль1Aa!",              // non-ASCII is fine alongside the ASCII classes
		strings.Repeat("Aa1!", 8), // exactly at the ceiling
	} {
		assert.NoErrorf(t, validatePassword(pw), "password %q", pw)
	}
}

func TestAuthLogin_TrimsPasswordBeforeValidating(t *testing.T) {
	repo := newFakeAuthRepo()
	as := newService(t, repo, &fakeUserRepo{}, nil)

	_, err := as.AuthLogin(event.LoginEvent{Username: "u", Password: "            "}, security.PSK{})
	assert.ErrorIs(t, err, ErrEmptyPassword, "whitespace is not a password")
}

func TestAuthLogin_SurroundingWhitespaceIsIgnored(t *testing.T) {
	repo := newFakeAuthRepo()
	as := newService(t, repo, &fakeUserRepo{}, &domain.AuthNodeInfo{ID: testNodeID})

	_, err := as.AuthLogin(event.LoginEvent{Username: "alice", Password: "  Claude1234$  "}, security.PSK{})
	require.NoError(t, err)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.authCalls, 1)
	assert.Equal(t, "alice:Claude1234$", repo.authCalls[0],
		"the trimmed password must be what unlocks the database")
}

func TestAuthLogin_WrongCredentialsAreRejected(t *testing.T) {
	repo := newFakeAuthRepo()
	repo.authErr = errors.New("wrong password")
	users := &fakeUserRepo{}

	as := newService(t, repo, users, nil)

	_, err := as.AuthLogin(event.LoginEvent{Username: "alice", Password: "Claude1234$"}, security.PSK{})
	assert.Error(t, err)
	assert.False(t, as.IsAuthenticated(), "a failed login must not mark the session authenticated")

	created, _ := users.snapshot()
	assert.Empty(t, created, "a failed login must not create an account")
}

func TestAuthLogin_FirstLoginCreatesOwnerAndUser(t *testing.T) {
	repo := newFakeAuthRepo()
	users := &fakeUserRepo{}
	as := newService(t, repo, users, &domain.AuthNodeInfo{ID: testNodeID, Role: "member"})

	resp, err := as.AuthLogin(event.LoginEvent{Username: "alice", Password: "Claude1234$"}, security.PSK{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, testNodeID, resp.ID)
	assert.True(t, as.IsAuthenticated())

	created, updated := users.snapshot()
	require.Len(t, created, 1)
	assert.Equal(t, "alice", created[0].Username)
	assert.NotEmpty(t, created[0].Id)

	require.Len(t, updated, 1)
	assert.Equal(t, testNodeID, updated[0].NodeId, "the account must be bound to this node")
	assert.Equal(t, "member", updated[0].Role)
	assert.Equal(t, created[0].Id, updated[0].Id, "the same identity must be carried through")
}

func TestAuthLogin_UsernameMismatchIsRejected(t *testing.T) {
	repo := newFakeAuthRepo()
	repo.owner = domain.Owner{UserId: "existing-id", Username: "alice"}
	users := &fakeUserRepo{}

	as := newService(t, repo, users, &domain.AuthNodeInfo{ID: testNodeID})

	_, err := as.AuthLogin(event.LoginEvent{Username: "mallory", Password: "Claude1234$"}, security.PSK{})
	assert.ErrorIs(t, err, ErrUsernamesMismatch)
	assert.False(t, as.IsAuthenticated())

	created, _ := users.snapshot()
	assert.Empty(t, created, "an existing owner must not be overwritten by a new account")
}

func TestAuthLogin_ReturningOwnerIsNotRecreated(t *testing.T) {
	repo := newFakeAuthRepo()
	repo.owner = domain.Owner{UserId: "existing-id", Username: "alice", CreatedAt: time.Now().Add(-time.Hour)}
	users := &fakeUserRepo{}

	as := newService(t, repo, users, &domain.AuthNodeInfo{ID: testNodeID})

	_, err := as.AuthLogin(event.LoginEvent{Username: "alice", Password: "Claude1234$"}, security.PSK{})
	require.NoError(t, err)

	created, updated := users.snapshot()
	assert.Empty(t, created, "an existing owner must not be created again")
	require.Len(t, updated, 1)
	assert.Equal(t, "existing-id", updated[0].Id)
}

func TestAuthLogin_ReloginKeepsRenamedProfileUsername(t *testing.T) {
	repo := newFakeAuthRepo()
	repo.owner = domain.Owner{UserId: "existing-id", Username: "alice", CreatedAt: time.Now().Add(-time.Hour)}
	users := &fakeUserRepo{}

	as := newService(t, repo, users, &domain.AuthNodeInfo{ID: testNodeID})

	_, err := as.AuthLogin(event.LoginEvent{Username: "alice", Password: "Claude1234$"}, security.PSK{})
	require.NoError(t, err)

	_, updated := users.snapshot()
	require.Len(t, updated, 1)
	// The login refresh must not carry the owner's login name: the user
	// repo's merge-style Update keeps the profile's (possibly renamed)
	// username, which a non-empty value here would clobber.
	assert.Empty(t, updated[0].Username)
}

func TestAuthLogin_OwnerCreationFailureAborts(t *testing.T) {
	repo := newFakeAuthRepo()
	repo.setErr = errors.New("disk full")
	users := &fakeUserRepo{}

	as := newService(t, repo, users, nil)

	_, err := as.AuthLogin(event.LoginEvent{Username: "alice", Password: "Claude1234$"}, security.PSK{})
	assert.Error(t, err)
	assert.False(t, as.IsAuthenticated())
}

func TestAuthLogin_UserCreationFailureAborts(t *testing.T) {
	repo := newFakeAuthRepo()
	users := &fakeUserRepo{createErr: errors.New("storage full")}

	as := newService(t, repo, users, nil)

	_, err := as.AuthLogin(event.LoginEvent{Username: "alice", Password: "Claude1234$"}, security.PSK{})
	assert.Error(t, err)
	assert.False(t, as.IsAuthenticated())
}

func TestAuthLogin_SecondLoginIsRefusedUntilReset(t *testing.T) {
	repo := newFakeAuthRepo()
	users := &fakeUserRepo{}
	as := newService(t, repo, users, &domain.AuthNodeInfo{ID: testNodeID})

	_, err := as.AuthLogin(event.LoginEvent{Username: "alice", Password: "Claude1234$"}, security.PSK{})
	require.NoError(t, err)

	_, err = as.AuthLogin(event.LoginEvent{Username: "alice", Password: "Claude1234$"}, security.PSK{})
	assert.ErrorIs(t, err, ErrAlreadyAuthenticated)

	as.Reset()
	assert.False(t, as.IsAuthenticated())
}

func TestAuthLogin_CancelledStartupAborts(t *testing.T) {
	repo := newFakeAuthRepo()
	ctx, cancel := context.WithCancel(context.Background())

	ready := make(chan domain.AuthNodeInfo)
	as := NewAuthService(ctx, repo, &fakeUserRepo{}, ready)

	go func() {
		<-ready // absorb the request, then never answer
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		_, err := as.AuthLogin(event.LoginEvent{Username: "alice", Password: "Claude1234$"}, security.PSK{})
		done <- err
	}()

	select {
	case err := <-done:
		assert.Error(t, err, "a cancelled startup must abort the login")
		assert.False(t, as.IsAuthenticated())
	case <-time.After(20 * time.Second):
		t.Fatal("login hung after node startup was cancelled")
	}
}

func TestAuthService_AccessorsAndLogout(t *testing.T) {
	repo := newFakeAuthRepo()
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	repo.privKey = priv

	as := newService(t, repo, &fakeUserRepo{}, nil)

	assert.Equal(t, repo, as.Storage())
	assert.Equal(t, priv, as.PrivateKey())
	assert.False(t, as.IsAuthenticated())

	as.AuthLogout()
	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.Equal(t, 1, repo.logoutSeen, "logout must reach the storage layer")
}
