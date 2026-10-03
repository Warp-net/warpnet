//nolint:all
package handler

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/wallet"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
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
	quotes   []wallet.Sponsorship
	quote    wallet.Quote
}

func (w *stubSponsoredWallet) Quote(_ context.Context, _ string, s wallet.Sponsorship) (wallet.Quote, error) {
	w.quotes = append(w.quotes, s)
	return w.quote, nil
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

type stubOrders map[string]domain.Order

func (o stubOrders) Get(tweetId, buyerId string) (domain.Order, error) {
	got, ok := o[tweetId+"/"+buyerId]
	if !ok {
		return domain.Order{}, database.ErrOrderNotFound
	}
	return got, nil
}

func (o stubOrders) Save(order domain.Order) error {
	o[order.TweetId+"/"+order.BuyerId] = order
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

type stubSponsoredMedia struct {
	images map[string]domain.Base64Image
	videos map[string]domain.Base64Video
	copies map[string]domain.MediaCopy
}

func newStubSponsoredMedia() stubSponsoredMedia {
	return stubSponsoredMedia{
		images: map[string]domain.Base64Image{},
		videos: map[string]domain.Base64Video{},
		copies: map[string]domain.MediaCopy{},
	}
}

func (m stubSponsoredMedia) GetImage(userId, key string) (domain.Base64Image, error) {
	img, ok := m.images[userId+"/"+key]
	if !ok {
		return "", database.ErrMediaNotFound
	}
	return img, nil
}

func (m stubSponsoredMedia) GetVideo(userId, key string) (domain.Base64Video, error) {
	video, ok := m.videos[userId+"/"+key]
	if !ok {
		return "", database.ErrMediaNotFound
	}
	return video, nil
}

func (m stubSponsoredMedia) GetCopy(userId, key string) (domain.MediaCopy, error) {
	c, ok := m.copies[userId+"/"+key]
	if !ok {
		return domain.MediaCopy{}, database.ErrMediaNotFound
	}
	return c, nil
}

func (m stubSponsoredMedia) SetCopy(userId, key string, c domain.MediaCopy) error {
	m.copies[userId+"/"+key] = c
	return nil
}

func (m stubSponsoredMedia) SetImage(string, domain.Base64Image) (domain.ImageKey, error) {
	return "", nil
}

func (m stubSponsoredMedia) SetVideo(string, domain.Base64Video) (domain.VideoKey, error) {
	return "", nil
}

func (m stubSponsoredMedia) SetForeignImageWithTTL(userId, key string, img domain.Base64Image) error {
	m.images[userId+"/"+key] = img
	return nil
}

func (m stubSponsoredMedia) SetForeignVideoWithTTL(userId, key string, video domain.Base64Video) error {
	m.videos[userId+"/"+key] = video
	return nil
}

func testIdentityKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return key
}

func authorNode(t *testing.T, onVerify func(event.VerifyOrderEvent) ([]byte, error)) stubStreamer {
	t.Helper()
	return stubStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
		assert.Equal(t, "author-node", nodeId)
		switch path {
		case event.PUBLIC_GET_TWEET:
			return json.Marshal(domain.Tweet{Id: "tweet-1", UserId: "author-1", Price: sponsoredPrice()})
		case event.PUBLIC_GET_WALLET_ADDRESS:
			return json.Marshal(event.WalletAddressResponse{Address: "TAuthorAddress", Chain: "testnet", UserId: "author-1"})
		case event.PUBLIC_POST_SPONSORED_ORDER:
			return onVerify(data.(event.VerifyOrderEvent))
		}
		t.Fatalf("unexpected stream to %s", path)
		return nil, nil
	}}
}

func TestStreamNewOrderHandler(t *testing.T) {
	owner := domain.Owner{UserId: "buyer-1", Username: "bob"}
	users := stubSponsoredUsers{"author-1": {Id: "author-1", NodeId: "author-node"}}
	confirm := func(ev event.VerifyOrderEvent) ([]byte, error) {
		return json.Marshal(event.OrderResponse{TweetId: ev.TweetId, TxId: ev.TxId, Confirmed: true})
	}
	newHandler := func(w SponsoredWallet, orders OrderStorer, streamer SponsoredStreamer) warpnet.WarpHandlerFunc {
		return StreamNewOrderHandler(stubAuth{owner: owner}, testIdentityKey(t), w, orders, users, streamer)
	}

	for _, tt := range []struct {
		name string
		ev   event.NewOrderEvent
		want string
	}{
		{"empty tweet id", event.NewOrderEvent{UserId: "author-1"}, "order: empty tweet id"},
		{"empty user id", event.NewOrderEvent{TweetId: "tweet-1"}, "order: empty user id"},
		{"own tweet", event.NewOrderEvent{TweetId: "tweet-1", UserId: "buyer-1"}, "order: an own tweet is not for sale"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newHandler(&stubSponsoredWallet{}, stubOrders{}, stubStreamer{})(marshal(t, tt.ev), nil)
			assert.EqualError(t, err, tt.want)
		})
	}

	t.Run("pays the author's price and claims the tweet", func(t *testing.T) {
		w := &stubSponsoredWallet{splitter: "TSplitter", payTx: "tx-1"}
		orders := stubOrders{}
		var claimed event.VerifyOrderEvent
		resp, err := newHandler(w, orders, authorNode(t, func(ev event.VerifyOrderEvent) ([]byte, error) {
			claimed = ev
			return confirm(ev)
		}))(marshal(t, event.NewOrderEvent{TweetId: "tweet-1", UserId: "author-1"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.OrderResponse{TweetId: "tweet-1", TxId: "tx-1", Confirmed: true}, resp)

		require.Len(t, w.pays, 1)
		saved := orders["tweet-1/buyer-1"]
		assert.Equal(t, wallet.Sponsorship{
			Splitter: "TSplitter", OrderId: saved.ID(), Author: "TAuthorAddress", Amount: "1500000", MaxFeePercent: 5,
		}, w.pays[0])
		assert.Len(t, saved.ID(), 64)
		assert.True(t, saved.Confirmed)
		assert.Equal(t, event.VerifyOrderEvent{TweetId: "tweet-1", UserId: "buyer-1", TxId: "tx-1", Nonce: saved.Nonce}, claimed)
	})

	t.Run("a saved receipt is claimed again, never paid twice", func(t *testing.T) {
		w := &stubSponsoredWallet{splitter: "TSplitter"}
		orders := stubOrders{"tweet-1/buyer-1": {TweetId: "tweet-1", BuyerId: "buyer-1", Nonce: "n1", TxId: "tx-0"}}
		resp, err := newHandler(w, orders, authorNode(t, confirm))(marshal(t, event.NewOrderEvent{TweetId: "tweet-1", UserId: "author-1"}), nil)
		require.NoError(t, err)
		assert.Empty(t, w.pays)
		assert.True(t, resp.(event.OrderResponse).Confirmed)
	})

	t.Run("an offline author leaves the paid receipt pending", func(t *testing.T) {
		w := &stubSponsoredWallet{splitter: "TSplitter", payTx: "tx-2"}
		orders := stubOrders{}
		resp, err := newHandler(w, orders, authorNode(t, func(event.VerifyOrderEvent) ([]byte, error) {
			return nil, warpnet.ErrNodeIsOffline
		}))(marshal(t, event.NewOrderEvent{TweetId: "tweet-1", UserId: "author-1"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.OrderResponse{TweetId: "tweet-1", TxId: "tx-2", Confirmed: false}, resp)
		assert.Equal(t, "tx-2", orders["tweet-1/buyer-1"].TxId)
	})

	t.Run("a network without a splitter takes no payment", func(t *testing.T) {
		w := &stubSponsoredWallet{}
		_, err := newHandler(w, stubOrders{}, authorNode(t, confirm))(marshal(t, event.NewOrderEvent{TweetId: "tweet-1", UserId: "author-1"}), nil)
		assert.EqualError(t, err, "order: sponsored payments are not open on testnet")
		assert.Empty(t, w.pays)
	})
}

func TestStreamGetOrderQuoteHandler(t *testing.T) {
	owner := domain.Owner{UserId: "buyer-1", Username: "bob"}
	users := stubSponsoredUsers{"author-1": {Id: "author-1", NodeId: "author-node"}}
	newHandler := func(w SponsoredWallet, streamer SponsoredStreamer) warpnet.WarpHandlerFunc {
		return StreamGetOrderQuoteHandler(stubAuth{owner: owner}, testIdentityKey(t), w, users, streamer)
	}
	noVerify := func(event.VerifyOrderEvent) ([]byte, error) {
		t.Fatal("a quote must not claim anything")
		return nil, nil
	}

	for _, tt := range []struct {
		name string
		ev   event.GetOrderQuoteEvent
		want string
	}{
		{"empty tweet id", event.GetOrderQuoteEvent{UserId: "author-1"}, "order quote: empty tweet id"},
		{"empty user id", event.GetOrderQuoteEvent{TweetId: "tweet-1"}, "order quote: empty user id"},
		{"own tweet", event.GetOrderQuoteEvent{TweetId: "tweet-1", UserId: "buyer-1"}, "order quote: an own tweet is not for sale"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newHandler(&stubSponsoredWallet{}, stubStreamer{})(marshal(t, tt.ev), nil)
			assert.EqualError(t, err, tt.want)
		})
	}

	t.Run("quotes the author's price without paying", func(t *testing.T) {
		w := &stubSponsoredWallet{splitter: "TSplitter", quote: wallet.Quote{
			Token: "TToken", FeePercent: 5, Fee: "75000", Total: "1575000", Balance: "3000000", TRX: "25000000",
			EnergyPrice: 100, BandwidthPrice: 1000, NetworkFee: "6726600",
			Steps: []wallet.QuoteStep{
				{Kind: "approve", Bandwidth: 345, Burn: "0"},
				{Kind: "pay", Energy: 63156, Bandwidth: 411, Burn: "6726600", Approximate: true},
			},
		}}
		resp, err := newHandler(w, authorNode(t, noVerify))(marshal(t, event.GetOrderQuoteEvent{TweetId: "tweet-1", UserId: "author-1"}), nil)
		require.NoError(t, err)

		require.Len(t, w.quotes, 1)
		assert.Equal(t, wallet.Sponsorship{Splitter: "TSplitter", Author: "TAuthorAddress", Amount: "1500000", MaxFeePercent: 5}, w.quotes[0])
		assert.Empty(t, w.pays)
		assert.Equal(t, event.OrderQuoteResponse{
			Token: "TToken", FeePercent: 5, Fee: "75000", Total: "1575000", Balance: "3000000", TRX: "25000000",
			EnergyPrice: 100, BandwidthPrice: 1000, NetworkFee: "6726600",
			Steps: []event.OrderQuoteStep{
				{Kind: "approve", Bandwidth: 345, Burn: "0"},
				{Kind: "pay", Energy: 63156, Bandwidth: 411, Burn: "6726600", Approximate: true},
			},
		}, resp)
	})

	t.Run("a network without a splitter has nothing to quote", func(t *testing.T) {
		w := &stubSponsoredWallet{}
		_, err := newHandler(w, authorNode(t, noVerify))(marshal(t, event.GetOrderQuoteEvent{TweetId: "tweet-1", UserId: "author-1"}), nil)
		assert.EqualError(t, err, "order: sponsored payments are not open on testnet")
		assert.Empty(t, w.quotes)
	})
}

func TestStreamVerifyOrderHandler(t *testing.T) {
	owner := domain.Owner{UserId: "author-1", Username: "alice"}
	buyerNode := newTestPeerID(t)
	users := stubSponsoredUsers{"buyer-1": {Id: "buyer-1", NodeId: buyerNode.String()}}
	paidTweet := stubTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
		return domain.Tweet{Id: tweetID, UserId: userID, Text: "paid", Price: sponsoredPrice()}, nil
	}}
	claim := event.VerifyOrderEvent{TweetId: "tweet-1", UserId: "buyer-1", TxId: "tx-1", Nonce: "n1"}
	fromBuyer := func() warpnet.WarpStream {
		_, s := stream.NewLoopbackStream(newTestPeerID(t), buyerNode, event.PUBLIC_POST_SPONSORED_ORDER)
		return s
	}
	newHandler := func(w SponsoredWallet, tweets SponsoredTweetFetcher, orders OrderStorer) warpnet.WarpHandlerFunc {
		return StreamVerifyOrderHandler(stubAuth{owner: owner}, testIdentityKey(t), w, tweets, orders, users, stubStreamer{})
	}

	t.Run("confirms a payment bound to this buyer and records it", func(t *testing.T) {
		w := &stubSponsoredWallet{splitter: "TSplitter", isPaid: true}
		orders := stubOrders{}
		resp, err := newHandler(w, paidTweet, orders)(marshal(t, claim), fromBuyer())
		require.NoError(t, err)
		assert.True(t, resp.(event.OrderResponse).Confirmed)

		want := domain.Order{TweetId: "tweet-1", BuyerId: "buyer-1", Nonce: "n1"}
		require.Len(t, w.checks, 1)
		assert.Equal(t, wallet.Sponsorship{Splitter: "TSplitter", OrderId: want.ID(), Author: "TAuthorAddress", Amount: "1500000"}, w.checks[0])
		assert.True(t, orders["tweet-1/buyer-1"].Confirmed)
	})

	t.Run("a pending payment is not recorded", func(t *testing.T) {
		orders := stubOrders{}
		resp, err := newHandler(&stubSponsoredWallet{splitter: "TSplitter"}, paidTweet, orders)(marshal(t, claim), fromBuyer())
		require.NoError(t, err)
		assert.False(t, resp.(event.OrderResponse).Confirmed)
		assert.Empty(t, orders)
	})

	t.Run("a claim from another node is refused", func(t *testing.T) {
		_, stranger := stream.NewLoopbackStream(newTestPeerID(t), newTestPeerID(t), event.PUBLIC_POST_SPONSORED_ORDER)
		w := &stubSponsoredWallet{splitter: "TSplitter", isPaid: true}
		_, err := newHandler(w, paidTweet, stubOrders{})(marshal(t, claim), stranger)
		assert.ErrorIs(t, err, warpnet.ErrForeignAuthor)
		assert.Empty(t, w.checks)
	})

	t.Run("a free tweet is not for sale", func(t *testing.T) {
		_, err := newHandler(&stubSponsoredWallet{splitter: "TSplitter"}, stubTweetRepo{}, stubOrders{})(marshal(t, claim), fromBuyer())
		assert.EqualError(t, err, "order: tweet is not sponsored")
	})

	t.Run("a confirmed order is not verified again", func(t *testing.T) {
		w := &stubSponsoredWallet{splitter: "TSplitter"}
		orders := stubOrders{"tweet-1/buyer-1": {TweetId: "tweet-1", BuyerId: "buyer-1", TxId: "tx-1", Confirmed: true}}
		resp, err := newHandler(w, paidTweet, orders)(marshal(t, claim), fromBuyer())
		require.NoError(t, err)
		assert.True(t, resp.(event.OrderResponse).Confirmed)
		assert.Empty(t, w.checks)
	})

	for _, tt := range []struct {
		name string
		ev   event.VerifyOrderEvent
		want string
	}{
		{"empty tweet id", event.VerifyOrderEvent{UserId: "buyer-1", TxId: "tx", Nonce: "n"}, "order: empty tweet id"},
		{"empty user id", event.VerifyOrderEvent{TweetId: "t", TxId: "tx", Nonce: "n"}, "order: empty user id"},
		{"empty tx id", event.VerifyOrderEvent{TweetId: "t", UserId: "buyer-1", Nonce: "n"}, "order: empty tx id"},
		{"empty nonce", event.VerifyOrderEvent{TweetId: "t", UserId: "buyer-1", TxId: "tx"}, "order: empty nonce"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newHandler(&stubSponsoredWallet{}, paidTweet, stubOrders{})(marshal(t, tt.ev), fromBuyer())
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
		orders := stubOrders{"tweet-1/buyer-1": {TweetId: "tweet-1", BuyerId: "buyer-1", Confirmed: true}}
		media := newStubSponsoredMedia()
		image, _ := watermarkedImage(t, "author-1")
		media.images["author-1/img-1"] = domain.Base64Image(image)
		h := StreamGetSponsoredTweetHandler(stubAuth{owner: domain.Owner{UserId: "author-1"}}, testSignerKey, tweets, orders, media, newStubSponsoredMedia(), users,
			stubStreamer{nodeInfo: warpnet.NodeInfo{ID: own, OwnerId: "author-1"}})

		resp, err := h(req, streamFrom(own))
		require.NoError(t, err)
		assert.Equal(t, full, resp, "the author reads the own tweet")

		resp, err = h(req, streamFrom(buyerNode))
		require.NoError(t, err)
		bought := resp.(domain.Tweet)
		assert.Equal(t, "paid", bought.Text, "a confirmed buyer gets the full tweet")
		require.Len(t, bought.ImageKeys, 1)
		assert.NotEqual(t, "img-1", bought.ImageKeys[0], "a confirmed buyer gets a copy of the image, not the original")

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
				if path == event.PUBLIC_POST_SPONSORED_ORDER {
					return json.Marshal(event.OrderResponse{TweetId: "tweet-1", TxId: "tx-1", Confirmed: confirmed})
				}
				return json.Marshal(full)
			},
		}

		resp, err := StreamGetSponsoredTweetHandler(auth, testSignerKey, tweets, stubOrders{}, newStubSponsoredMedia(), newStubSponsoredMedia(), users, streamer)(req, streamFrom(own))
		require.NoError(t, err)
		assert.Equal(t, domain.Tweet{}, resp, "nothing bought, nothing shown")
		assert.Empty(t, paths, "nothing bought, nothing asked")

		orders := stubOrders{"tweet-1/buyer-1": {TweetId: "tweet-1", BuyerId: "buyer-1", TxId: "tx-1", Nonce: "n1"}}
		h := StreamGetSponsoredTweetHandler(auth, testSignerKey, tweets, orders, newStubSponsoredMedia(), newStubSponsoredMedia(), users, streamer)

		confirmed = false
		resp, err = h(req, streamFrom(own))
		require.NoError(t, err, "a pending receipt is a state, not a failure the node logs")
		assert.Equal(t, event.OrderResponse{TweetId: "tweet-1", TxId: "tx-1"}, resp, "a receipt the author has not confirmed yet is still pending")

		confirmed = true
		paths = nil
		resp, err = h(req, streamFrom(own))
		require.NoError(t, err)
		assert.Equal(t, "paid", resp.(domain.Tweet).Text, "opening the tweet claims the receipt again")
		assert.Equal(t, []stream.WarpRoute{event.PUBLIC_POST_SPONSORED_ORDER, event.PUBLIC_GET_SPONSORED_TWEET}, paths)
		assert.Equal(t, "paid", orders["tweet-1/buyer-1"].Tweet.Text, "the bought tweet is kept with the order")

		paths = nil
		_, err = h(req, streamFrom(own))
		require.NoError(t, err)
		assert.Empty(t, paths, "a kept tweet needs no author")

		resp, err = h(req, streamFrom(strangerNode))
		require.NoError(t, err)
		assert.Equal(t, domain.Tweet{}, resp, "a buyer never passes the tweet on")
	})
}

func TestSponsoredCopy_NamesItsBuyer(t *testing.T) {
	own, buyerNode := newTestPeerID(t), newTestPeerID(t)
	image, imageKey := watermarkedImage(t, "author-1")
	video, videoKey := watermarkedVideo(t, "author-1")
	media := newStubSponsoredMedia()
	media.images["author-1/"+imageKey] = domain.Base64Image(image)
	media.videos["author-1/"+videoKey] = domain.Base64Video(video)
	copies := newStubSponsoredMedia()

	full := domain.Tweet{Id: "tweet-1", UserId: "author-1", Text: "paid", ImageKeys: []string{imageKey}, VideoKey: &videoKey, Price: sponsoredPrice()}
	tweets := stubTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) { return full, nil }}
	order := domain.Order{
		TweetId: "tweet-1", AuthorId: "author-1", BuyerId: "buyer-1", Nonce: "n1", TxId: "tx-1",
		Confirmed: true, CreatedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
	}
	users := stubSponsoredUsers{"buyer-1": {Id: "buyer-1", NodeId: buyerNode.String()}}
	streamer := stubStreamer{nodeInfo: warpnet.NodeInfo{ID: own, OwnerId: "author-1"}}
	fromBuyer := func(route warpnet.WarpProtocolID) warpnet.WarpStream {
		_, s := stream.NewLoopbackStream(own, buyerNode, route)
		return s
	}

	resp, err := StreamGetSponsoredTweetHandler(stubAuth{owner: domain.Owner{UserId: "author-1"}}, testSignerKey, tweets,
		stubOrders{"tweet-1/buyer-1": order}, media, copies, users, streamer,
	)(marshal(t, event.GetTweetEvent{UserId: "author-1", TweetId: "tweet-1"}), fromBuyer(event.PUBLIC_GET_SPONSORED_TWEET))
	require.NoError(t, err)
	bought := resp.(domain.Tweet)
	require.Len(t, bought.ImageKeys, 1)
	require.NotNil(t, bought.VideoKey)
	assert.NotEqual(t, imageKey, bought.ImageKeys[0], "the buyer never learns the original's key")
	assert.NotEqual(t, videoKey, *bought.VideoKey, "the buyer never learns the original's key")

	author := domain.User{Id: "author-1", NodeId: testSignerID.String()}
	traceBuyer := StreamGetCopyBuyerHandler(testSignerKey)
	want := event.CopyBuyerResponse{TweetId: "tweet-1", BuyerId: "buyer-1", OrderId: order.ID(), TxId: "tx-1", SoldAt: order.CreatedAt}

	served, err := StreamGetSponsoredImageHandler(streamer, media, copies, users)(
		marshal(t, event.GetImageEvent{UserId: "author-1", Key: bought.ImageKeys[0]}), fromBuyer(event.PUBLIC_GET_SPONSORED_IMAGE))
	require.NoError(t, err)
	copied := served.(event.GetImageResponse).File
	assert.NoError(t, verifyForeignImage(author, bought.ImageKeys[0], copied), "the buyer's node takes the copy for the author's image")
	traced, err := traceBuyer(marshal(t, event.GetCopyBuyerEvent{File: copied}), nil)
	require.NoError(t, err)
	assert.Equal(t, want, traced)

	served, err = StreamGetSponsoredVideoHandler(streamer, media, copies, users)(
		marshal(t, event.GetVideoEvent{UserId: "author-1", Key: *bought.VideoKey}), fromBuyer(event.PUBLIC_GET_SPONSORED_VIDEO))
	require.NoError(t, err)
	copied = served.(event.GetVideoResponse).File
	assert.NoError(t, verifyForeignVideo(author, *bought.VideoKey, copied), "the buyer's node takes the copy for the author's video")
	traced, err = traceBuyer(marshal(t, event.GetCopyBuyerEvent{File: copied}), nil)
	require.NoError(t, err)
	assert.Equal(t, want, traced)
}

func TestGetSponsoredImage_OnlyForItsBuyer(t *testing.T) {
	own, buyerNode, strangerNode := newTestPeerID(t), newTestPeerID(t), newTestPeerID(t)
	image, imageKey := watermarkedImage(t, "author-1")
	media := newStubSponsoredMedia()
	media.images["author-1/"+imageKey] = domain.Base64Image(image)
	copies := newStubSponsoredMedia()
	copies.copies["author-1/copy-1"] = domain.MediaCopy{OriginalKey: imageKey, BuyerId: "buyer-1", EncryptedOrder: []byte("sealed")}
	users := stubSponsoredUsers{
		"buyer-1":    {Id: "buyer-1", NodeId: buyerNode.String()},
		"stranger-1": {Id: "stranger-1", NodeId: strangerNode.String()},
	}
	streamer := stubStreamer{nodeInfo: warpnet.NodeInfo{ID: own, OwnerId: "author-1"}}
	get := func(h warpnet.WarpHandlerFunc, key string, remote warpnet.WarpPeerID) string {
		_, s := stream.NewLoopbackStream(own, remote, event.PUBLIC_GET_SPONSORED_IMAGE)
		resp, err := h(marshal(t, event.GetImageEvent{UserId: "author-1", Key: key}), s)
		require.NoError(t, err)
		return resp.(event.GetImageResponse).File
	}

	h := StreamGetSponsoredImageHandler(streamer, media, copies, users)
	assert.NotEmpty(t, get(h, "copy-1", buyerNode), "the buyer gets the copy")
	assert.Empty(t, get(h, "copy-1", strangerNode), "someone else who knows the key gets nothing")
	assert.Empty(t, get(h, "copy-1", newTestPeerID(t)), "an unknown node gets nothing")
	assert.Empty(t, get(h, imageKey, buyerNode), "the route gives no originals")

	assert.Empty(t, get(StreamGetImageHandler(streamer, media, users), "copy-1", buyerNode), "the image route gives no copies")
}

func TestGetSponsoredImage_OnTheBuyersNode(t *testing.T) {
	own := newTestPeerID(t)
	image, _ := watermarkedImage(t, "author-1")
	copied, err := markFile(image, []byte("sealed"), media_meta.EmbedOrderInJPEG)
	require.NoError(t, err)
	copyKey := contentKey(copied)
	users := stubSponsoredUsers{"author-1": {Id: "author-1", NodeId: testSignerID.String()}}
	var asked []stream.WarpRoute
	answer := image
	streamer := stubStreamer{
		nodeInfo: warpnet.NodeInfo{ID: own, OwnerId: "buyer-1"},
		genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			assert.Equal(t, testSignerID.String(), nodeId)
			asked = append(asked, path)
			return json.Marshal(event.GetImageResponse{File: answer})
		},
	}
	copies := newStubSponsoredMedia()
	h := StreamGetSponsoredImageHandler(streamer, newStubSponsoredMedia(), copies, users)
	get := func(remote warpnet.WarpPeerID) string {
		_, s := stream.NewLoopbackStream(own, remote, event.PUBLIC_GET_SPONSORED_IMAGE)
		resp, err := h(marshal(t, event.GetImageEvent{UserId: "author-1", Key: copyKey}), s)
		require.NoError(t, err)
		return resp.(event.GetImageResponse).File
	}

	assert.Empty(t, get(own), "a file that is not the copy is refused")
	assert.Empty(t, copies.images, "a refused file is not kept")

	answer, asked = copied, nil
	assert.Equal(t, copied, get(own), "the buyer gets the copy from the author's node")
	assert.Equal(t, []stream.WarpRoute{event.PUBLIC_GET_SPONSORED_IMAGE}, asked)
	assert.Equal(t, domain.Base64Image(copied), copies.images["author-1/"+copyKey], "the buyer's node keeps the copy")

	asked = nil
	assert.Equal(t, copied, get(own))
	assert.Empty(t, asked, "a kept copy needs no author")

	assert.Empty(t, get(newTestPeerID(t)), "a buyer never passes the copy on")
	assert.Empty(t, asked)
}

func TestStreamGetCopyBuyerHandler_UnmarkedFile(t *testing.T) {
	image, _ := watermarkedImage(t, "author-1")
	h := StreamGetCopyBuyerHandler(testSignerKey)

	resp, err := h(marshal(t, event.GetCopyBuyerEvent{File: image}), nil)
	require.NoError(t, err)
	assert.Equal(t, event.CopyBuyerResponse{}, resp, "the original names no buyer")

	sealed, err := security.EncryptAES([]byte(`{"buyer_id":"buyer-1"}`), testIdentityKey(t))
	require.NoError(t, err)
	foreign, err := markFile(image, sealed, media_meta.EmbedOrderInJPEG)
	require.NoError(t, err)
	_, err = h(marshal(t, event.GetCopyBuyerEvent{File: foreign}), nil)
	assert.EqualError(t, err, "sponsored buyer: this node did not sell the copy")
}
