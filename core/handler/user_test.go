//nolint:all
package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubUserFetcher struct {
	createFn        func(user domain.User) (domain.User, error)
	getFn           func(userId string) (domain.User, error)
	listFn          func(limit *uint64, cursor *string) ([]domain.User, string, error)
	whoToFollowFn   func(limit *uint64, cursor *string) ([]domain.User, string, error)
	updateFn        func(userId string, newUser domain.User) (domain.User, error)
	createWithTTLFn func(user domain.User, ttl time.Duration) (domain.User, error)
}

func (s stubUserFetcher) Create(user domain.User) (domain.User, error) {
	if s.createFn != nil {
		return s.createFn(user)
	}
	return user, nil
}
func (s stubUserFetcher) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{Id: userId, NodeId: "node-2", Network: warpnet.WarpnetName}, nil
}
func (s stubUserFetcher) List(limit *uint64, cursor *string) ([]domain.User, string, error) {
	if s.listFn != nil {
		return s.listFn(limit, cursor)
	}
	return nil, "", nil
}
func (s stubUserFetcher) WhoToFollow(limit *uint64, cursor *string) ([]domain.User, string, error) {
	if s.whoToFollowFn != nil {
		return s.whoToFollowFn(limit, cursor)
	}
	return nil, "", nil
}
func (s stubUserFetcher) Update(userId string, newUser domain.User) (domain.User, error) {
	if s.updateFn != nil {
		return s.updateFn(userId, newUser)
	}
	return newUser, nil
}
func (s stubUserFetcher) CreateWithTTL(user domain.User, ttl time.Duration) (domain.User, error) {
	if s.createWithTTLFn != nil {
		return s.createWithTTLFn(user, ttl)
	}
	return user, nil
}

type stubUserTweetsCounter struct {
	tweetsCountFn func(userID string) (uint64, error)
}

func (s stubUserTweetsCounter) TweetsCount(userID string) (uint64, error) {
	if s.tweetsCountFn != nil {
		return s.tweetsCountFn(userID)
	}
	return 0, nil
}

type stubUserFollowsCounter struct {
	getFollowersCountFn  func(userId string) (uint64, error)
	getFollowingsCountFn func(userId string) (uint64, error)
	getFollowersFn       func(userId string, limit *uint64, cursor *string) ([]string, string, error)
	getFollowingsFn      func(userId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubUserFollowsCounter) GetFollowersCount(userId string) (uint64, error) {
	if s.getFollowersCountFn != nil {
		return s.getFollowersCountFn(userId)
	}
	return 0, nil
}
func (s stubUserFollowsCounter) GetFollowingsCount(userId string) (uint64, error) {
	if s.getFollowingsCountFn != nil {
		return s.getFollowingsCountFn(userId)
	}
	return 0, nil
}
func (s stubUserFollowsCounter) GetFollowers(userId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.getFollowersFn != nil {
		return s.getFollowersFn(userId, limit, cursor)
	}
	return nil, "", nil
}
func (s stubUserFollowsCounter) GetFollowings(userId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.getFollowingsFn != nil {
		return s.getFollowingsFn(userId, limit, cursor)
	}
	return nil, "", nil
}

type stubUserStreamer struct {
	genericStreamFn func(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
	nodeInfo        warpnet.NodeInfo
}

func (s stubUserStreamer) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	if s.genericStreamFn != nil {
		return s.genericStreamFn(nodeId, path, data)
	}
	return nil, nil
}
func (s stubUserStreamer) NodeInfo() warpnet.NodeInfo { return s.nodeInfo }

func TestStreamGetUserHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetUserHandler(stubUserTweetsCounter{}, stubUserFollowsCounter{}, stubUserFetcher{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubUserStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetUserHandler(stubUserTweetsCounter{}, stubUserFollowsCounter{}, stubUserFetcher{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubUserStreamer{})
		_, err := h(marshal(t, event.GetUserEvent{}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("own profile - with counts", func(t *testing.T) {
		h := StreamGetUserHandler(
			stubUserTweetsCounter{tweetsCountFn: func(userID string) (uint64, error) { return 10, nil }},
			stubUserFollowsCounter{
				getFollowersCountFn:  func(userId string) (uint64, error) { return 100, nil },
				getFollowingsCountFn: func(userId string) (uint64, error) { return 50, nil },
			},
			stubUserFetcher{getFn: func(userId string) (domain.User, error) {
				return domain.User{Id: owner, Username: "test"}, nil
			}},
			stubAuth{owner: domain.Owner{UserId: owner}},
			stubUserStreamer{},
		)
		resp, err := h(marshal(t, event.GetUserEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		u := resp.(domain.User)
		if u.TweetsCount != 10 || u.FollowersCount != 100 || u.FollowingsCount != 50 {
			t.Fatalf("unexpected counts: tweets=%d followers=%d followings=%d", u.TweetsCount, u.FollowersCount, u.FollowingsCount)
		}
	})

	t.Run("own profile - repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamGetUserHandler(stubUserTweetsCounter{}, stubUserFollowsCounter{}, stubUserFetcher{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, repoErr
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, stubUserStreamer{})
		_, err := h(marshal(t, event.GetUserEvent{UserId: owner}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("other user profile - success", func(t *testing.T) {
		h := StreamGetUserHandler(stubUserTweetsCounter{}, stubUserFollowsCounter{}, stubUserFetcher{getFn: func(userId string) (domain.User, error) {
			return domain.User{Id: userId, NodeId: "node-2", Username: "other"}, nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, stubUserStreamer{})
		resp, err := h(marshal(t, event.GetUserEvent{UserId: "other-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		u := resp.(domain.User)
		if u.Username != "other" {
			t.Fatalf("unexpected user: %v", u)
		}
	})

	t.Run("other user profile - missing node id", func(t *testing.T) {
		h := StreamGetUserHandler(stubUserTweetsCounter{}, stubUserFollowsCounter{}, stubUserFetcher{getFn: func(userId string) (domain.User, error) {
			return domain.User{Id: userId, NodeId: ""}, nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, stubUserStreamer{})
		_, err := h(marshal(t, event.GetUserEvent{UserId: "other-1"}), nil)
		if err == nil {
			t.Fatal("expected error for missing node id")
		}
	})

	t.Run("other user profile - not found", func(t *testing.T) {
		h := StreamGetUserHandler(stubUserTweetsCounter{}, stubUserFollowsCounter{}, stubUserFetcher{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, stubUserStreamer{})
		_, err := h(marshal(t, event.GetUserEvent{UserId: "other-1"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestStreamGetUsersHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetUsersHandler(stubUserFetcher{}, stubUserStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetUsersHandler(stubUserFetcher{}, stubUserStreamer{})
		_, err := h(marshal(t, event.GetAllUsersEvent{}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("users exist locally - returns immediately", func(t *testing.T) {
		h := StreamGetUsersHandler(stubUserFetcher{listFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
			return []domain.User{{Id: "u1"}}, "end", nil
		}}, stubUserStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 1 {
			t.Fatalf("expected 1 user, got %d", len(r.Users))
		}
	})

	t.Run("no users locally - fetches and returns", func(t *testing.T) {
		callCount := 0
		h := StreamGetUsersHandler(stubUserFetcher{listFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
			callCount++
			if callCount == 1 {
				return nil, "", nil
			}
			return []domain.User{{Id: "u1"}}, "end", nil
		}}, stubUserStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 1 {
			t.Fatalf("expected 1 user, got %d", len(r.Users))
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamGetUsersHandler(stubUserFetcher{listFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
			return nil, "", repoErr
		}}, stubUserStreamer{})
		_, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
}

func TestStreamGetWhoToFollowHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{})
		_, err := h(marshal(t, event.GetAllUsersEvent{}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("filters out self and already followed", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-1", Username: "test"}},
			stubUserFetcher{
				whoToFollowFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
					return []domain.User{
						{Id: owner, Network: warpnet.WarpnetName},
						{Id: "already-followed", Network: warpnet.WarpnetName},
						{Id: "new-user", Network: warpnet.WarpnetName},
					}, "end", nil
				},
				getFn: func(userId string) (domain.User, error) {
					return domain.User{Id: owner, Network: warpnet.WarpnetName}, nil
				},
			},
			stubUserFollowsCounter{getFollowingsFn: func(userId string, limit *uint64, cursor *string) ([]string, string, error) {
				return []string{"already-followed"}, "", nil
			}},
		)
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 1 || r.Users[0].Id != "new-user" {
			t.Fatalf("expected only new-user, got: %v", r.Users)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{whoToFollowFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
			return nil, "", repoErr
		}}, stubUserFollowsCounter{})
		_, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{})
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 0 {
			t.Fatalf("expected empty users: %v", r.Users)
		}
	})
}

func TestStreamUpdateProfileHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUpdateProfileHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("success", func(t *testing.T) {
		h := StreamUpdateProfileHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{updateFn: func(userId string, newUser domain.User) (domain.User, error) {
			newUser.Id = userId
			return newUser, nil
		}})
		resp, err := h(marshal(t, event.NewUserEvent{Bio: "new bio"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		u := resp.(domain.User)
		if u.Bio != "new bio" {
			t.Fatalf("unexpected bio: %v", u)
		}
		if u.Id != owner {
			t.Fatalf("expected owner id to be used: %v", u)
		}
	})

	t.Run("update error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamUpdateProfileHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{updateFn: func(userId string, newUser domain.User) (domain.User, error) {
			return domain.User{}, repoErr
		}})
		_, err := h(marshal(t, event.NewUserEvent{Bio: "new bio"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
}
