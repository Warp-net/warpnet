package handler

import (
	"github.com/Warp-net/warpnet/core/mastodon"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

func StreamGetWhoToFollowHandler(
	authRepo UserAuthStorer,
	userRepo UserFetcher,
	followRepo UserFollowsCounter,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAllUsersEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		if ev.UserId == "" {
			return nil, errEmptyUserId
		}

		// Candidates come back in RTT-key order with the BadgerDB cursor
		// threaded through, so a "new" account (empty graph) gets the
		// nearest-by-RTT peers for free and pagination stays cursor-based.
		users, cursor, err := userRepo.WhoToFollow(ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}

		followingsLimit := uint64(80) //nolint:mnd    // TODO limit?
		followings, _, err := followRepo.GetFollowings(authRepo.GetOwner().UserId, &followingsLimit, nil)
		if err != nil {
			log.Errorf("get who to follow handler: get followings %v", err)
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
				Username: owner.Username,
				Network:  warpnet.WarpnetName,
				NodeId:   owner.NodeId,
			}
		}

		whotofollow := make([]domain.User, 0, len(users))
		latestByNode := make(map[string]int, len(users))

		// Pin the Mastodon entry account on top of the owner's first page
		// until it is followed. It is seeded without an avatar/tweets, so it
		// never survives the candidate filters - surface it explicitly.
		isOwnerView := ev.UserId == owner.UserId
		isFirstPage := ev.Cursor == nil || *ev.Cursor == ""
		if isOwnerView && isFirstPage {
			if entry, err := userRepo.Get(mastodon.EntryHandle); err == nil && entry.Id != "" {
				if _, followed := followedUsers[entry.Id]; !followed && entry.Id != owner.UserId {
					whotofollow = append(whotofollow, entry)
				}
			}
		}

		for _, user := range users {
			if user.IsOffline { // exclude offline
				continue
			}
			if user.Id == owner.UserId || user.NodeId == owner.NodeId { // exclude me
				continue
			}
			// if profile from Warpnet - don't show other network recommendations
			if profile.Id != owner.UserId && profile.Network != user.Network {
				continue
			}
			if _, ok := followedUsers[user.Id]; ok { // exclude already followed
				continue
			}
			if user.Id == mastodon.EntryHandle { // already pinned above
				continue
			}

			if user.NodeId != "" && user.Network != mastodon.Network {
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
