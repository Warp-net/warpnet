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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/authorship"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/wallet"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const purchaseNonceSize = 16

type SponsoredWallet interface {
	Address(ctx context.Context, seed string) (string, error)
	Pay(ctx context.Context, seed string, s wallet.Sponsorship) (wallet.Payment, error)
	IsPaid(ctx context.Context, txId string, s wallet.Sponsorship) (bool, error)
	Splitter() string
	MaxFeePercent() uint64
	Network() string
}

type PurchaseStorer interface {
	Get(tweetId, buyerId string) (domain.Purchase, error)
	Save(p domain.Purchase) error
}

type SponsoredTweetFetcher interface {
	Get(userID, tweetID string) (domain.Tweet, error)
}

type SponsoredUserFetcher interface {
	Get(userId string) (domain.User, error)
	Create(user domain.User) (domain.User, error)
	GetByNodeID(nodeId string) (domain.User, error)
}

type SponsoredStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

func StreamNewPurchaseHandler(
	auth WalletOwnerStorer,
	identityKey ed25519.PrivateKey,
	backend SponsoredWallet,
	purchases PurchaseStorer,
	userRepo SponsoredUserFetcher,
	streamer SponsoredStreamer,
) warpnet.WarpHandlerFunc {
	var inFlight sync.Map
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.NewPurchaseEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("purchase: empty tweet id")
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("purchase: empty user id")
		}
		owner := auth.GetOwner()
		if ev.UserId == owner.UserId {
			return nil, warpnet.WarpError("purchase: an own tweet is not for sale")
		}
		if _, busy := inFlight.LoadOrStore(ev.TweetId, struct{}{}); busy {
			return nil, warpnet.WarpError("purchase: already in progress")
		}
		defer inFlight.Delete(ev.TweetId)

		author, err := userRepo.Get(ev.UserId)
		if err != nil {
			return nil, err
		}

		purchase, err := purchases.Get(ev.TweetId, owner.UserId)
		if errors.Is(err, database.ErrPurchaseNotFound) {
			purchase, err = payAuthor(owner, identityKey, backend, purchases, streamer, author, ev.TweetId)
		}
		if err != nil {
			return nil, err
		}
		if !purchase.Confirmed {
			purchase, err = claimPurchase(purchases, streamer, author, purchase)
			if err != nil {
				return nil, err
			}
		}
		return event.PurchaseResponse{TweetId: ev.TweetId, TxId: purchase.TxId, Confirmed: purchase.Confirmed}, nil
	}
}

func payAuthor(
	owner domain.Owner,
	identityKey ed25519.PrivateKey,
	backend SponsoredWallet,
	purchases PurchaseStorer,
	streamer SponsoredStreamer,
	author domain.User,
	tweetId string,
) (domain.Purchase, error) {
	if backend.Splitter() == "" {
		return domain.Purchase{}, warpnet.WarpError("purchase: sponsored payments are not open on " + backend.Network())
	}

	teaser, err := streamedTweet(streamer, author, tweetId)
	if err != nil {
		return domain.Purchase{}, err
	}
	if !teaser.Price.IsPositive() {
		return domain.Purchase{}, warpnet.WarpError("purchase: tweet has no price")
	}

	address, err := authorAddress(streamer, author, backend.Network())
	if err != nil {
		return domain.Purchase{}, err
	}

	nonce := make([]byte, purchaseNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return domain.Purchase{}, err
	}
	purchase := domain.Purchase{
		TweetId:   tweetId,
		AuthorId:  author.Id,
		BuyerId:   owner.UserId,
		Nonce:     hex.EncodeToString(nonce),
		CreatedAt: time.Now(),
	}

	seed, err := walletSeed(owner, identityKey, backend.Network())
	if err != nil {
		return domain.Purchase{}, err
	}
	payment, err := backend.Pay(context.Background(), seed, wallet.Sponsorship{
		Splitter:      backend.Splitter(),
		OrderId:       purchase.OrderId(),
		Author:        address,
		Amount:        teaser.Price.Units.String(),
		MaxFeePercent: backend.MaxFeePercent(),
	})
	if err != nil {
		log.Errorf("purchase: pay for %s: %v", tweetId, err)
		return domain.Purchase{}, err
	}
	purchase.TxId = payment.PayTx
	if err := purchases.Save(purchase); err != nil {
		log.Errorf("purchase: %s paid in tx %s but the receipt was not saved: %v", tweetId, payment.PayTx, err)
	}
	return purchase, nil
}

func streamedTweet(streamer SponsoredStreamer, author domain.User, tweetId string) (domain.Tweet, error) {
	resp, err := streamer.GenericStream(author.NodeId, event.PUBLIC_GET_TWEET, event.GetTweetEvent{
		TweetId: tweetId,
		UserId:  author.Id,
	})
	if err != nil {
		return domain.Tweet{}, err
	}
	var possibleError event.ResponseError
	if _ = json.Unmarshal(resp, &possibleError); possibleError.Message != "" {
		return domain.Tweet{}, warpnet.WarpError("purchase: " + possibleError.Message)
	}
	var tweet domain.Tweet
	if err := json.Unmarshal(resp, &tweet); err != nil {
		return domain.Tweet{}, err
	}
	return tweet, nil
}

func authorAddress(streamer SponsoredStreamer, author domain.User, network string) (string, error) {
	resp, err := streamer.GenericStream(author.NodeId, event.PUBLIC_GET_WALLET_ADDRESS, event.WalletAddressEvent{Chain: network})
	if err != nil {
		return "", err
	}
	var address event.WalletAddressResponse
	if err := json.Unmarshal(resp, &address); err != nil {
		return "", err
	}
	if address.Address == "" || address.Chain != network || address.UserId != author.Id {
		return "", warpnet.WarpError("purchase: the author's node gave no wallet address on " + network)
	}
	return address.Address, nil
}

func claimPurchase(
	purchases PurchaseStorer,
	streamer SponsoredStreamer,
	author domain.User,
	purchase domain.Purchase,
) (domain.Purchase, error) {
	resp, err := streamer.GenericStream(author.NodeId, event.PUBLIC_POST_SPONSORED_PURCHASE, event.VerifyPurchaseEvent{
		TweetId: purchase.TweetId,
		UserId:  purchase.BuyerId,
		TxId:    purchase.TxId,
		Nonce:   purchase.Nonce,
	})
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		return purchase, nil
	}
	if err != nil {
		return purchase, err
	}
	var possibleError event.ResponseError
	if _ = json.Unmarshal(resp, &possibleError); possibleError.Message != "" {
		return purchase, warpnet.WarpError("purchase: " + possibleError.Message)
	}
	var verdict event.PurchaseResponse
	if err := json.Unmarshal(resp, &verdict); err != nil {
		return purchase, err
	}
	if !verdict.Confirmed {
		return purchase, nil
	}
	purchase.Confirmed = true
	return purchase, purchases.Save(purchase)
}

func StreamVerifyPurchaseHandler(
	auth WalletOwnerStorer,
	identityKey ed25519.PrivateKey,
	backend SponsoredWallet,
	tweetRepo SponsoredTweetFetcher,
	purchases PurchaseStorer,
	userRepo SponsoredUserFetcher,
	streamer SponsoredStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.VerifyPurchaseEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("purchase: empty tweet id")
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("purchase: empty user id")
		}
		if ev.TxId == "" {
			return nil, warpnet.WarpError("purchase: empty tx id")
		}
		if ev.Nonce == "" {
			return nil, warpnet.WarpError("purchase: empty nonce")
		}
		if _, err := authorship.VerifyActor(userRepo, streamer, s, ev.UserId); err != nil {
			return nil, err
		}

		owner := auth.GetOwner()
		tweet, err := tweetRepo.Get(owner.UserId, ev.TweetId)
		if err != nil {
			return nil, err
		}
		if !tweet.IsSponsored() {
			return nil, warpnet.WarpError("purchase: tweet is not sponsored")
		}
		if backend.Splitter() == "" {
			return nil, warpnet.WarpError("purchase: sponsored payments are not open on " + backend.Network())
		}

		known, err := purchases.Get(ev.TweetId, ev.UserId)
		if err == nil && known.Confirmed {
			return event.PurchaseResponse{TweetId: ev.TweetId, TxId: known.TxId, Confirmed: true}, nil
		}

		purchase := domain.Purchase{
			TweetId:   ev.TweetId,
			AuthorId:  owner.UserId,
			BuyerId:   ev.UserId,
			Nonce:     ev.Nonce,
			TxId:      ev.TxId,
			CreatedAt: time.Now(),
		}
		seed, err := walletSeed(owner, identityKey, backend.Network())
		if err != nil {
			return nil, err
		}
		ctx := context.Background()
		address, err := backend.Address(ctx, seed)
		if err != nil {
			return nil, err
		}
		paid, err := backend.IsPaid(ctx, ev.TxId, wallet.Sponsorship{
			Splitter: backend.Splitter(),
			OrderId:  purchase.OrderId(),
			Author:   address,
			Amount:   tweet.Price.Units.String(),
		})
		if err != nil {
			log.Errorf("purchase: verify %s for %s: %v", ev.TxId, ev.TweetId, err)
			return nil, err
		}
		if paid {
			purchase.Confirmed = true
			if err := purchases.Save(purchase); err != nil {
				return nil, err
			}
		}
		return event.PurchaseResponse{TweetId: ev.TweetId, TxId: ev.TxId, Confirmed: paid}, nil
	}
}
