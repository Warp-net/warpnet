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
	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/wallet"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

const (
	orderNonceSize   = 16
	orderLimit       = 10
	orderLimitWindow = time.Hour
	sameDayWindow    = 24 * time.Hour

	ErrOrderLimit warpnet.WarpError = "order: one buyer gets at most 10 of an author's tweets an hour"
)

type SponsoredWallet interface {
	Address(ctx context.Context, seed string) (string, error)
	Pay(ctx context.Context, seed string, s wallet.Sponsorship) (wallet.Payment, error)
	Quote(ctx context.Context, seed string, s wallet.Sponsorship) (wallet.Quote, error)
	IsPaid(ctx context.Context, txId string, s wallet.Sponsorship) (bool, error)
	Splitter() string
	MaxFeePercent() uint64
	Network() string
}

type OrderLister interface {
	ListByBuyer(buyerId string) ([]domain.Order, error)
}

type OrderStorer interface {
	OrderLister
	Get(tweetId, buyerId string) (domain.Order, error)
	Save(o domain.Order) error
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

type SponsoredMediaStorer interface {
	GetImage(userId, key string) (domain.Base64Image, error)
	GetVideo(userId, key string) (domain.Base64Video, error)
	SetCopy(userId, key string, c domain.MediaCopy) error
}

func StreamNewOrderHandler(
	auth WalletOwnerStorer,
	identityKey ed25519.PrivateKey,
	backend SponsoredWallet,
	orders OrderStorer,
	userRepo SponsoredUserFetcher,
	streamer SponsoredStreamer,
) warpnet.WarpHandlerFunc {
	var inFlight sync.Map
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.NewOrderEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("order: empty tweet id")
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("order: empty user id")
		}
		owner := auth.GetOwner()
		if ev.UserId == owner.UserId {
			return nil, warpnet.WarpError("order: an own tweet is not for sale")
		}
		if _, busy := inFlight.LoadOrStore(ev.TweetId, struct{}{}); busy {
			return nil, warpnet.WarpError("order: already in progress")
		}
		defer inFlight.Delete(ev.TweetId)

		author, err := userRepo.Get(ev.UserId)
		if err != nil {
			return nil, err
		}

		order, err := orders.Get(ev.TweetId, owner.UserId)
		if errors.Is(err, database.ErrOrderNotFound) {
			order, err = payAuthor(owner, identityKey, backend, orders, streamer, author, ev.TweetId)
		}
		if err != nil {
			return nil, err
		}
		if !order.Confirmed {
			order, err = claimOrder(orders, streamer, author, order)
			if err != nil {
				return nil, err
			}
		}
		return event.OrderResponse{TweetId: ev.TweetId, TxId: order.TxId, Confirmed: order.Confirmed}, nil
	}
}

func payAuthor(
	owner domain.Owner,
	identityKey ed25519.PrivateKey,
	backend SponsoredWallet,
	orders OrderStorer,
	streamer SponsoredStreamer,
	author domain.User,
	tweetId string,
) (domain.Order, error) {
	sponsorship, err := tweetSponsorship(backend, streamer, author, tweetId)
	if err != nil {
		return domain.Order{}, err
	}

	nonce := make([]byte, orderNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return domain.Order{}, err
	}
	order := domain.Order{
		TweetId:   tweetId,
		AuthorId:  author.Id,
		BuyerId:   owner.UserId,
		Nonce:     hex.EncodeToString(nonce),
		CreatedAt: time.Now(),
	}

	seed, err := walletSeed(owner, identityKey, backend.Network())
	if err != nil {
		return domain.Order{}, err
	}
	sponsorship.OrderId = order.ID()
	payment, err := backend.Pay(context.Background(), seed, sponsorship)
	if err != nil {
		log.Errorf("order: pay for %s: %v", tweetId, err)
		return domain.Order{}, err
	}
	order.TxId = payment.PayTx
	if err := orders.Save(order); err != nil {
		log.Errorf("order: %s paid in tx %s but the receipt was not saved: %v", tweetId, payment.PayTx, err)
	}
	return order, nil
}

func tweetSponsorship(
	backend SponsoredWallet,
	streamer SponsoredStreamer,
	author domain.User,
	tweetId string,
) (wallet.Sponsorship, error) {
	if backend.Splitter() == "" {
		return wallet.Sponsorship{}, warpnet.WarpError("order: sponsored payments are not open on " + backend.Network())
	}

	teaser, err := streamedTweet(streamer, author, tweetId)
	if err != nil {
		return wallet.Sponsorship{}, err
	}
	if !teaser.Price.IsPositive() {
		return wallet.Sponsorship{}, warpnet.WarpError("order: tweet has no price")
	}

	address, err := authorAddress(streamer, author, backend.Network())
	if err != nil {
		return wallet.Sponsorship{}, err
	}
	return wallet.Sponsorship{
		Splitter:      backend.Splitter(),
		Author:        address,
		Amount:        teaser.Price.Units.String(),
		MaxFeePercent: backend.MaxFeePercent(),
	}, nil
}

func StreamGetOrderQuoteHandler(
	auth WalletOwnerStorer,
	identityKey ed25519.PrivateKey,
	backend SponsoredWallet,
	userRepo SponsoredUserFetcher,
	streamer SponsoredStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.GetOrderQuoteEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("order quote: empty tweet id")
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("order quote: empty user id")
		}
		owner := auth.GetOwner()
		if ev.UserId == owner.UserId {
			return nil, warpnet.WarpError("order quote: an own tweet is not for sale")
		}

		author, err := userRepo.Get(ev.UserId)
		if err != nil {
			return nil, err
		}
		sponsorship, err := tweetSponsorship(backend, streamer, author, ev.TweetId)
		if err != nil {
			return nil, err
		}
		seed, err := walletSeed(owner, identityKey, backend.Network())
		if err != nil {
			return nil, err
		}
		quote, err := backend.Quote(context.Background(), seed, sponsorship)
		if err != nil {
			log.Warnf("order quote: %s: %v", ev.TweetId, err)
			return nil, err
		}

		steps := make([]event.OrderQuoteStep, 0, len(quote.Steps))
		for _, step := range quote.Steps {
			steps = append(steps, event.OrderQuoteStep(step))
		}
		return event.OrderQuoteResponse{
			Token:          quote.Token,
			FeePercent:     quote.FeePercent,
			Fee:            quote.Fee,
			Total:          quote.Total,
			Balance:        quote.Balance,
			TRX:            quote.TRX,
			EnergyPrice:    quote.EnergyPrice,
			BandwidthPrice: quote.BandwidthPrice,
			NetworkFee:     quote.NetworkFee,
			Steps:          steps,
		}, nil
	}
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
		return domain.Tweet{}, warpnet.WarpError("order: " + possibleError.Message)
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
	var possibleError event.ResponseError
	if _ = json.Unmarshal(resp, &possibleError); possibleError.Message != "" {
		return "", warpnet.WarpError(possibleError.Message)
	}
	var address event.WalletAddressResponse
	if err := json.Unmarshal(resp, &address); err != nil {
		return "", err
	}
	if address.Address == "" || address.Chain != network || address.UserId != author.Id {
		return "", warpnet.WarpError("order: the author's node gave no wallet address on " + network)
	}
	return address.Address, nil
}

func claimOrder(
	orders OrderStorer,
	streamer SponsoredStreamer,
	author domain.User,
	order domain.Order,
) (domain.Order, error) {
	resp, err := streamer.GenericStream(author.NodeId, event.PUBLIC_POST_SPONSORED_ORDER, event.VerifyOrderEvent{
		TweetId: order.TweetId,
		UserId:  order.BuyerId,
		TxId:    order.TxId,
		Nonce:   order.Nonce,
	})
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		return order, nil
	}
	if err != nil {
		return order, err
	}
	var possibleError event.ResponseError
	if _ = json.Unmarshal(resp, &possibleError); possibleError.Message != "" {
		return order, warpnet.WarpError("order: " + possibleError.Message)
	}
	var verdict event.OrderResponse
	if err := json.Unmarshal(resp, &verdict); err != nil {
		return order, err
	}
	if !verdict.Confirmed {
		return order, nil
	}
	order.Confirmed = true
	return order, orders.Save(order)
}

func StreamVerifyOrderHandler(
	auth WalletOwnerStorer,
	identityKey ed25519.PrivateKey,
	backend SponsoredWallet,
	tweetRepo SponsoredTweetFetcher,
	orders OrderStorer,
	userRepo SponsoredUserFetcher,
	streamer SponsoredStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.VerifyOrderEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("order: empty tweet id")
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("order: empty user id")
		}
		if ev.TxId == "" {
			return nil, warpnet.WarpError("order: empty tx id")
		}
		if ev.Nonce == "" {
			return nil, warpnet.WarpError("order: empty nonce")
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
			return nil, warpnet.WarpError("order: tweet is not sponsored")
		}
		if backend.Splitter() == "" {
			return nil, warpnet.WarpError("order: sponsored payments are not open on " + backend.Network())
		}

		known, err := orders.Get(ev.TweetId, ev.UserId)
		if err == nil && known.Confirmed {
			return event.OrderResponse{TweetId: ev.TweetId, TxId: known.TxId, Confirmed: true}, nil
		}
		isReached, err := isOrderLimitReached(orders, ev.UserId)
		if err != nil {
			return nil, err
		}
		if isReached {
			return event.OrderResponse{TweetId: ev.TweetId, TxId: ev.TxId}, nil
		}

		order := domain.Order{
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
			OrderId:  order.ID(),
			Author:   address,
			Amount:   tweet.Price.Units.String(),
		})
		if err != nil {
			log.Errorf("order: verify %s for %s: %v", ev.TxId, ev.TweetId, err)
			return nil, err
		}
		if paid {
			order.Confirmed = true
			if err := orders.Save(order); err != nil {
				return nil, err
			}
		}
		return event.OrderResponse{TweetId: ev.TweetId, TxId: ev.TxId, Confirmed: paid}, nil
	}
}

func StreamGetSponsoredTweetHandler(
	auth OwnerTweetStorer,
	identityKey ed25519.PrivateKey,
	tweetRepo SponsoredTweetFetcher,
	orders OrderStorer,
	mediaRepo SponsoredMediaStorer,
	userRepo SponsoredUserFetcher,
	streamer SponsoredStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetTweetEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("sponsored tweet: empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("sponsored tweet: empty tweet id")
		}

		owner := auth.GetOwner()
		isOwn := isOwnRequest(s, streamer.NodeInfo())
		if ev.UserId == owner.UserId {
			tweet, err := tweetRepo.Get(owner.UserId, ev.TweetId)
			if err != nil {
				return nil, err
			}
			if isOwn {
				return tweet, nil
			}
			buyer, order, isPaid := paidOrder(s, userRepo, orders, ev.TweetId)
			if !isPaid {
				return domain.Tweet{}, nil
			}
			signer := media_meta.Watermark{PrivKey: identityKey, NodeId: streamer.NodeInfo().ID.String(), OwnerId: owner.UserId}
			return buyerTweet(signer, mediaRepo, tweet, buyer, order)
		}
		if !isOwn {
			return domain.Tweet{}, nil
		}

		order, err := orders.Get(ev.TweetId, owner.UserId)
		if errors.Is(err, database.ErrOrderNotFound) {
			return domain.Tweet{}, nil
		}
		if err != nil {
			return nil, err
		}
		if order.Tweet != nil {
			return *order.Tweet, nil
		}

		author, err := userRepo.Get(ev.UserId)
		if err != nil {
			return nil, err
		}
		if !order.Confirmed {
			order, err = claimOrder(orders, streamer, author, order)
			if err != nil {
				return nil, err
			}
			if !order.Confirmed {
				return event.OrderResponse{TweetId: ev.TweetId, TxId: order.TxId}, nil
			}
		}
		resp, err := streamer.GenericStream(author.NodeId, event.PUBLIC_GET_SPONSORED_TWEET, ev)
		if err != nil {
			return nil, err
		}
		var possibleError event.ResponseError
		if _ = json.Unmarshal(resp, &possibleError); possibleError.Message != "" {
			return nil, warpnet.WarpError(possibleError.Message)
		}
		var tweet domain.Tweet
		if err := json.Unmarshal(resp, &tweet); err != nil {
			return nil, err
		}
		if tweet.Id == "" {
			return domain.Tweet{}, nil
		}
		order.Tweet = &tweet
		if err := orders.Save(order); err != nil {
			log.Warnf("sponsored tweet: caching %s: %v", ev.TweetId, err)
		}
		return tweet, nil
	}
}

func paidOrder(
	s warpnet.WarpStream,
	userRepo SponsoredUserFetcher,
	orders OrderStorer,
	tweetId string,
) (domain.User, domain.Order, bool) {
	if s == nil || s.Conn() == nil {
		return domain.User{}, domain.Order{}, false
	}
	buyer, err := userRepo.GetByNodeID(s.Conn().RemotePeer().String())
	if err != nil {
		return domain.User{}, domain.Order{}, false
	}
	order, err := orders.Get(tweetId, buyer.Id)
	return buyer, order, err == nil && order.Confirmed
}

func buyerTweet(
	signer media_meta.Watermark,
	mediaRepo SponsoredMediaStorer,
	tweet domain.Tweet,
	buyer domain.User,
	order domain.Order,
) (domain.Tweet, error) {
	orderJSON, err := json.Marshal(order)
	if err != nil {
		return domain.Tweet{}, err
	}
	encryptedOrder, err := security.EncryptAES(orderJSON, signer.PrivKey)
	if err != nil {
		return domain.Tweet{}, err
	}

	imageKeys := make([]string, 0, len(tweet.ImageKeys))
	for _, key := range tweet.ImageKeys {
		img, err := mediaRepo.GetImage(tweet.UserId, key)
		if err != nil {
			return domain.Tweet{}, err
		}
		label, err := labelPNG(string(img), buyerLabel(buyer))
		if err != nil {
			return domain.Tweet{}, err
		}
		c := domain.MediaCopy{OriginalKey: key, EncryptedOrder: encryptedOrder, Label: label}
		copyKey, err := newCopy(mediaRepo, tweet.UserId, string(img), c, imageMarker(c, signer))
		if err != nil {
			return domain.Tweet{}, err
		}
		imageKeys = append(imageKeys, copyKey)
	}
	tweet.ImageKeys = imageKeys

	if tweet.VideoKey == nil {
		return tweet, nil
	}
	video, err := mediaRepo.GetVideo(tweet.UserId, *tweet.VideoKey)
	if err != nil {
		return domain.Tweet{}, err
	}
	c := domain.MediaCopy{OriginalKey: *tweet.VideoKey, EncryptedOrder: encryptedOrder}
	copyKey, err := newCopy(mediaRepo, tweet.UserId, string(video), c, media_meta.EmbedOrderInVideo)
	if err != nil {
		return domain.Tweet{}, err
	}
	tweet.VideoKey = &copyKey
	return tweet, nil
}

func newCopy(
	mediaRepo SponsoredMediaStorer,
	userId, file string,
	c domain.MediaCopy,
	mark func(raw, encryptedOrder []byte) ([]byte, error),
) (string, error) {
	copyFile, err := markFile(file, c.EncryptedOrder, mark)
	if err != nil {
		return "", err
	}
	copyKey := contentKey(copyFile)
	return copyKey, mediaRepo.SetCopy(userId, copyKey, c)
}

func countOrders(orders OrderLister, buyerId string, from, to time.Time) (int, error) {
	list, err := orders.ListByBuyer(buyerId)
	if err != nil {
		return 0, err
	}
	var count int
	for _, o := range list {
		if o.Confirmed && !o.CreatedAt.Before(from) && !o.CreatedAt.After(to) {
			count++
		}
	}
	return count, nil
}

func isOrderLimitReached(orders OrderLister, buyerId string) (bool, error) {
	now := time.Now()
	count, err := countOrders(orders, buyerId, now.Add(-orderLimitWindow), now)
	return count >= orderLimit, err
}

func StreamGetCopyBuyerHandler(identityKey ed25519.PrivateKey, orders OrderLister) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.GetCopyBuyerEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		_, raw, err := splitDataURI(ev.File)
		if err != nil {
			return nil, err
		}

		encryptedOrder, err := media_meta.ExtractOrder(raw)
		if errors.Is(err, media_meta.ErrNoMetadata) || errors.Is(err, media_meta.ErrNoOrder) {
			return event.CopyBuyerResponse{}, nil
		}
		if err != nil {
			return nil, err
		}
		orderJSON, err := security.DecryptAES(encryptedOrder, identityKey)
		if err != nil {
			return nil, warpnet.WarpError("sponsored buyer: this node did not sell the copy")
		}

		var order domain.Order
		if err := json.Unmarshal(orderJSON, &order); err != nil {
			return nil, err
		}
		sameDay, err := countOrders(
			orders, order.BuyerId, order.CreatedAt.Add(-sameDayWindow), order.CreatedAt.Add(sameDayWindow),
		)
		if err != nil {
			return nil, err
		}
		return event.CopyBuyerResponse{
			TweetId:       order.TweetId,
			BuyerId:       order.BuyerId,
			OrderId:       order.ID(),
			TxId:          order.TxId,
			SoldAt:        order.CreatedAt,
			SameDayOrders: sameDay,
		}, nil
	}
}
