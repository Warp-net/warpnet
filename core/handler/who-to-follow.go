package handler

import (
	"github.com/Warp-net/warpnet/core/bridge"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"strings"
	"sync"
)

var recommendedNetworks = []string{
	warpnet.WarpnetName,
	bridge.MastodonNetwork,
	bridge.ThreadsNetwork,
	bridge.NostrNetwork,
}

type WhoToFollowAuthStorer interface {
	GetOwner() domain.Owner
}

type WhoToFollowFollowsLister interface {
	GetFollowings(userId string, limit *uint64, cursor *string) ([]string, string, error)
}

func StreamGetWhoToFollowHandler(
	authRepo WhoToFollowAuthStorer,
	userRepo UserFetcher,
	followRepo WhoToFollowFollowsLister,
	streamer UserStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetWhoToFollowEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}

		cursors := ev.Cursors
		if len(cursors) == 0 && ev.Cursor != nil && *ev.Cursor != "" {
			cursors = map[string]string{warpnet.WarpnetName: *ev.Cursor}
		}

		// Bridged accounts reach this node only from the gateway's suggestions.
		if ev.UserId == streamer.NodeInfo().OwnerId && len(cursors) == 0 {
			go refreshUsers(userRepo, ev.GetAllUsersEvent, streamer)
		}

		if len(cursors) == 0 {
			cursors = make(map[string]string, len(recommendedNetworks))
			for _, network := range recommendedNetworks {
				cursors[network] = ""
			}
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

		var (
			g    errgroup.Group
			mx   sync.Mutex
			resp = event.WhoToFollowResponse{
				Networks: make(map[string]event.UsersResponse, len(cursors)),
			}
		)
		for network, cursor := range cursors {
			g.Go(func() error {
				users, next, err := userRepo.WhoToFollow(network, ev.Limit, &cursor)
				if err != nil {
					return err
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

				mx.Lock()
				resp.Networks[network] = event.UsersResponse{Cursor: next, Users: whotofollow}
				mx.Unlock()
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return nil, err
		}

		resp.UsersResponse = resp.Networks[warpnet.WarpnetName]
		if resp.Users == nil {
			resp.Users = []domain.User{}
		}
		return resp, nil
	}
}
