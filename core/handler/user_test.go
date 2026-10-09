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
	"github.com/Warp-net/warpnet/json"
)

type stubUserFetcher struct {
	createFn        func(user domain.User) (domain.User, error)
	getFn           func(userId string) (domain.User, error)
	listFn          func(limit *uint64, cursor *string) ([]domain.User, string, error)
	searchFn        func(query string, limit *uint64, cursor *string) ([]domain.User, string, error)
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
func (s stubUserFetcher) Search(query string, limit *uint64, cursor *string) ([]domain.User, string, error) {
	if s.searchFn != nil {
		return s.searchFn(query, limit, cursor)
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

	t.Run("other user profile - unknown user without a node id asks nobody", func(t *testing.T) {
		var asked bool
		h := StreamGetUserHandler(stubUserTweetsCounter{}, stubUserFollowsCounter{}, stubUserFetcher{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, stubUserStreamer{
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				asked = true
				return nil, nil
			},
		})
		_, err := h(marshal(t, event.GetUserEvent{UserId: "other-1"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
		if asked {
			t.Fatal("a request carrying no node id leaves nobody to ask")
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

	t.Run("filters out Warpnet users federated back by a gateway", func(t *testing.T) {
		h := StreamGetUsersHandler(stubUserFetcher{listFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
			return []domain.User{{Id: "01KTRA1QJ8M2W7Y4ZB6C9D3E5F@warpnet-gw.example"}, {Id: "u1"}}, "end", nil
		}}, stubUserStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 1 || r.Users[0].Id != "u1" {
			t.Fatalf("expected only u1, got: %v", r.Users)
		}
	})

	t.Run("no users locally - fetches from remote and returns", func(t *testing.T) {
		requestUser := "requester-1"
		fetchedUsers := []domain.User{{Id: "u1"}}
		listCallCount := 0
		genericStreamCalled := 0
		persistedUsers := 0

		h := StreamGetUsersHandler(
			stubUserFetcher{
				listFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
					listCallCount++
					if persistedUsers == 0 {
						return nil, "", nil
					}
					return fetchedUsers, "end", nil
				},
				getFn: func(userId string) (domain.User, error) {
					return domain.User{Id: userId, NodeId: "node-2"}, nil
				},
				createFn: func(user domain.User) (domain.User, error) {
					persistedUsers++
					return user, nil
				},
			},
			stubUserStreamer{
				nodeInfo: warpnet.NodeInfo{OwnerId: owner},
				genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
					genericStreamCalled++
					resp := event.UsersResponse{Users: fetchedUsers, Cursor: "end"}
					b, _ := json.Marshal(resp)
					return b, nil
				},
			},
		)

		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: requestUser}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if genericStreamCalled != 1 {
			t.Fatalf("expected GenericStream to be called once, got %d", genericStreamCalled)
		}
		if persistedUsers != len(fetchedUsers) {
			t.Fatalf("expected %d persisted users, got %d", len(fetchedUsers), persistedUsers)
		}
		if listCallCount < 2 {
			t.Fatalf("expected local list to be retried after refresh, got %d calls", listCallCount)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 1 {
			t.Fatalf("expected 1 user, got %d", len(r.Users))
		}
	})

	t.Run("a remote list updates only the remote node's owner", func(t *testing.T) {
		var updated []string
		h := StreamGetUsersHandler(
			stubUserFetcher{
				getFn: func(userId string) (domain.User, error) {
					return domain.User{Id: userId, NodeId: "node-2"}, nil
				},
				updateFn: func(userId string, newUser domain.User) (domain.User, error) {
					updated = append(updated, userId)
					return newUser, nil
				},
			},
			stubUserStreamer{
				nodeInfo: warpnet.NodeInfo{OwnerId: owner},
				genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
					return json.Marshal(event.UsersResponse{Users: []domain.User{
						{Id: "remote-owner", NodeId: "node-2", Username: "fresh"},
						{Id: "someone-else", NodeId: "node-3", Username: "stale"},
					}})
				},
			},
		)
		if _, err := h(marshal(t, event.GetAllUsersEvent{UserId: "remote-owner"}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(updated) != 1 || updated[0] != "remote-owner" {
			t.Fatalf("a second-hand copy overwrote a local record: %v", updated)
		}
	})

	t.Run("a remote list brings a known user back online and keeps its profile", func(t *testing.T) {
		updated := map[string]domain.User{}
		h := StreamGetUsersHandler(
			stubUserFetcher{
				getFn: func(userId string) (domain.User, error) {
					if userId == "someone-else" {
						return domain.User{Id: userId, NodeId: "node-3", Username: "fresh", RoundTripTime: 42, IsOffline: true}, nil
					}
					return domain.User{Id: userId, NodeId: "node-2"}, nil
				},
				updateFn: func(userId string, newUser domain.User) (domain.User, error) {
					updated[userId] = newUser
					return newUser, nil
				},
			},
			stubUserStreamer{
				nodeInfo: warpnet.NodeInfo{OwnerId: owner},
				genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
					return json.Marshal(event.UsersResponse{Users: []domain.User{
						{Id: "remote-owner", NodeId: "node-2", Username: "fresh"},
						{Id: "someone-else", NodeId: "node-3", Username: "stale"},
					}})
				},
			},
		)
		if _, err := h(marshal(t, event.GetAllUsersEvent{UserId: "remote-owner"}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		revived, ok := updated["someone-else"]
		if !ok || revived.IsOffline {
			t.Fatalf("a user another node sees online stayed offline here: %v", updated)
		}
		if revived.Username != "" || revived.RoundTripTime != 42 {
			t.Fatalf("bringing a user back online changed its profile: %+v", revived)
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

func TestStreamGetUserStatusHandler(t *testing.T) {
	owner := "owner-1"
	ownNode := warpnet.WarpPeerID("own-node")
	auth := stubAuth{owner: domain.Owner{UserId: owner}}
	infoOf := func(ownerId string) []byte {
		return []byte(`{"type":"member","owner_id":"` + ownerId +
			`","node_id":"12D3KooWH5YPwJiptN44YWhwePX1jHrnCLfXzEqQTMnto6fgKcHz"}`)
	}
	otherUser := func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: "node-2"}, nil
	}
	streamerAnswering := func(resp []byte, err error) stubUserStreamer {
		return stubUserStreamer{
			nodeInfo: warpnet.NodeInfo{ID: ownNode, OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				if nodeId != "node-2" || path != event.PUBLIC_GET_INFO {
					t.Fatalf("unexpected request: %s %s", nodeId, path)
				}
				return resp, err
			},
		}
	}

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetUserStatusHandler(stubUserFetcher{}, auth, stubUserStreamer{})
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetUserStatusHandler(stubUserFetcher{}, auth, stubUserStreamer{})
		_, err := h(marshal(t, event.GetUserEvent{}), nil)
		if !errors.Is(err, errEmptyUserId) {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("owner is online without asking anyone", func(t *testing.T) {
		h := StreamGetUserStatusHandler(stubUserFetcher{}, auth, stubUserStreamer{
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				t.Fatal("the owner's own node must not be asked")
				return nil, nil
			},
		})
		resp, err := h(marshal(t, event.GetUserEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if st := resp.(event.UserStatusResponse); !st.IsOnline || st.LastSeen == nil {
			t.Fatalf("unexpected status: %+v", st)
		}
	})

	t.Run("unknown user", func(t *testing.T) {
		h := StreamGetUserStatusHandler(stubUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, auth, stubUserStreamer{})
		_, err := h(marshal(t, event.GetUserEvent{UserId: "other-1"}), nil)
		if !errors.Is(err, database.ErrUserNotFound) {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("user without a node id", func(t *testing.T) {
		h := StreamGetUserStatusHandler(stubUserFetcher{getFn: func(userId string) (domain.User, error) {
			return domain.User{Id: userId}, nil
		}}, auth, stubUserStreamer{})
		if _, err := h(marshal(t, event.GetUserEvent{UserId: "other-1"}), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("node answers as the user's node", func(t *testing.T) {
		var stored domain.User
		h := StreamGetUserStatusHandler(stubUserFetcher{
			getFn: func(userId string) (domain.User, error) {
				return domain.User{Id: userId, NodeId: "node-2", IsOffline: true}, nil
			},
			updateFn: func(userId string, newUser domain.User) (domain.User, error) {
				stored = newUser
				return newUser, nil
			},
		}, auth, streamerAnswering(infoOf("other-1"), nil))
		resp, err := h(marshal(t, event.GetUserEvent{UserId: "other-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		st := resp.(event.UserStatusResponse)
		if !st.IsOnline || st.LastSeen == nil || st.UserId != "other-1" {
			t.Fatalf("unexpected status: %+v", st)
		}
		if stored.IsOffline || stored.LastSeen == nil {
			t.Fatalf("online status not stored: %+v", stored)
		}
	})

	t.Run("offline node", func(t *testing.T) {
		seen := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
		var stored *domain.User
		h := StreamGetUserStatusHandler(stubUserFetcher{
			getFn: func(userId string) (domain.User, error) {
				return domain.User{Id: userId, NodeId: "node-2", LastSeen: &seen}, nil
			},
			updateFn: func(userId string, newUser domain.User) (domain.User, error) {
				stored = &newUser
				return newUser, nil
			},
		}, auth, streamerAnswering(nil, warpnet.ErrNodeIsOffline))
		resp, err := h(marshal(t, event.GetUserEvent{UserId: "other-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		st := resp.(event.UserStatusResponse)
		if st.IsOnline || st.LastSeen == nil || !st.LastSeen.Equal(seen) {
			t.Fatalf("unexpected status: %+v", st)
		}
		if stored == nil || !stored.IsOffline {
			t.Fatalf("offline status not stored: %+v", stored)
		}
	})

	t.Run("already offline user is not written again", func(t *testing.T) {
		h := StreamGetUserStatusHandler(stubUserFetcher{
			getFn: func(userId string) (domain.User, error) {
				return domain.User{Id: userId, NodeId: "node-2", IsOffline: true}, nil
			},
			updateFn: func(userId string, newUser domain.User) (domain.User, error) {
				t.Fatal("unexpected update")
				return newUser, nil
			},
		}, auth, streamerAnswering(nil, warpnet.ErrNodeIsOffline))
		resp, err := h(marshal(t, event.GetUserEvent{UserId: "other-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.UserStatusResponse).IsOnline {
			t.Fatal("expected offline")
		}
	})

	t.Run("node owned by another user", func(t *testing.T) {
		h := StreamGetUserStatusHandler(stubUserFetcher{getFn: otherUser}, auth, streamerAnswering(infoOf("someone-else"), nil))
		resp, err := h(marshal(t, event.GetUserEvent{UserId: "other-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.UserStatusResponse).IsOnline {
			t.Fatal("a node serving another account must not light the user up")
		}
	})

	t.Run("record pointing at this node", func(t *testing.T) {
		h := StreamGetUserStatusHandler(stubUserFetcher{getFn: func(userId string) (domain.User, error) {
			return domain.User{Id: userId, NodeId: ownNode.String()}, nil
		}}, auth, stubUserStreamer{
			nodeInfo: warpnet.NodeInfo{ID: ownNode, OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				t.Fatal("this node must not stream to itself")
				return nil, nil
			},
		})
		resp, err := h(marshal(t, event.GetUserEvent{UserId: "other-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.UserStatusResponse).IsOnline {
			t.Fatal("expected offline")
		}
	})

	t.Run("unclear answers are errors, not a status", func(t *testing.T) {
		cases := map[string]stubUserStreamer{
			"rejected":     streamerAnswering(marshal(t, event.ResponseError{Code: 429, Message: "too many requests"}), nil),
			"legacy error": streamerAnswering([]byte(`["denied"]`), nil),
			"empty":        streamerAnswering(nil, nil),
			"stream error": streamerAnswering(nil, stream.ErrResponseRead),
		}
		for name, streamer := range cases {
			t.Run(name, func(t *testing.T) {
				h := StreamGetUserStatusHandler(stubUserFetcher{
					getFn: otherUser,
					updateFn: func(userId string, newUser domain.User) (domain.User, error) {
						t.Fatal("unexpected update")
						return newUser, nil
					},
				}, auth, streamer)
				if _, err := h(marshal(t, event.GetUserEvent{UserId: "other-1"}), nil); err == nil {
					t.Fatal("expected error")
				}
			})
		}
	})
}
