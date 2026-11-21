package handler

import (
	"github.com/Warp-net/warpnet/core/bridge"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
	"strings"
	"sync/atomic"
	"time"
)

type WhoToFollowAuthStorer interface {
	GetOwner() domain.Owner
}

type WhoToFollowUserFetcher interface {
	Get(userId string) (user domain.User, err error)
	WhoToFollow(limit *uint64, cursor *string) ([]domain.User, string, error)
}

type WhoToFollowFollowsLister interface {
	GetFollowings(userId string, limit *uint64, cursor *string) ([]string, string, error)
}

func StreamGetWhoToFollowHandler(
	authRepo WhoToFollowAuthStorer,
	userRepo WhoToFollowUserFetcher,
	followRepo WhoToFollowFollowsLister,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAllUsersEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}

		users, cursor, err := userRepo.WhoToFollow(ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}

		followingsLimit := uint64(80) //nolint:mnd    // TODO limit?
		followings, _, err := followRepo.GetFollowings(authRepo.GetOwner().UserId, &followingsLimit, nil)
		if err != nil {
			log.Errorf("get who to follow handler: get followers %v", err)
		}

		followedUsers := map[string]struct{}{}
		for _, followingId := range followings {
			followedUsers[followingId] = struct{}{}
		}

		owner := authRepo.GetOwner()

		profile, err := userRepo.Get(ev.UserId)
		if err != nil {
			log.Errorf("get who to follow handler: get user %v", err)
			profile = domain.User{
				Id:       owner.UserId,
				Username: owner.Login,
				Network:  warpnet.WarpnetName,
				NodeId:   owner.NodeId,
			}
		}

		whotofollow := make([]domain.User, 0, len(users))
		latestByNode := make(map[string]int, len(users))
		for _, user := range users {
			if user.IsOffline { // exclude offline
				continue
			}
			if bridge.IsFederatedWarpnet(user) {
				continue
			}

			if strings.Contains(user.Id, owner.UserId) || user.NodeId == owner.NodeId { // exclude me
				continue
			}
			// if profile from Warpnet - don't show other network recommendations
			if profile.Id != owner.UserId && profile.Network != user.Network {
				continue
			}
			if _, ok := followedUsers[user.Id]; ok { // exclude already followed
				continue
			}

			if user.NodeId != "" && !bridge.IsBridged(user.Network) {
				if idx, ok := latestByNode[user.NodeId]; ok {
					if user.CreatedAt.After(whotofollow[idx].CreatedAt) {
						whotofollow[idx] = user
					}
					continue
				}
			}

			latestByNode[user.NodeId] = len(whotofollow)

			whotofollow = append(whotofollow, user)
		}

		return event.UsersResponse{
			Cursor: cursor,
			Users:  whotofollow,
		}, nil
	}
}

const (
	bridgedSuggestionsEvery = 10 * time.Minute
	bridgedSuggestionsRetry = 30 * time.Second
	bridgedSuggestionsWait  = 3 * time.Second
)

// WithBridgedSuggestions refreshes the owner's suggestions from the gateway
// before answering, at most once per bridgedSuggestionsEvery, or again after
// bridgedSuggestionsRetry when the gateway was not reached yet. Who-to-follow
// reads only this node's users, so without it a new node offered no Mastodon
// or Nostr accounts until its owner happened to open the gateway's profile. A
// slow gateway holds the answer for bridgedSuggestionsWait at most; what it
// sends later is stored for the next request.
func WithBridgedSuggestions(h warpnet.WarpHandlerFunc, userRepo UserFetcher, streamer UserStreamer) warpnet.WarpHandlerFunc {
	var last atomic.Int64
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAllUsersEvent
		if json.Unmarshal(buf, &ev) == nil && ev.UserId != "" && ev.UserId == streamer.NodeInfo().OwnerId {
			prev, now := last.Load(), time.Now().UnixNano()
			if time.Duration(now-prev) >= bridgedSuggestionsEvery && last.CompareAndSwap(prev, now) {
				done := make(chan struct{})
				go func() {
					defer close(done)
					gatewayId := bridge.GatewayNodeID()
					users, err := requestUsers(streamer, gatewayId, event.PUBLIC_GET_WHOTOFOLLOW, ev)
					if err != nil {
						log.Warnf("who to follow: bridged suggestions: %v", err)
						last.Store(now - int64(bridgedSuggestionsEvery-bridgedSuggestionsRetry))
						return
					}
					storeUsers(userRepo, users, gatewayId, streamer.NodeInfo())
				}()
				select {
				case <-done:
				case <-time.After(bridgedSuggestionsWait):
				}
			}
		}
		return h(buf, s)
	}
}
