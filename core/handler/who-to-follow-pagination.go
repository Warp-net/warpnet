package handler

import (
	"encoding/base64"
	"strings"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

const (
	whoToFollowCursorPrefix = "recommendations:"
	whoToFollowEndCursor    = "end"
)

type whoToFollowCursor struct {
	Remote  string   `json:"remote"`
	Pending []string `json:"pending,omitempty"`
	Skip    []string `json:"skip,omitempty"`
}

func decodeWhoToFollowCursor(cursor *string) (whoToFollowCursor, bool, error) {
	var page whoToFollowCursor
	if cursor == nil || !strings.HasPrefix(*cursor, whoToFollowCursorPrefix) {
		return page, false, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(*cursor, whoToFollowCursorPrefix))
	if err != nil {
		return page, true, err
	}
	err = json.Unmarshal(data, &page)
	return page, true, err
}

func (page whoToFollowCursor) encode() string {
	if len(page.Pending) == 0 && (page.Remote == "" || page.Remote == whoToFollowEndCursor) {
		return whoToFollowEndCursor
	}
	data, _ := json.Marshal(page)
	return whoToFollowCursorPrefix + base64.RawURLEncoding.EncodeToString(data)
}

func firstWhoToFollowPage(remote event.UsersResponse, shown []domain.User, filter func([]domain.User) []domain.User) whoToFollowCursor {
	page := whoToFollowCursor{Remote: remote.Cursor}
	seen := make(map[string]bool, len(shown))
	for _, user := range shown {
		seen[user.Id] = true
		page.Skip = append(page.Skip, user.Id)
	}
	for _, user := range filter(remote.Users) {
		if !seen[user.Id] {
			page.Pending = append(page.Pending, user.Id)
			seen[user.Id] = true
		}
	}
	return page
}

func nextWhoToFollowPage(repo WhoToFollowUserFetcher, streamer WhoToFollowStreamer, ev event.GetAllUsersEvent, page whoToFollowCursor, filter func([]domain.User) []domain.User) (event.UsersResponse, error) {
	limit := uint64(20) //nolint:mnd // Match the local recommendation page size.
	if ev.Limit != nil && *ev.Limit > 0 {
		limit = *ev.Limit
	}
	users := make([]domain.User, 0)
	seen := make(map[string]bool, len(page.Skip))
	for _, id := range page.Skip {
		seen[id] = true
	}
	for {
		var candidates []domain.User
		for _, id := range page.Pending {
			user, err := repo.Get(id)
			if err != nil {
				return event.UsersResponse{}, err
			}
			candidates = append(candidates, user)
		}
		page.Pending = nil
		for _, user := range filter(candidates) {
			if seen[user.Id] {
				continue
			}
			seen[user.Id] = true
			if uint64(len(users)) < limit {
				users = append(users, user)
			} else {
				page.Pending = append(page.Pending, user.Id)
			}
		}
		if uint64(len(users)) >= limit || page.Remote == "" || page.Remote == whoToFollowEndCursor {
			return event.UsersResponse{Users: users, Cursor: page.encode()}, nil
		}
		remaining := limit - uint64(len(users))
		request := event.GetAllUsersEvent{UserId: ev.UserId, Limit: &remaining, Cursor: &page.Remote}
		remote, err := refreshWhoToFollow(repo, streamer, fediverse.GatewayNodeID(), request)
		if err != nil {
			return event.UsersResponse{}, err
		}
		if remote.Cursor == page.Remote {
			return event.UsersResponse{}, warpnet.WarpError("gateway recommendation cursor did not advance")
		}
		page.Remote = remote.Cursor
		for _, user := range remote.Users {
			page.Pending = append(page.Pending, user.Id)
		}
	}
}
