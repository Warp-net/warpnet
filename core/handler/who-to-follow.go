package handler

import (
	"errors"
	"strings"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type WhoToFollowAuthStorer interface {
	GetOwner() domain.Owner
}

type WhoToFollowUserFetcher interface {
	Get(userId string) (domain.User, error)
	Create(user domain.User) (domain.User, error)
	Update(userId string, user domain.User) (domain.User, error)
	WhoToFollow(limit *uint64, cursor *string) ([]domain.User, string, error)
}

type WhoToFollowStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
}

type WhoToFollowFollowsLister interface {
	GetFollowings(userId string, limit *uint64, cursor *string) ([]string, string, error)
}

func StreamGetWhoToFollowHandler(
	authRepo WhoToFollowAuthStorer,
	userRepo WhoToFollowUserFetcher,
	followRepo WhoToFollowFollowsLister,
	streamer WhoToFollowStreamer,
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

		owner := authRepo.GetOwner()
		gatewayID := fediverse.GatewayNodeID()
		isOwnRequest := ev.UserId == owner.UserId
		isRemoteGateway := gatewayID != owner.NodeId
		isFirstPage := ev.Cursor == nil ||
			*ev.Cursor == ""
		if isOwnRequest && isRemoteGateway && isFirstPage {
			refreshWhoToFollow(userRepo, streamer, gatewayID, event.GetAllUsersEvent{UserId: owner.UserId})
			users, cursor, err = userRepo.WhoToFollow(ev.Limit, ev.Cursor)
			if err != nil {
				return nil, err
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

func refreshWhoToFollow(repo WhoToFollowUserFetcher, streamer WhoToFollowStreamer, nodeID string, ev event.GetAllUsersEvent) {
	data, err := streamer.GenericStream(nodeID, event.PUBLIC_GET_WHOTOFOLLOW, ev)
	if err != nil {
		if !errors.Is(err, warpnet.ErrNodeIsOffline) {
			log.Warnf("who to follow: stream to node %s: %v", nodeID, err)
		}
		return
	}
	var remoteError event.ResponseError
	if _ = json.Unmarshal(data, &remoteError); remoteError.Message != "" {
		log.Warnf("who to follow: remote response: %v", remoteError)
		return
	}
	var response event.UsersResponse
	if err := json.Unmarshal(data, &response); err != nil {
		log.Warnf("who to follow: unmarshal response: %v", err)
		return
	}
	for _, candidate := range response.Users {
		_, _ = repo.Create(candidate)
		_, _ = repo.Update(candidate.Id, candidate)
	}
}
