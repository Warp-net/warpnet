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
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/wallet"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/hashicorp/golang-lru/v2/expirable"
	log "github.com/sirupsen/logrus"
)

type WalletOwnerStorer interface {
	GetOwner() domain.Owner
}

const walletDerivation = "warpnet-account"

type WalletBackend interface {
	Address(ctx context.Context, seed string) (string, error)
	Balance(ctx context.Context, address string) (wallet.Account, error)
	Transfer(ctx context.Context, seed, asset, to, amount string) (string, error)
	Export(ctx context.Context, seed string) (address, privateKey string, err error)
	History(ctx context.Context, address, asset string, limit int) ([]wallet.Transfer, error)
	Token() string
	Decimals() uint8
	Network() string
}

func StreamGetWalletHandler(auth WalletOwnerStorer, identityKey ed25519.PrivateKey, backend WalletBackend) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		seed, err := walletSeed(auth.GetOwner(), identityKey, backend.Network())
		if err != nil {
			return nil, err
		}
		ctx := context.Background()
		address, err := backend.Address(ctx, seed)
		if err != nil {
			log.Errorf("wallet: address: %v", err)
			return nil, err
		}
		account, err := backend.Balance(ctx, address)
		if err != nil {
			log.Errorf("wallet: balance: %v", err)
			return nil, err
		}
		if account.Activated {
			log.Infof(
				"wallet: reusing the existing account %s on %s, on chain since %s, re-derived from the owner identity key",
				address, backend.Network(), time.UnixMilli(account.CreatedAt).UTC().Format(time.RFC3339),
			)
		} else {
			log.Infof(
				"wallet: derived a new account %s on %s from the owner identity key, not activated on chain yet",
				address, backend.Network(),
			)
		}
		return event.WalletResponse{
			Address:     address,
			Token:       backend.Token(),
			UsdtBalance: account.TokenBalance,
			TrxBalance:  account.TRX,
			Activated:   account.Activated,
			CreatedAt:   account.CreatedAt,
			Derivation:  walletDerivation,
			Decimals:    backend.Decimals(),
			Network:     backend.Network(),
		}, nil
	}
}

func StreamGetOwnWalletAddressHandler(auth WalletOwnerStorer, identityKey ed25519.PrivateKey, backend WalletBackend) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		seed, err := walletSeed(auth.GetOwner(), identityKey, backend.Network())
		if err != nil {
			return nil, err
		}
		address, err := backend.Address(context.Background(), seed)
		if err != nil {
			log.Errorf("wallet: own address: %v", err)
			return nil, err
		}
		return event.WalletOwnAddressResponse{
			Address:  address,
			Token:    backend.Token(),
			Decimals: backend.Decimals(),
			Network:  backend.Network(),
		}, nil
	}
}

func StreamWalletSendHandler(auth WalletOwnerStorer, identityKey ed25519.PrivateKey, backend WalletBackend) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.WalletSendEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.To == "" {
			return nil, warpnet.WarpError("wallet: empty recipient")
		}
		if ev.Amount == "" {
			return nil, warpnet.WarpError("wallet: empty amount")
		}
		asset, err := walletAsset(ev.Asset, backend)
		if err != nil {
			return nil, err
		}
		seed, err := walletSeed(auth.GetOwner(), identityKey, backend.Network())
		if err != nil {
			return nil, err
		}
		tx, err := backend.Transfer(context.Background(), seed, asset, ev.To, ev.Amount)
		if err != nil {
			log.Errorf("wallet: transfer: %v", err)
			return nil, warpnet.WarpError("wallet: " + err.Error())
		}
		return event.WalletSendResponse{Tx: tx, Asset: asset, To: ev.To, Amount: ev.Amount}, nil
	}
}

func StreamGetWalletHistoryHandler(auth WalletOwnerStorer, identityKey ed25519.PrivateKey, backend WalletBackend) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.WalletEvent
		if len(buf) > 0 {
			_ = json.Unmarshal(buf, &ev)
		}
		seed, err := walletSeed(auth.GetOwner(), identityKey, backend.Network())
		if err != nil {
			return nil, err
		}
		ctx := context.Background()
		address, err := backend.Address(ctx, seed)
		if err != nil {
			log.Errorf("wallet: address: %v", err)
			return nil, err
		}
		asset, err := walletAsset(ev.Asset, backend)
		if err != nil {
			return nil, err
		}
		transfers, err := backend.History(ctx, address, asset, ev.Limit)
		if err != nil {
			log.Errorf("wallet: history: %v", err)
			return nil, err
		}
		items := make([]event.WalletHistoryItem, 0, len(transfers))
		for _, t := range transfers {
			items = append(items, event.WalletHistoryItem{
				Tx:        t.Tx,
				Asset:     t.Asset,
				From:      t.From,
				To:        t.To,
				Value:     t.Value,
				Timestamp: t.Timestamp,
				Incoming:  t.Incoming,
			})
		}
		return event.WalletHistoryResponse{Transfers: items}, nil
	}
}

func StreamGetWalletKeyHandler(auth WalletOwnerStorer, identityKey ed25519.PrivateKey, backend WalletBackend) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		seed, err := walletSeed(auth.GetOwner(), identityKey, backend.Network())
		if err != nil {
			return nil, err
		}
		address, privateKey, err := backend.Export(context.Background(), seed)
		if err != nil {
			log.Errorf("wallet: export: %v", err)
			return nil, err
		}
		return event.WalletKeyResponse{Address: address, PrivateKey: privateKey}, nil
	}
}

const (
	walletContactsFanout = 8
	walletContactsLimit  = 50
	walletContactsWait   = 1200 * time.Millisecond
	walletContactsRetry  = 10 * time.Minute
	walletContactsHolds  = 256
)

type WalletAddressStorer interface {
	SetAddress(chain, userId, address string) error
	ListAddresses(chain string, limit *uint64, cursor *string) ([]domain.WalletAddress, string, error)
}

type WalletFollowLister interface {
	GetFollowers(userId string, limit *uint64, cursor *string) ([]string, string, error)
	GetFollowings(userId string, limit *uint64, cursor *string) ([]string, string, error)
}

type WalletUserFetcher interface {
	Get(userId string) (domain.User, error)
}

type WalletStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

func StreamGetWalletAddressHandler(auth WalletOwnerStorer, identityKey ed25519.PrivateKey, backend WalletBackend) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		owner := auth.GetOwner()
		seed, err := walletSeed(owner, identityKey, backend.Network())
		if err != nil {
			return nil, err
		}
		address, err := backend.Address(context.Background(), seed)
		if err != nil {
			log.Errorf("wallet: address for a peer: %v", err)
			return nil, err
		}
		return event.WalletAddressResponse{
			Address: address,
			Chain:   backend.Network(),
			UserId:  owner.UserId,
		}, nil
	}
}

func StreamGetWalletContactsHandler(
	auth WalletOwnerStorer,
	identityKey ed25519.PrivateKey,
	backend WalletBackend,
	wallets WalletAddressStorer,
	follows WalletFollowLister,
	users WalletUserFetcher,
	streamer WalletStreamer,
) warpnet.WarpHandlerFunc {
	refresher := newWalletContactsRefresher()
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.WalletContactsEvent
		if len(buf) > 0 {
			_ = json.Unmarshal(buf, &ev)
		}
		owner := auth.GetOwner()
		seed, err := walletSeed(owner, identityKey, backend.Network())
		if err != nil {
			return nil, err
		}
		chain := backend.Network()
		address, err := backend.Address(context.Background(), seed)
		if err != nil {
			log.Errorf("wallet: contacts: own address: %v", err)
			return nil, err
		}
		if ev.Force {
			refresher.clearHolds()
		}
		if err := wallets.SetAddress(chain, owner.UserId, address); err != nil {
			log.Warnf("wallet: contacts: storing the own address failed: %v", err)
		}

		select {
		case <-refresher.start(chain, owner, wallets, follows, users, streamer):
		case <-time.After(walletContactsWait):
		}

		stored, _, err := wallets.ListAddresses(chain, nil, nil)
		if err != nil {
			log.Errorf("wallet: contacts: listing addresses: %v", err)
			return nil, err
		}
		contacts := make([]event.WalletContact, 0, len(stored))
		for _, item := range stored {
			if item.UserId == owner.UserId || item.Address == "" {
				continue
			}
			contact := event.WalletContact{Address: item.Address, UserId: item.UserId}
			if users != nil {
				if user, err := users.Get(item.UserId); err == nil {
					contact.Username = user.Username
					contact.AvatarKey = user.AvatarKey
				}
			}
			contacts = append(contacts, contact)
		}
		return event.WalletContactsResponse{Contacts: contacts}, nil
	}
}

type walletContactsRefresher struct {
	running     atomic.Bool
	unreachable *expirable.LRU[string, struct{}]
}

func newWalletContactsRefresher() *walletContactsRefresher {
	return &walletContactsRefresher{
		unreachable: expirable.NewLRU[string, struct{}](walletContactsHolds, nil, walletContactsRetry),
	}
}

func (r *walletContactsRefresher) start(
	chain string,
	owner domain.Owner,
	wallets WalletAddressStorer,
	follows WalletFollowLister,
	users WalletUserFetcher,
	streamer WalletStreamer,
) <-chan struct{} {
	done := make(chan struct{})
	if !r.running.CompareAndSwap(false, true) {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		defer r.running.Store(false)
		r.refresh(chain, owner, wallets, follows, users, streamer)
	}()
	return done
}

func (r *walletContactsRefresher) clearHolds() {
	r.unreachable.Purge()
}

func (r *walletContactsRefresher) skipped(peerId string) bool {
	_, held := r.unreachable.Get(peerId)
	return held
}

func (r *walletContactsRefresher) hold(peerId string) {
	r.unreachable.Add(peerId, struct{}{})
}

func (r *walletContactsRefresher) refresh(
	chain string,
	owner domain.Owner,
	wallets WalletAddressStorer,
	follows WalletFollowLister,
	users WalletUserFetcher,
	streamer WalletStreamer,
) {
	if follows == nil || users == nil || streamer == nil {
		return
	}
	limit := uint64(walletContactsLimit)
	followings, _, err := follows.GetFollowings(owner.UserId, &limit, nil)
	if err != nil {
		log.Warnf("wallet: contacts: followings: %v", err)
	}
	followers, _, err := follows.GetFollowers(owner.UserId, &limit, nil)
	if err != nil {
		log.Warnf("wallet: contacts: followers: %v", err)
	}

	seen := map[string]bool{owner.UserId: true}
	peers := make([]string, 0, len(followings)+len(followers))
	for _, id := range append(followings, followers...) {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		peers = append(peers, id)
	}
	if len(peers) == 0 {
		return
	}

	ownNode := streamer.NodeInfo()
	sem := make(chan struct{}, walletContactsFanout)
	var wg sync.WaitGroup
	for _, peerId := range peers {
		wg.Add(1)
		go func(peerId string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if r.skipped(peerId) {
				return
			}
			address, ok := fetchWalletAddress(chain, peerId, ownNode, users, streamer)
			if !ok {
				r.hold(peerId)
				return
			}
			if err := wallets.SetAddress(chain, peerId, address); err != nil {
				log.Warnf("wallet: contacts: storing the address of %s failed: %v", peerId, err)
			}
		}(peerId)
	}
	wg.Wait()
}

func fetchWalletAddress(
	chain, peerId string,
	ownNode warpnet.NodeInfo,
	users WalletUserFetcher,
	streamer WalletStreamer,
) (string, bool) {
	user, err := users.Get(peerId)
	if err != nil || user.NodeId == "" || user.NodeId == ownNode.ID.String() {
		return "", false
	}
	resp, err := streamer.GenericStream(user.NodeId, event.PUBLIC_GET_WALLET_ADDRESS, event.WalletAddressEvent{Chain: chain})
	if err != nil {
		if !errors.Is(err, warpnet.ErrNodeIsOffline) {
			log.Warnf("wallet: contacts: asking %s for an address: %v", peerId, err)
		}
		return "", false
	}
	var out event.WalletAddressResponse
	if err := json.Unmarshal(resp, &out); err != nil {
		return "", false
	}
	if out.Address == "" || out.Chain != chain {
		return "", false
	}
	return out.Address, true
}

func walletAsset(asset string, backend WalletBackend) (string, error) {
	asset = strings.TrimSpace(asset)
	if asset == "" {
		return backend.Token(), nil
	}
	for _, known := range []string{backend.Token(), wallet.NativeCoin} {
		if strings.EqualFold(asset, known) {
			return known, nil
		}
	}
	return "", warpnet.WarpError("wallet: unknown asset " + asset)
}

func walletSeed(owner domain.Owner, identityKey ed25519.PrivateKey, network string) (string, error) {
	if owner.Username == "" {
		return "", warpnet.WarpError("wallet: no owner in session")
	}
	if len(identityKey) == 0 {
		return "", warpnet.WarpError("wallet: no identity key in session")
	}
	raw := security.DeriveWalletSeed(identityKey, network, owner.Username)
	defer security.Wipe(raw)
	return hex.EncodeToString(raw), nil
}
