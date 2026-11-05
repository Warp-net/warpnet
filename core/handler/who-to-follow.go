package handler

import (
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

		owner := authRepo.GetOwner()
		isOwnRequest := ev.UserId == owner.UserId
		isFirstPage := ev.Cursor == nil ||
			*ev.Cursor == ""
		page, continuing, err := decodeWhoToFollowCursor(ev.Cursor)
		if err != nil {
			return nil, err
		}
		if continuing && !isOwnRequest {
			return nil, warpnet.WarpError("recommendation cursor belongs to the owner")
		}

		var users []domain.User
		var cursor string
		if !continuing {
			users, cursor, err = userRepo.WhoToFollow(ev.Limit, ev.Cursor)
			if err != nil {
				return nil, err
			}
		}

		var remote event.UsersResponse
		remoteAvailable := false
		if isOwnRequest && isFirstPage {
			remote, err = refreshWhoToFollow(userRepo, streamer, fediverse.GatewayNodeID(), ev)
			remoteAvailable = err == nil
			if err != nil {
				log.Warnf("who to follow: gateway discovery: %v", err)
			}
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

		filter := func(candidates []domain.User) []domain.User {
			return filterWhoToFollow(candidates, owner, profile, followedUsers)
		}
		if continuing {
			return nextWhoToFollowPage(userRepo, streamer, ev, page, filter)
		}
		users = filter(users)
		if remoteAvailable {
			page = firstWhoToFollowPage(remote, users, filter)
			cursor = page.encode()
			if len(users) == 0 && cursor != whoToFollowEndCursor {
				return nextWhoToFollowPage(userRepo, streamer, ev, page, filter)
			}
		}
		return event.UsersResponse{Cursor: cursor, Users: users}, nil
	}
}

func filterWhoToFollow(users []domain.User, owner domain.Owner, profile domain.User, followedUsers map[string]struct{}) []domain.User {
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

	return whotofollow
}

func refreshWhoToFollow(repo WhoToFollowUserFetcher, streamer WhoToFollowStreamer, nodeID string, ev event.GetAllUsersEvent) (event.UsersResponse, error) {
	data, err := streamer.GenericStream(nodeID, event.PUBLIC_GET_WHOTOFOLLOW, ev)
	if err != nil {
		return event.UsersResponse{}, err
	}
	var remoteError event.ResponseError
	if _ = json.Unmarshal(data, &remoteError); remoteError.Message != "" {
		return event.UsersResponse{}, warpnet.WarpError(remoteError.Message)
	}
	var response event.UsersResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return response, err
	}
	for _, candidate := range response.Users {
		_, _ = repo.Create(candidate)
		_, _ = repo.Update(candidate.Id, candidate)
	}
	return response, nil
}
