//nolint:all
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

package handler

import (
	"context"
	"crypto/ed25519"
	"github.com/Warp-net/warpnet/core/stream"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/wallet"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

type stubWalletOwner struct{ owner domain.Owner }

func (s stubWalletOwner) GetOwner() domain.Owner { return s.owner }

type stubWalletBackend struct {
	addressFn  func(string) (string, error)
	balanceFn  func(string) (wallet.Account, error)
	transferFn func(string, string, string) (string, error)
	exportFn   func(string) (string, string, error)
	historyFn  func(string, int) ([]wallet.Transfer, error)
}

func (s stubWalletBackend) Address(_ context.Context, seed string) (string, error) {
	return s.addressFn(seed)
}
func (s stubWalletBackend) Balance(_ context.Context, addr string) (wallet.Account, error) {
	return s.balanceFn(addr)
}
func (s stubWalletBackend) Transfer(_ context.Context, seed, to, amount string) (string, error) {
	return s.transferFn(seed, to, amount)
}
func (s stubWalletBackend) Export(_ context.Context, seed string) (string, string, error) {
	return s.exportFn(seed)
}
func (s stubWalletBackend) History(_ context.Context, addr string, limit int) ([]wallet.Transfer, error) {
	return s.historyFn(addr, limit)
}
func (s stubWalletBackend) Token() string   { return "TXYZ" }
func (s stubWalletBackend) Decimals() uint8 { return 6 }
func (s stubWalletBackend) Network() string { return "testnet" }

func walletKey() ed25519.PrivateKey { return make(ed25519.PrivateKey, ed25519.PrivateKeySize) }

func ownerAuth() stubWalletOwner {
	return stubWalletOwner{owner: domain.Owner{Username: "alice", UserId: "u1"}}
}

func marshalWallet(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestGetWalletHandler(t *testing.T) {
	backend := stubWalletBackend{
		addressFn: func(seed string) (string, error) {
			if seed == "" {
				t.Fatal("empty seed")
			}
			return "TAddr", nil
		},
		balanceFn: func(addr string) (wallet.Account, error) {
			if addr != "TAddr" {
				t.Fatalf("balance for %s", addr)
			}
			return wallet.Account{TokenBalance: "1500000", TRX: "25000000", Activated: true, CreatedAt: 1757000000000}, nil
		},
	}
	out, err := StreamGetWalletHandler(ownerAuth(), walletKey(), backend)([]byte(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := out.(event.WalletResponse)
	if resp.Address != "TAddr" || resp.UsdtBalance != "1500000" || resp.TrxBalance != "25000000" || resp.Decimals != 6 || resp.Token != "TXYZ" || resp.Network != "testnet" {
		t.Fatalf("resp = %+v", resp)
	}
	if !resp.Activated || resp.CreatedAt != 1757000000000 || resp.Derivation != walletDerivation {
		t.Fatalf("an on-chain account must be reported as existing: %+v", resp)
	}
}

func TestGetWalletHandlerReportsFreshAccount(t *testing.T) {
	backend := stubWalletBackend{
		addressFn: func(seed string) (string, error) { return "TFresh", nil },
		balanceFn: func(addr string) (wallet.Account, error) {
			return wallet.Account{TokenBalance: "0", TRX: "0"}, nil
		},
	}
	out, err := StreamGetWalletHandler(ownerAuth(), walletKey(), backend)([]byte(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := out.(event.WalletResponse)
	if resp.Activated || resp.CreatedAt != 0 {
		t.Fatalf("an address with no chain presence must not be reported as existing: %+v", resp)
	}
	if resp.Derivation != walletDerivation {
		t.Fatalf("the response must say how the wallet was derived: %+v", resp)
	}
}

func TestGetWalletHandlerNoOwner(t *testing.T) {
	_, err := StreamGetWalletHandler(stubWalletOwner{}, walletKey(), stubWalletBackend{})([]byte(`{}`), nil)
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("err = %v", err)
	}
}

func TestGetWalletHandlerNoKey(t *testing.T) {
	_, err := StreamGetWalletHandler(ownerAuth(), nil, stubWalletBackend{})([]byte(`{}`), nil)
	if err == nil || !strings.Contains(err.Error(), "identity key") {
		t.Fatalf("err = %v", err)
	}
}

func TestGetOwnWalletAddressHandler(t *testing.T) {
	var balanceCalls int
	backend := stubWalletBackend{
		addressFn: func(string) (string, error) { return "TAddr", nil },
		balanceFn: func(string) (wallet.Account, error) {
			balanceCalls++
			return wallet.Account{}, nil
		},
	}
	out, err := StreamGetOwnWalletAddressHandler(ownerAuth(), walletKey(), backend)(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := out.(event.WalletOwnAddressResponse)
	if resp.Address != "TAddr" || resp.Token != "TXYZ" || resp.Decimals != 6 || resp.Network != "testnet" {
		t.Fatalf("resp = %+v", resp)
	}
	if balanceCalls != 0 {
		t.Fatalf("balance called %d times: the address route must not touch the chain", balanceCalls)
	}
}

func TestGetOwnWalletAddressHandlerNoOwner(t *testing.T) {
	backend := stubWalletBackend{addressFn: func(string) (string, error) { return "TAddr", nil }}
	auth := stubWalletOwner{owner: domain.Owner{}}
	if _, err := StreamGetOwnWalletAddressHandler(auth, walletKey(), backend)(nil, nil); err == nil {
		t.Fatal("expected an error without an owner")
	}
}

func TestGetOwnWalletAddressHandlerBackendFailure(t *testing.T) {
	backend := stubWalletBackend{
		addressFn: func(string) (string, error) { return "", warpnet.WarpError("engine unavailable") },
	}
	if _, err := StreamGetOwnWalletAddressHandler(ownerAuth(), walletKey(), backend)(nil, nil); err == nil {
		t.Fatal("expected the backend error to surface")
	}
}

func TestWalletSendHandler(t *testing.T) {
	var gotTo, gotAmount string
	backend := stubWalletBackend{
		addressFn: func(string) (string, error) { return "TAddr", nil },
		transferFn: func(_, to, amount string) (string, error) {
			gotTo, gotAmount = to, amount
			return "0xtx", nil
		},
	}
	body := marshalWallet(t, event.WalletSendEvent{To: "TGf63ryqEJdybz5S23U2sBcFbW8uAsWSPS", Amount: "1000000"})
	out, err := StreamWalletSendHandler(ownerAuth(), walletKey(), backend)(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := out.(event.WalletSendResponse)
	if resp.Tx != "0xtx" || gotTo != "TGf63ryqEJdybz5S23U2sBcFbW8uAsWSPS" || gotAmount != "1000000" {
		t.Fatalf("resp = %+v, sent to %s amount %s", resp, gotTo, gotAmount)
	}
}

func TestWalletSendHandlerValidation(t *testing.T) {
	backend := stubWalletBackend{addressFn: func(string) (string, error) { return "TAddr", nil }}
	cases := []event.WalletSendEvent{
		{To: "", Amount: "1"},
		{To: "TGf63ryqEJdybz5S23U2sBcFbW8uAsWSPS", Amount: ""},
	}
	for _, c := range cases {
		if _, err := StreamWalletSendHandler(ownerAuth(), walletKey(), backend)(marshalWallet(t, c), nil); err == nil {
			t.Fatalf("expected error for %+v", c)
		}
	}
	if _, err := StreamWalletSendHandler(ownerAuth(), walletKey(), backend)([]byte("not json"), nil); err == nil {
		t.Fatal("expected error on bad payload")
	}
}

func TestWalletHistoryHandler(t *testing.T) {
	backend := stubWalletBackend{
		addressFn: func(string) (string, error) { return "TAddr", nil },
		historyFn: func(addr string, limit int) ([]wallet.Transfer, error) {
			if addr != "TAddr" {
				t.Fatalf("history for %s", addr)
			}
			return []wallet.Transfer{{Tx: "a", From: "TFrom", To: "TAddr", Value: "5", Incoming: true}}, nil
		},
	}
	out, err := StreamGetWalletHistoryHandler(ownerAuth(), walletKey(), backend)(marshalWallet(t, event.WalletEvent{Limit: 10}), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := out.(event.WalletHistoryResponse)
	if len(resp.Transfers) != 1 || !resp.Transfers[0].Incoming || resp.Transfers[0].Value != "5" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestWalletKeyHandler(t *testing.T) {
	backend := stubWalletBackend{
		exportFn: func(seed string) (string, string, error) {
			if seed == "" {
				t.Fatal("empty seed")
			}
			return "TAddr", "deadbeef", nil
		},
	}
	out, err := StreamGetWalletKeyHandler(ownerAuth(), walletKey(), backend)([]byte(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := out.(event.WalletKeyResponse)
	if resp.Address != "TAddr" || resp.PrivateKey != "deadbeef" {
		t.Fatalf("resp = %+v", resp)
	}
}

type stubWalletAddresses struct {
	mu     sync.Mutex
	stored map[string]domain.WalletAddress
}

func newStubWalletAddresses() *stubWalletAddresses {
	return &stubWalletAddresses{stored: map[string]domain.WalletAddress{}}
}

func (s *stubWalletAddresses) SetAddress(chain, userId, address string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stored[chain+"/"+userId] = domain.WalletAddress{Address: address, Chain: chain, UserId: userId}
	return nil
}

func (s *stubWalletAddresses) ListAddresses(chain string, _ *uint64, _ *string) ([]domain.WalletAddress, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.WalletAddress, 0, len(s.stored))
	for _, item := range s.stored {
		if item.Chain == chain {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserId < out[j].UserId })
	return out, "", nil
}

type stubWalletFollows struct {
	followings []string
	followers  []string
}

func (s stubWalletFollows) GetFollowings(string, *uint64, *string) ([]string, string, error) {
	return s.followings, "", nil
}
func (s stubWalletFollows) GetFollowers(string, *uint64, *string) ([]string, string, error) {
	return s.followers, "", nil
}

type stubWalletUsers struct{ users map[string]domain.User }

func (s stubWalletUsers) Get(userId string) (domain.User, error) {
	user, ok := s.users[userId]
	if !ok {
		return domain.User{}, warpnet.WarpError("no user")
	}
	return user, nil
}

type stubWalletPeerStreamer struct {
	mu        sync.Mutex
	responses map[string][]byte
	errs      map[string]error
	asked     []string
}

func (s *stubWalletPeerStreamer) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{OwnerId: "owner-1"}
}

func (s *stubWalletPeerStreamer) GenericStream(nodeId string, _ stream.WarpRoute, _ any) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked = append(s.asked, nodeId)
	if err, ok := s.errs[nodeId]; ok {
		return nil, err
	}
	return s.responses[nodeId], nil
}

func walletContactsBackend() stubWalletBackend {
	return stubWalletBackend{
		addressFn: func(string) (string, error) { return "TOwnAddress", nil },
	}
}

func TestGetWalletAddressHandler(t *testing.T) {
	out, err := StreamGetWalletAddressHandler(ownerAuth(), walletKey(), walletContactsBackend())([]byte(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := out.(event.WalletAddressResponse)
	if resp.Address != "TOwnAddress" || resp.Chain != "testnet" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.UserId == "" {
		t.Fatal("a peer must learn which user the address belongs to")
	}
}

func TestGetWalletContactsHandler(t *testing.T) {
	peerResp, _ := json.Marshal(event.WalletAddressResponse{Address: "TPeerAddress", Chain: "testnet", UserId: "peer-1"})
	streamer := &stubWalletPeerStreamer{
		responses: map[string][]byte{"peer-node-1": peerResp},
		errs:      map[string]error{"peer-node-2": warpnet.ErrNodeIsOffline},
	}
	users := stubWalletUsers{users: map[string]domain.User{
		"peer-1": {Id: "peer-1", NodeId: "peer-node-1", Username: "alice", AvatarKey: "avatar-1"},
		"peer-2": {Id: "peer-2", NodeId: "peer-node-2", Username: "bob"},
	}}
	store := newStubWalletAddresses()
	follows := stubWalletFollows{followings: []string{"peer-1"}, followers: []string{"peer-2"}}

	out, err := StreamGetWalletContactsHandler(
		ownerAuth(), walletKey(), walletContactsBackend(), store, follows, users, streamer,
	)([]byte(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := out.(event.WalletContactsResponse)
	if len(resp.Contacts) != 1 {
		t.Fatalf("contacts = %+v, want only the peer that answered", resp.Contacts)
	}
	if resp.Contacts[0].Address != "TPeerAddress" || resp.Contacts[0].Username != "alice" {
		t.Fatalf("contact = %+v", resp.Contacts[0])
	}
	if resp.Contacts[0].UserId != "peer-1" || resp.Contacts[0].AvatarKey != "avatar-1" {
		t.Fatalf("contact = %+v, want the id and avatar key the picker renders", resp.Contacts[0])
	}
	stored, _, _ := store.ListAddresses("testnet", nil, nil)
	if len(stored) != 2 {
		t.Fatalf("stored = %+v, want the owner's own address plus the peer's", stored)
	}
	for _, item := range stored {
		if item.UserId == resp.Contacts[0].UserId {
			continue
		}
		if item.Address != "TOwnAddress" {
			t.Fatalf("the owner's own address must be published to the repo: %+v", item)
		}
	}
}

func TestGetWalletContactsExcludesTheOwner(t *testing.T) {
	store := newStubWalletAddresses()
	out, err := StreamGetWalletContactsHandler(
		ownerAuth(), walletKey(), walletContactsBackend(), store,
		stubWalletFollows{}, stubWalletUsers{}, &stubWalletPeerStreamer{},
	)([]byte(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := out.(event.WalletContactsResponse)
	if len(resp.Contacts) != 0 {
		t.Fatalf("contacts = %+v, want none: you cannot pay yourself", resp.Contacts)
	}
}
