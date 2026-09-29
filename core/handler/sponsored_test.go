//nolint:all
package handler

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/wallet"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSponsoredWallet struct {
	splitter string
	pays     []wallet.Sponsorship
	payTx    string
	payErr   error
	checks   []wallet.Sponsorship
	isPaid   bool
}

func (w *stubSponsoredWallet) Address(_ context.Context, _ string) (string, error) {
	return "TAuthorAddress", nil
}

func (w *stubSponsoredWallet) Pay(_ context.Context, _ string, s wallet.Sponsorship) (wallet.Payment, error) {
	w.pays = append(w.pays, s)
	return wallet.Payment{PayTx: w.payTx}, w.payErr
}

func (w *stubSponsoredWallet) IsPaid(_ context.Context, _ string, s wallet.Sponsorship) (bool, error) {
	w.checks = append(w.checks, s)
	return w.isPaid, nil
}

func (w *stubSponsoredWallet) Splitter() string      { return w.splitter }
func (w *stubSponsoredWallet) MaxFeePercent() uint64 { return 5 }
func (w *stubSponsoredWallet) Network() string       { return "testnet" }

type stubPurchases map[string]domain.Purchase

func (p stubPurchases) Get(tweetId, buyerId string) (domain.Purchase, error) {
	got, ok := p[tweetId+"/"+buyerId]
	if !ok {
		return domain.Purchase{}, database.ErrPurchaseNotFound
	}
	return got, nil
}

func (p stubPurchases) Save(purchase domain.Purchase) error {
	p[purchase.TweetId+"/"+purchase.BuyerId] = purchase
	return nil
}

type stubSponsoredUsers map[string]domain.User

func (u stubSponsoredUsers) Get(userId string) (domain.User, error) {
	got, ok := u[userId]
	if !ok {
		return domain.User{}, database.ErrUserNotFound
	}
	return got, nil
}

func (u stubSponsoredUsers) Create(user domain.User) (domain.User, error) {
	u[user.Id] = user
	return user, nil
}

func (u stubSponsoredUsers) GetByNodeID(nodeId string) (domain.User, error) {
	for _, user := range u {
		if user.NodeId == nodeId {
			return user, nil
		}
	}
	return domain.User{}, database.ErrUserNotFound
}

func testIdentityKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return key
}

func authorNode(t *testing.T, onVerify func(event.VerifyPurchaseEvent) ([]byte, error)) stubStreamer {
	t.Helper()
	return stubStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
		assert.Equal(t, "author-node", nodeId)
		switch path {
		case event.PUBLIC_GET_TWEET:
			return json.Marshal(domain.Tweet{Id: "tweet-1", UserId: "author-1", Price: sponsoredPrice()})
		case event.PUBLIC_GET_WALLET_ADDRESS:
			return json.Marshal(event.WalletAddressResponse{Address: "TAuthorAddress", Chain: "testnet", UserId: "author-1"})
		case event.PUBLIC_POST_SPONSORED_PURCHASE:
			return onVerify(data.(event.VerifyPurchaseEvent))
		}
		t.Fatalf("unexpected stream to %s", path)
		return nil, nil
	}}
}

func TestStreamNewPurchaseHandler(t *testing.T) {
	owner := domain.Owner{UserId: "buyer-1", Username: "bob"}
	users := stubSponsoredUsers{"author-1": {Id: "author-1", NodeId: "author-node"}}
	confirm := func(ev event.VerifyPurchaseEvent) ([]byte, error) {
		return json.Marshal(event.PurchaseResponse{TweetId: ev.TweetId, TxId: ev.TxId, Confirmed: true})
	}
	newHandler := func(w SponsoredWallet, purchases PurchaseStorer, streamer SponsoredStreamer) warpnet.WarpHandlerFunc {
		return StreamNewPurchaseHandler(stubAuth{owner: owner}, testIdentityKey(t), w, purchases, users, streamer)
	}

	for _, tt := range []struct {
		name string
		ev   event.NewPurchaseEvent
		want string
	}{
		{"empty tweet id", event.NewPurchaseEvent{UserId: "author-1"}, "purchase: empty tweet id"},
		{"empty user id", event.NewPurchaseEvent{TweetId: "tweet-1"}, "purchase: empty user id"},
		{"own tweet", event.NewPurchaseEvent{TweetId: "tweet-1", UserId: "buyer-1"}, "purchase: an own tweet is not for sale"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newHandler(&stubSponsoredWallet{}, stubPurchases{}, stubStreamer{})(marshal(t, tt.ev), nil)
			assert.EqualError(t, err, tt.want)
		})
	}

	t.Run("pays the author's price and claims the tweet", func(t *testing.T) {
		w := &stubSponsoredWallet{splitter: "TSplitter", payTx: "tx-1"}
		purchases := stubPurchases{}
		var claimed event.VerifyPurchaseEvent
		resp, err := newHandler(w, purchases, authorNode(t, func(ev event.VerifyPurchaseEvent) ([]byte, error) {
			claimed = ev
			return confirm(ev)
		}))(marshal(t, event.NewPurchaseEvent{TweetId: "tweet-1", UserId: "author-1"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.PurchaseResponse{TweetId: "tweet-1", TxId: "tx-1", Confirmed: true}, resp)

		require.Len(t, w.pays, 1)
		saved := purchases["tweet-1/buyer-1"]
		assert.Equal(t, wallet.Sponsorship{
			Splitter: "TSplitter", OrderId: saved.OrderId(), Author: "TAuthorAddress", Amount: "1500000", MaxFeePercent: 5,
		}, w.pays[0])
		assert.Len(t, saved.OrderId(), 64)
		assert.True(t, saved.Confirmed)
		assert.Equal(t, event.VerifyPurchaseEvent{TweetId: "tweet-1", UserId: "buyer-1", TxId: "tx-1", Nonce: saved.Nonce}, claimed)
	})

	t.Run("a saved receipt is claimed again, never paid twice", func(t *testing.T) {
		w := &stubSponsoredWallet{splitter: "TSplitter"}
		purchases := stubPurchases{"tweet-1/buyer-1": {TweetId: "tweet-1", BuyerId: "buyer-1", Nonce: "n1", TxId: "tx-0"}}
		resp, err := newHandler(w, purchases, authorNode(t, confirm))(marshal(t, event.NewPurchaseEvent{TweetId: "tweet-1", UserId: "author-1"}), nil)
		require.NoError(t, err)
		assert.Empty(t, w.pays)
		assert.True(t, resp.(event.PurchaseResponse).Confirmed)
	})

	t.Run("an offline author leaves the paid receipt pending", func(t *testing.T) {
		w := &stubSponsoredWallet{splitter: "TSplitter", payTx: "tx-2"}
		purchases := stubPurchases{}
		resp, err := newHandler(w, purchases, authorNode(t, func(event.VerifyPurchaseEvent) ([]byte, error) {
			return nil, warpnet.ErrNodeIsOffline
		}))(marshal(t, event.NewPurchaseEvent{TweetId: "tweet-1", UserId: "author-1"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.PurchaseResponse{TweetId: "tweet-1", TxId: "tx-2", Confirmed: false}, resp)
		assert.Equal(t, "tx-2", purchases["tweet-1/buyer-1"].TxId)
	})

	t.Run("a network without a splitter takes no payment", func(t *testing.T) {
		w := &stubSponsoredWallet{}
		_, err := newHandler(w, stubPurchases{}, authorNode(t, confirm))(marshal(t, event.NewPurchaseEvent{TweetId: "tweet-1", UserId: "author-1"}), nil)
		assert.EqualError(t, err, "purchase: sponsored payments are not open on testnet")
		assert.Empty(t, w.pays)
	})
}

func TestStreamVerifyPurchaseHandler(t *testing.T) {
	owner := domain.Owner{UserId: "author-1", Username: "alice"}
	buyerNode := newTestPeerID(t)
	users := stubSponsoredUsers{"buyer-1": {Id: "buyer-1", NodeId: buyerNode.String()}}
	paidTweet := stubTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
		return domain.Tweet{Id: tweetID, UserId: userID, Text: "paid", Price: sponsoredPrice()}, nil
	}}
	claim := event.VerifyPurchaseEvent{TweetId: "tweet-1", UserId: "buyer-1", TxId: "tx-1", Nonce: "n1"}
	fromBuyer := func() warpnet.WarpStream {
		_, s := stream.NewLoopbackStream(newTestPeerID(t), buyerNode, event.PUBLIC_POST_SPONSORED_PURCHASE)
		return s
	}
	newHandler := func(w SponsoredWallet, tweets SponsoredTweetFetcher, purchases PurchaseStorer) warpnet.WarpHandlerFunc {
		return StreamVerifyPurchaseHandler(stubAuth{owner: owner}, testIdentityKey(t), w, tweets, purchases, users, stubStreamer{})
	}

	t.Run("confirms a payment bound to this buyer and records it", func(t *testing.T) {
		w := &stubSponsoredWallet{splitter: "TSplitter", isPaid: true}
		purchases := stubPurchases{}
		resp, err := newHandler(w, paidTweet, purchases)(marshal(t, claim), fromBuyer())
		require.NoError(t, err)
		assert.True(t, resp.(event.PurchaseResponse).Confirmed)

		want := domain.Purchase{TweetId: "tweet-1", BuyerId: "buyer-1", Nonce: "n1"}
		require.Len(t, w.checks, 1)
		assert.Equal(t, wallet.Sponsorship{Splitter: "TSplitter", OrderId: want.OrderId(), Author: "TAuthorAddress", Amount: "1500000"}, w.checks[0])
		assert.True(t, purchases["tweet-1/buyer-1"].Confirmed)
	})

	t.Run("a pending payment is not recorded", func(t *testing.T) {
		purchases := stubPurchases{}
		resp, err := newHandler(&stubSponsoredWallet{splitter: "TSplitter"}, paidTweet, purchases)(marshal(t, claim), fromBuyer())
		require.NoError(t, err)
		assert.False(t, resp.(event.PurchaseResponse).Confirmed)
		assert.Empty(t, purchases)
	})

	t.Run("a claim from another node is refused", func(t *testing.T) {
		_, stranger := stream.NewLoopbackStream(newTestPeerID(t), newTestPeerID(t), event.PUBLIC_POST_SPONSORED_PURCHASE)
		w := &stubSponsoredWallet{splitter: "TSplitter", isPaid: true}
		_, err := newHandler(w, paidTweet, stubPurchases{})(marshal(t, claim), stranger)
		assert.ErrorIs(t, err, warpnet.ErrForeignAuthor)
		assert.Empty(t, w.checks)
	})

	t.Run("a free tweet is not for sale", func(t *testing.T) {
		_, err := newHandler(&stubSponsoredWallet{splitter: "TSplitter"}, stubTweetRepo{}, stubPurchases{})(marshal(t, claim), fromBuyer())
		assert.EqualError(t, err, "purchase: tweet is not sponsored")
	})

	t.Run("a confirmed purchase is not verified again", func(t *testing.T) {
		w := &stubSponsoredWallet{splitter: "TSplitter"}
		purchases := stubPurchases{"tweet-1/buyer-1": {TweetId: "tweet-1", BuyerId: "buyer-1", TxId: "tx-1", Confirmed: true}}
		resp, err := newHandler(w, paidTweet, purchases)(marshal(t, claim), fromBuyer())
		require.NoError(t, err)
		assert.True(t, resp.(event.PurchaseResponse).Confirmed)
		assert.Empty(t, w.checks)
	})

	for _, tt := range []struct {
		name string
		ev   event.VerifyPurchaseEvent
		want string
	}{
		{"empty tweet id", event.VerifyPurchaseEvent{UserId: "buyer-1", TxId: "tx", Nonce: "n"}, "purchase: empty tweet id"},
		{"empty user id", event.VerifyPurchaseEvent{TweetId: "t", TxId: "tx", Nonce: "n"}, "purchase: empty user id"},
		{"empty tx id", event.VerifyPurchaseEvent{TweetId: "t", UserId: "buyer-1", Nonce: "n"}, "purchase: empty tx id"},
		{"empty nonce", event.VerifyPurchaseEvent{TweetId: "t", UserId: "buyer-1", TxId: "tx"}, "purchase: empty nonce"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newHandler(&stubSponsoredWallet{}, paidTweet, stubPurchases{})(marshal(t, tt.ev), fromBuyer())
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestStreamGetSponsoredTweetHandler(t *testing.T) {
	own, buyerNode, strangerNode := newTestPeerID(t), newTestPeerID(t), newTestPeerID(t)
	full := domain.Tweet{Id: "tweet-1", UserId: "author-1", Text: "paid", ImageKeys: []string{"img-1"}, Price: sponsoredPrice()}
	tweets := stubTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) { return full, nil }}
	req := marshal(t, event.GetTweetEvent{UserId: "author-1", TweetId: "tweet-1"})
	streamFrom := func(remote warpnet.WarpPeerID) warpnet.WarpStream {
		_, s := stream.NewLoopbackStream(own, remote, event.PUBLIC_GET_SPONSORED_TWEET)
		return s
	}

	t.Run("on the author's node", func(t *testing.T) {
		users := stubSponsoredUsers{
			"buyer-1":    {Id: "buyer-1", NodeId: buyerNode.String()},
			"stranger-1": {Id: "stranger-1", NodeId: strangerNode.String()},
		}
		purchases := stubPurchases{"tweet-1/buyer-1": {TweetId: "tweet-1", BuyerId: "buyer-1", Confirmed: true}}
		h := StreamGetSponsoredTweetHandler(stubAuth{owner: domain.Owner{UserId: "author-1"}}, tweets, purchases, users,
			stubStreamer{nodeInfo: warpnet.NodeInfo{ID: own, OwnerId: "author-1"}})

		resp, err := h(req, streamFrom(own))
		require.NoError(t, err)
		assert.Equal(t, "paid", resp.(domain.Tweet).Text, "the author reads the own tweet")

		resp, err = h(req, streamFrom(buyerNode))
		require.NoError(t, err)
		assert.Equal(t, full, resp, "a confirmed buyer gets the full tweet")

		resp, err = h(req, streamFrom(strangerNode))
		require.NoError(t, err)
		assert.Equal(t, domain.Tweet{}, resp, "someone who did not pay gets nothing")

		resp, err = h(req, streamFrom(newTestPeerID(t)))
		require.NoError(t, err)
		assert.Equal(t, domain.Tweet{}, resp, "an unknown node gets nothing")
	})

	t.Run("on the buyer's node", func(t *testing.T) {
		users := stubSponsoredUsers{"author-1": {Id: "author-1", NodeId: "author-node"}}
		auth := stubAuth{owner: domain.Owner{UserId: "buyer-1"}}
		var paths []stream.WarpRoute
		confirmed := true
		streamer := stubStreamer{
			nodeInfo: warpnet.NodeInfo{ID: own, OwnerId: "buyer-1"},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				assert.Equal(t, "author-node", nodeId)
				paths = append(paths, path)
				if path == event.PUBLIC_POST_SPONSORED_PURCHASE {
					return json.Marshal(event.PurchaseResponse{TweetId: "tweet-1", TxId: "tx-1", Confirmed: confirmed})
				}
				return json.Marshal(full)
			},
		}

		resp, err := StreamGetSponsoredTweetHandler(auth, tweets, stubPurchases{}, users, streamer)(req, streamFrom(own))
		require.NoError(t, err)
		assert.Equal(t, domain.Tweet{}, resp, "nothing bought, nothing shown")
		assert.Empty(t, paths, "nothing bought, nothing asked")

		purchases := stubPurchases{"tweet-1/buyer-1": {TweetId: "tweet-1", BuyerId: "buyer-1", TxId: "tx-1", Nonce: "n1"}}
		h := StreamGetSponsoredTweetHandler(auth, tweets, purchases, users, streamer)

		confirmed = false
		_, err = h(req, streamFrom(own))
		assert.ErrorIs(t, err, ErrSponsoredPending, "a receipt the author has not confirmed yet is still pending")

		confirmed = true
		paths = nil
		resp, err = h(req, streamFrom(own))
		require.NoError(t, err)
		assert.Equal(t, "paid", resp.(domain.Tweet).Text, "opening the tweet claims the receipt again")
		assert.Equal(t, []stream.WarpRoute{event.PUBLIC_POST_SPONSORED_PURCHASE, event.PUBLIC_GET_SPONSORED_TWEET}, paths)
		assert.Equal(t, "paid", purchases["tweet-1/buyer-1"].Tweet.Text, "the bought tweet is kept with the purchase")

		paths = nil
		_, err = h(req, streamFrom(own))
		require.NoError(t, err)
		assert.Empty(t, paths, "a kept tweet needs no author")

		resp, err = h(req, streamFrom(strangerNode))
		require.NoError(t, err)
		assert.Equal(t, domain.Tweet{}, resp, "a buyer never passes the tweet on")
	})
}
