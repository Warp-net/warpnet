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
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
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
	orderNonceSize    = 16
	dailyOrdersWindow = 24 * time.Hour
	// A payment sent this long ago has landed or never will: its transaction
	// expires a minute after it is signed.
	paymentLifetime = 3 * time.Minute
	quoteLifetime   = 30 * time.Minute

	ErrChainUnavailable warpnet.WarpError = "order: the TRON network cannot be reached, nothing was paid; try again later"
	ErrPaymentUnknown   warpnet.WarpError = "order: the payment got no answer and may have gone through, so nothing is sent again before the chain is checked; try again in a few minutes"
	ErrPaymentPending   warpnet.WarpError = "order: the payment is on chain and its transaction is being looked up; try again in a minute"
	ErrPriceUnagreed    warpnet.WarpError = "order: no price was agreed for this tweet; open it again to see what it costs"
	ErrPayerTaken       warpnet.WarpError = "order: this wallet already paid for another buyer of this author"
	ErrPayerAddress     warpnet.WarpError = "order: the paying wallet is not a TRON address in base58"
)

type SponsoredAuthStorer interface {
	GetOwner() domain.Owner
}

type SponsoredWallet interface {
	Address(ctx context.Context, seed string) (string, error)
	Balance(ctx context.Context, address string) (wallet.Account, error)
	Pay(ctx context.Context, seed string, s wallet.Sponsorship) (wallet.Payment, error)
	Payment(ctx context.Context, s wallet.Sponsorship, since time.Time) (wallet.PaymentState, error)
	Quote(ctx context.Context, seed string, s wallet.Sponsorship) (wallet.Quote, error)
	IsPaid(ctx context.Context, txId string, s wallet.Sponsorship) (wallet.Receipt, error)
	IsAvailable(ctx context.Context) bool
	Splitter() string
	Splitters() []string
	MaxFeePercent() uint64
	Network() string
}

// Quotes keeps what this node last quoted its owner for each tweet, so an order
// is held to the price the owner was shown even when the client sends none.
type Quotes struct {
	mu    sync.Mutex
	items map[string]quoted
}

type quoted struct {
	sponsorship wallet.Sponsorship
	at          time.Time
}

func NewQuotes() *Quotes {
	return &Quotes{items: map[string]quoted{}}
}

func (q *Quotes) put(tweetId string, s wallet.Sponsorship) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	for id, old := range q.items {
		if now.Sub(old.at) > quoteLifetime {
			delete(q.items, id)
		}
	}
	q.items[tweetId] = quoted{sponsorship: s, at: now}
}

func (q *Quotes) get(tweetId string) (wallet.Sponsorship, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	found, ok := q.items[tweetId]
	if !ok || time.Since(found.at) > quoteLifetime {
		return wallet.Sponsorship{}, false
	}
	return found.sponsorship, true
}

type OrderCounter interface {
	CountConfirmed(buyerId string, from, to time.Time) (int, error)
}

type OrderStorer interface {
	OrderCounter
	Get(tweetId, buyerId string) (domain.Order, error)
	Save(o domain.Order) error
	Delete(tweetId, buyerId string) error
	IsPayerTaken(payer, buyerNodeId string) (bool, error)
}

type OrderSettingsFetcher interface {
	GetSponsoredSettings(userId string) (domain.SponsoredSettings, error)
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

type SponsoredNotifier interface {
	Add(not domain.Notification) error
}

type SponsoredMediaStorer interface {
	GetImage(userId, key string) (domain.Base64Image, error)
	GetVideo(userId, key string) (domain.Base64Video, error)
}

type SponsoredCopyStorer interface {
	SetCopy(userId, key string, c domain.MediaCopy) error
}

type SponsoredCopyLister interface {
	ListCopies(userId, originalKey string) ([]domain.MediaCopy, error)
}

func StreamNewOrderHandler(
	auth SponsoredAuthStorer,
	identityKey ed25519.PrivateKey,
	backend SponsoredWallet,
	orders OrderStorer,
	userRepo SponsoredUserFetcher,
	streamer SponsoredStreamer,
	quotes *Quotes,
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
			order, err = openOrder(owner, identityKey, backend, orders, streamer, quotes, author, ev)
		}
		if err != nil {
			return nil, err
		}
		if !order.Confirmed {
			if order, err = settleOrder(owner, identityKey, backend, orders, order, ev.Amount); err != nil {
				return nil, err
			}
			claimed, err := claimOrder(orders, streamer, author, order)
			if err != nil {
				log.Warnf("order: %s is paid in tx %s, the author has not confirmed it yet: %v", ev.TweetId, order.TxId, err)
			} else {
				order = claimed
			}
		}
		return event.OrderResponse{TweetId: ev.TweetId, TxId: order.TxId, Confirmed: order.Confirmed}, nil
	}
}

// openOrder records an order before anything is paid, holding it to the price
// its buyer agreed to and keeping where the money goes, so every later attempt
// pays exactly that and never asks the author's node again.
func openOrder(
	owner domain.Owner,
	identityKey ed25519.PrivateKey,
	backend SponsoredWallet,
	orders OrderStorer,
	streamer SponsoredStreamer,
	quotes *Quotes,
	author domain.User,
	ev event.NewOrderEvent,
) (domain.Order, error) {
	if !backend.IsAvailable(context.Background()) {
		return domain.Order{}, ErrChainUnavailable
	}
	seed, err := wallet.DeriveSeed(owner, identityKey, backend.Network())
	if err != nil {
		return domain.Order{}, err
	}
	payer, err := backend.Address(context.Background(), seed)
	if err != nil {
		return domain.Order{}, err
	}
	sponsorship, err := tweetSponsorship(backend, streamer, author, ev.TweetId, payer)
	if err != nil {
		return domain.Order{}, err
	}
	quote, isQuoted := quotes.get(ev.TweetId)
	if err := agreePrice(sponsorship, ev.Amount, quote, isQuoted); err != nil {
		return domain.Order{}, err
	}

	nonce := make([]byte, orderNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return domain.Order{}, err
	}
	order := domain.Order{
		TweetId:       ev.TweetId,
		AuthorId:      author.Id,
		BuyerId:       owner.UserId,
		Payer:         payer,
		Nonce:         hex.EncodeToString(nonce),
		CreatedAt:     time.Now(),
		Amount:        sponsorship.Amount,
		AuthorAddress: sponsorship.Author,
		Splitter:      sponsorship.Splitter,
	}
	return order, orders.Save(order)
}

// agreePrice holds an order to the price its buyer agreed to: the amount the
// buyer's client sent and the one this node quoted. The author's node is asked
// for the price again when the order is placed, and a node that then answers
// higher than what the buyer saw would otherwise be paid its new price.
func agreePrice(s wallet.Sponsorship, amount string, quote wallet.Sponsorship, isQuoted bool) error {
	if amount == "" && !isQuoted {
		return ErrPriceUnagreed
	}
	agreed := []string{amount}
	if isQuoted {
		agreed = append(agreed, quote.Amount)
	}
	for _, units := range agreed {
		if units != "" && !sameUnits(s.Amount, units) {
			return warpnet.WarpError(fmt.Sprintf("order: the author now asks %s for this tweet, not the %s agreed; nothing was paid", s.Amount, units))
		}
	}
	if isQuoted && (s.Author != quote.Author || s.Splitter != quote.Splitter) {
		return warpnet.WarpError("order: the author's wallet changed since the quote; nothing was paid")
	}
	return nil
}

func sameUnits(a, b string) bool {
	x, okA := new(big.Int).SetString(strings.TrimSpace(a), 10)
	y, okB := new(big.Int).SetString(strings.TrimSpace(b), 10)
	return okA && okB && x.Cmp(y) == 0
}

// settleOrder brings an unconfirmed order to a payment it can claim. It asks the
// splitter whether the order is paid and adopts the transaction that paid it,
// and pays only when the order is unpaid and no earlier payment of it can still
// land. Every attempt pays under the order's own id, which the splitter will not
// let the same wallet pay twice.
func settleOrder(
	owner domain.Owner,
	identityKey ed25519.PrivateKey,
	backend SponsoredWallet,
	orders OrderStorer,
	order domain.Order,
	amount string,
) (domain.Order, error) {
	if order.Splitter == "" {
		// Opened before orders kept what they pay, on a splitter that records nothing.
		if order.TxId == "" {
			return order, ErrPaymentUnknown
		}
		return order, nil
	}
	if amount != "" && !sameUnits(order.Amount, amount) {
		return order, warpnet.WarpError(fmt.Sprintf("order: this order pays %s, not the %s asked; nothing was paid", order.Amount, amount))
	}
	ctx := context.Background()
	sponsorship := orderSponsorship(order, backend.MaxFeePercent())
	state, err := backend.Payment(ctx, sponsorship, order.CreatedAt.Add(-time.Hour))
	if err != nil {
		return order, err
	}
	if state.Paid {
		if state.Tx != "" && state.Tx != order.TxId {
			order.TxId = state.Tx
			if err := orders.Save(order); err != nil {
				return order, err
			}
		}
		if order.TxId == "" {
			return order, ErrPaymentPending
		}
		return order, nil
	}
	if !order.SentAt.IsZero() && time.Since(order.SentAt) < paymentLifetime {
		if order.TxId == "" {
			return order, ErrPaymentUnknown
		}
		return order, nil
	}
	seed, err := wallet.DeriveSeed(owner, identityKey, backend.Network())
	if err != nil {
		return order, err
	}
	return payOrder(ctx, backend, orders, seed, order, sponsorship)
}

// payOrder sends an order's payment and records what came of it. The attempt is
// saved before it is sent, so a crash leaves an order that waits out the
// payment's lifetime instead of one that pays again at once.
func payOrder(ctx context.Context, backend SponsoredWallet, orders OrderStorer, seed string, order domain.Order, s wallet.Sponsorship) (domain.Order, error) {
	order.TxId = ""
	order.SentAt = time.Now()
	if err := orders.Save(order); err != nil {
		return order, err
	}
	payment, err := backend.Pay(ctx, seed, s)
	switch {
	case errors.Is(err, wallet.ErrAlreadyPaid):
		return order, ErrPaymentPending
	case errors.Is(err, wallet.ErrReverted):
		// Nothing moved but the network fee, so the order can be paid again now.
		order.SentAt = time.Time{}
		if saveErr := orders.Save(order); saveErr != nil {
			log.Errorf("order: %s reverted, but its order was not released: %v", order.TweetId, saveErr)
		}
		return order, warpnet.WarpError("order: the payment reverted and moved nothing but the network fee; try again: " + err.Error())
	case payment.PayTx != "":
		order.TxId = payment.PayTx
		if saveErr := orders.Save(order); saveErr != nil {
			log.Errorf("order: %s paid in tx %s but the receipt was not saved: %v", order.TweetId, payment.PayTx, saveErr)
		}
		if err != nil {
			log.Warnf("order: the payment for %s got no answer, tx %s may still land: %v", order.TweetId, payment.PayTx, err)
		}
		return order, nil
	case err == nil || errors.Is(err, wallet.ErrNoAnswer):
		log.Errorf("order: the payment for %s may have gone through, nothing is sent again before the chain is checked: %v", order.TweetId, err)
		return order, ErrPaymentUnknown
	default:
		// The engine answered without a transaction, so it sent nothing.
		order.SentAt = time.Time{}
		if saveErr := orders.Save(order); saveErr != nil {
			log.Errorf("order: %s was not paid, but its order was not released: %v", order.TweetId, saveErr)
		}
		log.Errorf("order: pay for %s: %v", order.TweetId, err)
		return order, err
	}
}

func orderSponsorship(order domain.Order, maxFeePercent uint64) wallet.Sponsorship {
	return wallet.Sponsorship{
		Splitter:      order.Splitter,
		OrderId:       order.ID(),
		Author:        order.AuthorAddress,
		Buyer:         order.Payer,
		Amount:        order.Amount,
		MaxFeePercent: maxFeePercent,
		MaxTotal:      maxTotal(order.Amount, maxFeePercent),
	}
}

// maxTotal is the most an order may cost: its amount and the highest fee the
// buyer accepts on top.
func maxTotal(amount string, maxFeePercent uint64) string {
	units, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return ""
	}
	fee := new(big.Int).Div(new(big.Int).Mul(units, new(big.Int).SetUint64(maxFeePercent)), big.NewInt(100))
	return new(big.Int).Add(units, fee).String()
}

func tweetSponsorship(
	backend SponsoredWallet,
	streamer SponsoredStreamer,
	author domain.User,
	tweetId, payer string,
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

	address, splitter, err := authorAddress(streamer, author, backend.Network(), payer)
	if err != nil {
		return wallet.Sponsorship{}, err
	}
	// Pay through the splitter the author's node accepts, so the payment is one it
	// will confirm. A node that does not say is too old to confirm a payment
	// through this node's splitter, or takes no payments at all.
	if splitter == "" {
		return wallet.Sponsorship{}, warpnet.WarpError("order: the author's node does not say which splitter it takes payments through and needs an update; nothing was paid")
	}
	if !slices.Contains(backend.Splitters(), splitter) {
		return wallet.Sponsorship{}, warpnet.WarpError("order: the author takes payments through " + splitter + ", which this node does not know; one of the two nodes needs an update")
	}
	return wallet.Sponsorship{
		Splitter:      splitter,
		Author:        address,
		Amount:        teaser.Price.Units.String(),
		MaxFeePercent: backend.MaxFeePercent(),
	}, nil
}

func StreamGetOrderQuoteHandler(
	auth SponsoredAuthStorer,
	identityKey ed25519.PrivateKey,
	backend SponsoredWallet,
	userRepo SponsoredUserFetcher,
	streamer SponsoredStreamer,
	quotes *Quotes,
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
		seed, err := wallet.DeriveSeed(owner, identityKey, backend.Network())
		if err != nil {
			return nil, err
		}
		payer, err := backend.Address(context.Background(), seed)
		if err != nil {
			return nil, err
		}
		sponsorship, err := tweetSponsorship(backend, streamer, author, ev.TweetId, payer)
		if err != nil {
			return nil, err
		}
		quote, err := backend.Quote(context.Background(), seed, sponsorship)
		if err != nil {
			log.Warnf("order quote: %s: %v", ev.TweetId, err)
			return nil, err
		}
		quotes.put(ev.TweetId, sponsorship)

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

// authorAddress asks the author's node where to pay: its wallet, and the
// splitter it confirms payments through when it says.
func authorAddress(streamer SponsoredStreamer, author domain.User, network, payer string) (address, splitter string, err error) {
	resp, err := streamer.GenericStream(author.NodeId, event.PUBLIC_GET_WALLET_ADDRESS, event.WalletAddressEvent{Chain: network, Payer: payer})
	if err != nil {
		return "", "", err
	}
	var possibleError event.ResponseError
	if _ = json.Unmarshal(resp, &possibleError); possibleError.Message != "" {
		return "", "", warpnet.WarpError(possibleError.Message)
	}
	var answer event.WalletAddressResponse
	if err := json.Unmarshal(resp, &answer); err != nil {
		return "", "", err
	}
	if answer.Address == "" || answer.Chain != network || answer.UserId != author.Id {
		return "", "", warpnet.WarpError("order: the author's node gave no wallet address on " + network)
	}
	return answer.Address, answer.Splitter, nil
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
		Payer:   order.Payer,
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
	auth SponsoredAuthStorer,
	identityKey ed25519.PrivateKey,
	backend SponsoredWallet,
	tweetRepo SponsoredTweetFetcher,
	orders OrderStorer,
	userRepo SponsoredUserFetcher,
	notifier SponsoredNotifier,
	streamer SponsoredStreamer,
	settings OrderSettingsFetcher,
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
		buyer, err := authorship.VerifyActor(userRepo, streamer, s, ev.UserId)
		if err != nil {
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
		now := time.Now()
		count, err := orders.CountConfirmed(ev.UserId, now.Add(-domain.OrderLimitWindow), now)
		if err != nil {
			return nil, err
		}
		if count >= domain.OrderLimit {
			return event.OrderResponse{TweetId: ev.TweetId, TxId: ev.TxId}, nil
		}
		sponsored, err := settings.GetSponsoredSettings(owner.UserId)
		if err != nil {
			return nil, err
		}
		if err := checkBuyerPayer(orders, ev.Payer, buyer.NodeId, sponsored.ClaimMinStakeTRX(now)); err != nil {
			return nil, err
		}

		order := domain.Order{
			TweetId:     ev.TweetId,
			AuthorId:    owner.UserId,
			BuyerId:     ev.UserId,
			BuyerNodeId: buyer.NodeId,
			Payer:       ev.Payer,
			Nonce:       ev.Nonce,
			TxId:        ev.TxId,
			CreatedAt:   time.Now(),
		}
		seed, err := wallet.DeriveSeed(owner, identityKey, backend.Network())
		if err != nil {
			return nil, err
		}
		ctx := context.Background()
		address, err := backend.Address(ctx, seed)
		if err != nil {
			return nil, err
		}
		receipt, err := backend.IsPaid(ctx, ev.TxId, wallet.Sponsorship{
			Splitter: backend.Splitter(),
			OrderId:  order.ID(),
			Author:   address,
			Buyer:    ev.Payer,
			Amount:   tweet.Price.Units.String(),
		})
		if err != nil {
			log.Errorf("order: verify %s for %s: %v", ev.TxId, ev.TweetId, err)
			return nil, err
		}
		if receipt.Confirmed {
			// A buyer who paid under one stake rule is not refused for a stricter one
			// set after the payment: the gentler of the rule then and the rule now applies.
			paidAt := receipt.BlockTime
			if paidAt.IsZero() {
				paidAt = now
			}
			minStake := min(sponsored.ClaimMinStakeTRX(paidAt), sponsored.ClaimMinStakeTRX(now))
			if err := checkPayerStake(backend, ev.Payer, minStake); err != nil {
				return nil, err
			}
			order.Confirmed = true
			if err := orders.Save(order); err != nil {
				return nil, err
			}
			if err := notifyOrderLimit(notifier, orders, owner.UserId, buyer); err != nil {
				log.Errorf("order: telling the author about %s's order limit: %v", buyer.Id, err)
			}
		}
		return event.OrderResponse{TweetId: ev.TweetId, TxId: ev.TxId, Confirmed: receipt.Confirmed}, nil
	}
}

func StreamGetSponsoredTweetHandler(
	auth SponsoredAuthStorer,
	identityKey ed25519.PrivateKey,
	tweetRepo SponsoredTweetFetcher,
	orders OrderStorer,
	mediaRepo SponsoredMediaStorer,
	copyRepo SponsoredCopyStorer,
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
		isOwn := warpnet.VerifyAuthorship(s, streamer.NodeInfo().ID.String()) == nil
		if ev.UserId == owner.UserId && isOwn {
			return tweetRepo.Get(owner.UserId, ev.TweetId)
		}
		if ev.UserId == owner.UserId {
			buyer, err := findRequester(s, userRepo)
			if err != nil {
				return nil, err
			}
			order, err := orders.Get(ev.TweetId, buyer.Id)
			if err != nil && !errors.Is(err, database.ErrOrderNotFound) {
				return nil, err
			}
			if !order.Confirmed {
				return domain.Tweet{}, nil
			}
			tweet, err := tweetRepo.Get(owner.UserId, ev.TweetId)
			if err != nil {
				return nil, err
			}
			signer := media_meta.Metadata{PrivKey: identityKey, NodeId: streamer.NodeInfo().ID.String(), OwnerId: owner.UserId}
			return copyTweetMedia(signer, mediaRepo, copyRepo, tweet, buyer, order)
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
		if order.TxId == "" {
			return domain.Tweet{}, nil
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

func findRequester(s warpnet.WarpStream, userRepo SponsoredUserFetcher) (domain.User, error) {
	if s == nil || s.Conn() == nil {
		return domain.User{}, database.ErrUserNotFound
	}
	return userRepo.GetByNodeID(s.Conn().RemotePeer().String())
}

func copyTweetMedia(
	signer media_meta.Metadata,
	mediaRepo SponsoredMediaStorer,
	copyRepo SponsoredCopyStorer,
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

	watermarkText := buyer.Id
	imageKeys := make([]string, 0, len(tweet.ImageKeys))
	for _, key := range tweet.ImageKeys {
		img, err := mediaRepo.GetImage(tweet.UserId, key)
		if err != nil {
			return domain.Tweet{}, err
		}
		_, raw, err := media_meta.SplitDataURI(string(img))
		if err != nil {
			return domain.Tweet{}, err
		}
		watermark, err := media_meta.WatermarkPNG(raw, watermarkText)
		if err != nil {
			return domain.Tweet{}, err
		}
		c := domain.MediaCopy{OriginalKey: key, BuyerId: order.BuyerId, EncryptedOrder: encryptedOrder, Watermark: watermark, IsPixelMarked: true}
		copied, err := media_meta.BuildImageCopy(string(img), c, signer)
		if err != nil {
			return domain.Tweet{}, err
		}
		copyKey := media_meta.BuildContentKey(copied)
		if err := copyRepo.SetCopy(tweet.UserId, copyKey, c); err != nil {
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
	c := domain.MediaCopy{OriginalKey: *tweet.VideoKey, BuyerId: order.BuyerId, EncryptedOrder: encryptedOrder}
	copied, err := media_meta.BuildVideoCopy(string(video), c)
	if err != nil {
		return domain.Tweet{}, err
	}
	copyKey := media_meta.BuildContentKey(copied)
	if err := copyRepo.SetCopy(tweet.UserId, copyKey, c); err != nil {
		return domain.Tweet{}, err
	}
	tweet.VideoKey = &copyKey
	return tweet, nil
}

func notifyOrderLimit(notifier SponsoredNotifier, orders OrderCounter, authorId string, buyer domain.User) error {
	now := time.Now()
	count, err := orders.CountConfirmed(buyer.Id, now.Add(-domain.OrderLimitWindow), now)
	if err != nil || count != domain.OrderLimit {
		return err
	}
	name := buyer.Username
	if name == "" {
		name = buyer.Id
	}
	return notifier.Add(domain.Notification{
		Type:        domain.NotificationOrderLimitType,
		Text:        name + " bought " + strconv.Itoa(domain.OrderLimit) + " of your tweets within an hour; their next orders wait",
		RecepientId: authorId,
		ActorId:     buyer.Id,
	})
}

func StreamGetCopyBuyerHandler(
	auth SponsoredAuthStorer,
	identityKey ed25519.PrivateKey,
	tweetRepo SponsoredTweetFetcher,
	orders OrderStorer,
	mediaRepo SponsoredMediaStorer,
	copyRepo SponsoredCopyLister,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.GetCopyBuyerEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		_, raw, err := media_meta.SplitDataURI(ev.File)
		if err != nil {
			return nil, err
		}

		encryptedOrder, err := media_meta.ExtractOrder(raw)
		isUnmarked := errors.Is(err, media_meta.ErrNoMetadata) || errors.Is(err, media_meta.ErrNoOrder)
		if isUnmarked && (ev.TweetId == "" || media_meta.IsISOBaseMediaFile(raw)) {
			return event.CopyBuyerResponse{}, nil
		}
		if isUnmarked {
			tweet, err := tweetRepo.Get(auth.GetOwner().UserId, ev.TweetId)
			if err != nil {
				return nil, err
			}
			order, score, err := findOrderByPixelMark(identityKey, tweet, raw, orders, mediaRepo, copyRepo)
			if err != nil {
				return nil, err
			}
			if score < media_meta.MinPixelMarkScore {
				return event.CopyBuyerResponse{}, nil
			}
			resp, err := buildCopyBuyerResponse(orders, order)
			if err != nil {
				return nil, err
			}
			resp.IsPixelMatch, resp.PixelScore = true, score
			return resp, nil
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
		return buildCopyBuyerResponse(orders, order)
	}
}

func buildCopyBuyerResponse(orders OrderCounter, order domain.Order) (event.CopyBuyerResponse, error) {
	dailyOrdersCount, err := orders.CountConfirmed(
		order.BuyerId, order.CreatedAt.Add(-dailyOrdersWindow), order.CreatedAt.Add(dailyOrdersWindow),
	)
	if err != nil {
		return event.CopyBuyerResponse{}, err
	}
	return event.CopyBuyerResponse{
		TweetId:          order.TweetId,
		BuyerId:          order.BuyerId,
		OrderId:          order.ID(),
		TxId:             order.TxId,
		SoldAt:           order.CreatedAt,
		DailyOrdersCount: dailyOrdersCount,
	}, nil
}

func findOrderByPixelMark(
	identityKey ed25519.PrivateKey,
	tweet domain.Tweet,
	suspect []byte,
	orders OrderStorer,
	mediaRepo SponsoredMediaStorer,
	copyRepo SponsoredCopyLister,
) (domain.Order, float64, error) {
	var (
		best      domain.Order
		bestScore float64
	)
	for _, key := range tweet.ImageKeys {
		sold, err := listPixelMarkedOrders(tweet, key, orders, copyRepo)
		if err != nil {
			return domain.Order{}, 0, err
		}
		if len(sold) == 0 {
			continue
		}
		original, err := mediaRepo.GetImage(tweet.UserId, key)
		if err != nil {
			return domain.Order{}, 0, err
		}
		_, raw, err := media_meta.SplitDataURI(string(original))
		if err != nil {
			return domain.Order{}, 0, err
		}
		seeds := make([][]byte, len(sold))
		for i, order := range sold {
			seeds[i] = media_meta.BuildPixelMarkSeed(identityKey, key, order.BuyerId)
		}
		scores, err := media_meta.ScorePixelMark(raw, suspect, seeds)
		if err != nil {
			return domain.Order{}, 0, err
		}
		for i, score := range scores {
			if score > bestScore {
				best, bestScore = sold[i], score
			}
		}
	}
	return best, bestScore, nil
}

func listPixelMarkedOrders(tweet domain.Tweet, originalKey string, orders OrderStorer, copyRepo SponsoredCopyLister) ([]domain.Order, error) {
	copies, err := copyRepo.ListCopies(tweet.UserId, originalKey)
	if err != nil {
		return nil, err
	}
	sold := make([]domain.Order, 0, len(copies))
	for _, c := range copies {
		if !c.IsPixelMarked {
			continue
		}
		order, err := orders.Get(tweet.Id, c.BuyerId)
		if errors.Is(err, database.ErrOrderNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		sold = append(sold, order)
	}
	return sold, nil
}

// checkBuyerPayer refuses, before anything is asked of the chain, a claim whose
// payer is malformed, missing where a stake is asked, or bound to another node.
func checkBuyerPayer(orders OrderStorer, payer, buyerNodeId string, minStakeTRX uint64) error {
	if payer != "" && !wallet.IsAddress(payer) {
		return ErrPayerAddress
	}
	if minStakeTRX == 0 {
		return nil
	}
	if payer == "" {
		return stakeRefusal(minStakeTRX)
	}
	isTaken, err := orders.IsPayerTaken(payer, buyerNodeId)
	if err != nil {
		return err
	}
	if isTaken {
		return ErrPayerTaken
	}
	return nil
}

// checkPayerStake refuses a payer with less staked or unstaking TRX than asked.
func checkPayerStake(backend SponsoredWallet, payer string, minStakeTRX uint64) error {
	if minStakeTRX == 0 {
		return nil
	}
	if payer == "" {
		return stakeRefusal(minStakeTRX)
	}
	account, err := backend.Balance(context.Background(), payer)
	if err != nil {
		return err
	}
	if !account.IsLockedEnough(minStakeTRX) {
		return stakeRefusal(minStakeTRX)
	}
	return nil
}

func stakeRefusal(minStakeTRX uint64) error {
	return warpnet.WarpError(fmt.Sprintf("order: the author sells only to wallets with at least %d TRX staked", minStakeTRX))
}
