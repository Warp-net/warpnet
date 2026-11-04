package handler

import (
	"errors"
	"strings"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type WhoToFollowAuthStorer interface {
	GetOwner() domain.Owner
}

type WhoToFollowUserFetcher interface {
	Create(user domain.User) (domain.User, error)
	Update(userId string, user domain.User) (domain.User, error)
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
	streamers ...UserStreamer,
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

		owner := authRepo.GetOwner()
		if len(streamers) > 0 && streamers[0] != nil && ev.UserId == owner.UserId && (ev.Cursor == nil || *ev.Cursor == "") {
			refreshGatewayRecommendations(userRepo, streamers[0], owner)
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
		for _, user := range users {
			if user.IsOffline { // exclude offline
				continue
			}
			if fediverse.IsFederatedWarpnet(user) {
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

			if user.NodeId != "" && !fediverse.IsBridged(user.Network) {
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

// Store discovery candidates on the user node so profile and media requests can route them.
func refreshGatewayRecommendations(repo WhoToFollowUserFetcher, streamer UserStreamer, owner domain.Owner) {
	gatewayID := fediverse.GatewayNodeID()
	if gatewayID == owner.NodeId {
		return
	}
	limit := uint64(80)
	data, err := streamer.GenericStream(gatewayID, event.PUBLIC_GET_WHOTOFOLLOW, event.GetAllUsersEvent{UserId: owner.UserId, Limit: &limit})
	if err != nil {
		log.Warnf("who to follow: gateway discovery: %v", err)
		return
	}
	var remoteError event.ResponseError
	if err := json.Unmarshal(data, &remoteError); err != nil {
		log.Warnf("who to follow: gateway response: %v", err)
		return
	}
	if remoteError.Message != "" {
		log.Warnf("who to follow: gateway discovery: %s", remoteError.Message)
		return
	}
	var response event.UsersResponse
	if err := json.Unmarshal(data, &response); err != nil {
		log.Warnf("who to follow: gateway users: %v", err)
		return
	}
	for _, user := range response.Users[:min(len(response.Users), int(limit))] {
		if !fediverse.IsBridged(user.Network) || user.NodeId != gatewayID || user.Id == owner.UserId || user.IsOffline || fediverse.IsFederatedWarpnet(user) {
			continue
		}
		name, host, ok := strings.Cut(user.Id, "@")
		if !ok || name == "" || host == "" {
			continue
		}
		if _, err := repo.Create(user); errors.Is(err, database.ErrUserAlreadyExists) {
			_, err = repo.Update(user.Id, user)
			if err != nil {
				log.Warnf("who to follow: update discovered user: %v", err)
			}
		} else if err != nil {
			log.Warnf("who to follow: store discovered user: %v", err)
		}
	}
}
