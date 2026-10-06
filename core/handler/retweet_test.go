//nolint:all
package handler

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

type stubRetweetUserRepo struct {
	getBatchFn func(ids ...string) ([]domain.User, error)
	getFn      func(userId string) (domain.User, error)
	createFn   func(user domain.User) (domain.User, error)
}

func (s stubRetweetUserRepo) Create(user domain.User) (domain.User, error) {
	if s.createFn != nil {
		return s.createFn(user)
	}
	return user, nil
}

func (s stubRetweetUserRepo) GetBatch(ids ...string) ([]domain.User, error) {
	if s.getBatchFn != nil {
		return s.getBatchFn(ids...)
	}
	return nil, nil
}
func (s stubRetweetUserRepo) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{Id: userId, NodeId: "node-2"}, nil
}

type stubReTweetRepo struct {
	getFn           func(userID, tweetID string) (domain.Tweet, error)
	newRetweetFn    func(tweet domain.Tweet) (domain.Tweet, error)
	unRetweetFn     func(retweetedByUserID, tweetId string) error
	retweetsCountFn func(tweetId string) (uint64, error)
	retweetersFn    func(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubReTweetRepo) Get(userID, tweetID string) (domain.Tweet, error) {
	if s.getFn != nil {
		return s.getFn(userID, tweetID)
	}
	return domain.Tweet{Id: tweetID, UserId: userID}, nil
}
func (s stubReTweetRepo) NewRetweet(tweet domain.Tweet, _ bool) (domain.Tweet, error) {
	if s.newRetweetFn != nil {
		return s.newRetweetFn(tweet)
	}
	return tweet, nil
}
func (s stubReTweetRepo) UnRetweet(retweetedByUserID, tweetId string, _ bool) error {
	if s.unRetweetFn != nil {
		return s.unRetweetFn(retweetedByUserID, tweetId)
	}
	return nil
}
func (s stubReTweetRepo) RetweetsCount(tweetId string) (uint64, error) {
	if s.retweetsCountFn != nil {
		return s.retweetsCountFn(tweetId)
	}
	return 0, nil
}
func (s stubReTweetRepo) Retweeters(tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.retweetersFn != nil {
		return s.retweetersFn(tweetId, limit, cursor)
	}
	return nil, "", nil
}

type stubTimelineRepo struct {
	addFn    func(userId string, tweet domain.Tweet) error
	deleteFn func(userID, tweetID string) error
}

func (s stubTimelineRepo) DeleteTweetFromTimeline(userID, tweetID string) error {
	if s.deleteFn != nil {
		return s.deleteFn(userID, tweetID)
	}
	return nil
}

func (s stubTimelineRepo) AddTweetToTimeline(userId string, tweet domain.Tweet) error {
	if s.addFn != nil {
		return s.addFn(userId, tweet)
	}
	return nil
}

func TestStreamNewReTweetHandler(t *testing.T) {
	owner := "owner-1"
	tweetOwner := "tweet-owner"
	retweeter := owner
	tweetId := "tweet-1"

	_, actorConn := authorStream(t)
	actorNode := actorConn.Conn().RemotePeer().String()
	actorUsers := func(actorId string, otherFn func(userId string) (domain.User, error)) stubRetweetUserRepo {
		return stubRetweetUserRepo{getFn: func(userId string) (domain.User, error) {
			if userId == actorId {
				return domain.User{Id: userId, Username: "retweeter-user", NodeId: actorNode}, nil
			}
			if otherFn != nil {
				return otherFn(userId)
			}
			return domain.User{Id: userId, NodeId: "node-2"}, nil
		}}
	}

	makeTweet := func() event.NewRetweetEvent {
		rt := retweeter
		return domain.Tweet{
			Id:          tweetId,
			UserId:      tweetOwner,
			Text:        "original",
			RetweetedBy: &rt,
			CreatedAt:   time.Now(),
		}
	}

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing retweeted by", func(t *testing.T) {
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{})
		tw := domain.Tweet{Id: tweetId, UserId: tweetOwner}
		_, err := h(marshal(t, event.NewRetweetEvent(tw)), nil)
		if err == nil || err.Error() != "retweeted by unknown" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("missing tweet id", func(t *testing.T) {
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{})
		rt := retweeter
		tw := domain.Tweet{UserId: tweetOwner, RetweetedBy: &rt}
		_, err := h(marshal(t, event.NewRetweetEvent(tw)), nil)
		if err == nil || err.Error() != "empty retweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamNewReTweetHandler(actorUsers(retweeter, nil), stubReTweetRepo{
			newRetweetFn: func(tweet domain.Tweet) (domain.Tweet, error) { return domain.Tweet{}, repoErr },
		}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h(marshal(t, makeTweet()), actorConn)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("own tweet retweet", func(t *testing.T) {
		h := StreamNewReTweetHandler(actorUsers(tweetOwner, nil), stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: tweetOwner},
		})
		rt := tweetOwner
		tw := domain.Tweet{Id: tweetId, UserId: tweetOwner, RetweetedBy: &rt, CreatedAt: time.Now()}
		resp, err := h(marshal(t, event.NewRetweetEvent(tw)), actorConn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Id == "" {
			t.Fatalf("expected tweet in response")
		}
	})

	t.Run("someone retweeted my tweet - adds notification", func(t *testing.T) {
		notified := false
		h := StreamNewReTweetHandler(actorUsers(retweeter, nil), stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{addFn: func(not domain.Notification) error {
			notified = true
			if not.Type != domain.NotificationRetweetType {
				t.Fatalf("expected retweet type, got: %v", not.Type)
			}
			if not.RecepientId != tweetOwner {
				t.Fatalf("expected notification for tweet owner, got: %v", not.RecepientId)
			}
			return nil
		}}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: tweetOwner},
		})
		resp, err := h(marshal(t, makeTweet()), actorConn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Id == "" {
			t.Fatalf("expected tweet in response")
		}
		if !notified {
			t.Fatal("expected notification to be added")
		}
	})

	t.Run("owner retweeter adds to timeline", func(t *testing.T) {
		timelineAdded := false
		h := StreamNewReTweetHandler(actorUsers(retweeter, nil), stubReTweetRepo{}, stubTimelineRepo{
			addFn: func(userId string, tweet domain.Tweet) error {
				timelineAdded = true
				return nil
			},
		}, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, makeTweet()), actorConn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !timelineAdded {
			t.Fatal("expected timeline to be updated")
		}
		_ = resp
	})

	t.Run("tweet owner user not found", func(t *testing.T) {
		h := StreamNewReTweetHandler(actorUsers(retweeter, func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}), stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, makeTweet()), actorConn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Id == "" {
			t.Fatal("expected tweet response")
		}
	})

	t.Run("stream node offline", func(t *testing.T) {
		h := StreamNewReTweetHandler(actorUsers(retweeter, nil), stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		})
		_, err := h(marshal(t, makeTweet()), actorConn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("stream error", func(t *testing.T) {
		streamErr := errors.New("broken")
		h := StreamNewReTweetHandler(actorUsers(retweeter, nil), stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, streamErr
			},
		})
		_, err := h(marshal(t, makeTweet()), actorConn)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("remote response with error payload", func(t *testing.T) {
		respErr, _ := json.Marshal(event.ResponseError{Code: 500, Message: "oops"})
		h := StreamNewReTweetHandler(actorUsers(retweeter, nil), stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return respErr, nil
			},
		})
		_, err := h(marshal(t, makeTweet()), actorConn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})
}

func TestStreamUnretweetHandler(t *testing.T) {
	owner := "owner-1"
	tweetOwner := "tweet-owner"
	tweetId := "tweet-1"

	_, actorConn := authorStream(t)
	actorNode := actorConn.Conn().RemotePeer().String()
	actorUsers := func(otherFn func(userId string) (domain.User, error)) stubRetweetUserRepo {
		return stubRetweetUserRepo{getFn: func(userId string) (domain.User, error) {
			if userId == owner {
				return domain.User{Id: userId, NodeId: actorNode}, nil
			}
			if otherFn != nil {
				return otherFn(userId)
			}
			return domain.User{Id: userId, NodeId: "node-2"}, nil
		}}
	}

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{}, stubRetweetUserRepo{}, stubTimelineRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty retweeter id", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{}, stubRetweetUserRepo{}, stubTimelineRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId}), nil)
		if err == nil || err.Error() != "empty retweeter id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{}, stubRetweetUserRepo{}, stubTimelineRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnretweetEvent{RetweeterId: owner}), nil)
		if err == nil || err.Error() != "empty tweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("get tweet error", func(t *testing.T) {
		repoErr := errors.New("not found")
		h := StreamUnretweetHandler(stubReTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{}, repoErr
		}}, actorUsers(nil), stubTimelineRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), actorConn)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("unretweet error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamUnretweetHandler(stubReTweetRepo{
			unRetweetFn: func(retweetedByUserID, tweetId string) error { return repoErr },
		}, actorUsers(nil), stubTimelineRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), actorConn)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected unretweet error: %v", err)
		}
	})

	t.Run("own tweet unretweet", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetID, UserId: owner}, nil
		}}, actorUsers(nil), stubTimelineRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), actorConn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted: %v", resp)
		}
	})

	t.Run("tweet owner not found", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetID, UserId: tweetOwner}, nil
		}}, actorUsers(func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}), stubTimelineRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), actorConn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted: %v", resp)
		}
	})

	t.Run("stream node offline", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetID, UserId: tweetOwner}, nil
		}}, actorUsers(nil), stubTimelineRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		})
		resp, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), actorConn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted: %v", resp)
		}
	})

	t.Run("stream error", func(t *testing.T) {
		streamErr := errors.New("broken")
		h := StreamUnretweetHandler(stubReTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetID, UserId: tweetOwner}, nil
		}}, actorUsers(nil), stubTimelineRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, streamErr
			},
		})
		_, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), actorConn)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	for _, tc := range []struct {
		name          string
		eventTweetId  string
		author        string
		wantTimeline  string
		wantForwarded bool
	}{
		{"foreign tweet", tweetId, tweetOwner, tweetId, true},
		{"own tweet by its id", tweetId, owner, domain.RetweetPrefix + tweetId, false},
		{"own tweet by its RT: id", domain.RetweetPrefix + tweetId, owner, domain.RetweetPrefix + tweetId, false},
	} {
		t.Run("owner unretweet of "+tc.name, func(t *testing.T) {
			var gotGet, gotUnretweet, gotTimeline string
			forwarded := false
			h := StreamUnretweetHandler(stubReTweetRepo{
				getFn: func(userID, tweetID string) (domain.Tweet, error) {
					gotGet = tweetID
					return domain.Tweet{Id: tweetID, UserId: tc.author}, nil
				},
				unRetweetFn: func(_, tweetID string) error {
					gotUnretweet = tweetID
					return nil
				},
			}, actorUsers(nil), stubTimelineRepo{deleteFn: func(userID, tweetID string) error {
				if userID != owner {
					t.Fatalf("timeline of %q touched, want %q", userID, owner)
				}
				gotTimeline = tweetID
				return nil
			}}, stubStreamer{
				nodeInfo: warpnet.NodeInfo{OwnerId: owner},
				genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
					forwarded = true
					return nil, nil
				},
			})

			resp, err := h(marshal(t, event.UnretweetEvent{TweetId: tc.eventTweetId, RetweeterId: owner}), actorConn)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if resp != event.Accepted {
				t.Fatalf("expected accepted: %v", resp)
			}
			if gotGet != tweetId || gotUnretweet != tweetId {
				t.Fatalf("expected source id %q, got get=%q unretweet=%q", tweetId, gotGet, gotUnretweet)
			}
			if gotTimeline != tc.wantTimeline {
				t.Fatalf("expected timeline delete of %q, got %q", tc.wantTimeline, gotTimeline)
			}
			if forwarded != tc.wantForwarded {
				t.Fatalf("forwarded=%v, want %v", forwarded, tc.wantForwarded)
			}
		})
	}

	t.Run("forwarded unretweet leaves the author's timeline alone", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetID, UserId: tweetOwner}, nil
		}}, actorUsers(nil), stubTimelineRepo{deleteFn: func(userID, tweetID string) error {
			t.Fatalf("unexpected timeline delete %s/%s", userID, tweetID)
			return nil
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: tweetOwner}, genericStreamFn: failOnStream(t)})
		resp, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), actorConn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted: %v", resp)
		}
	})

	t.Run("timeline failure does not fail the unretweet", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetID, UserId: owner}, nil
		}}, actorUsers(nil), stubTimelineRepo{deleteFn: func(userID, tweetID string) error {
			return errors.New("timeline down")
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), actorConn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted: %v", resp)
		}
	})

	t.Run("failed unretweet leaves the timeline alone", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamUnretweetHandler(stubReTweetRepo{
			unRetweetFn: func(retweetedByUserID, tweetId string) error { return repoErr },
		}, actorUsers(nil), stubTimelineRepo{deleteFn: func(userID, tweetID string) error {
			t.Fatalf("unexpected timeline delete %s/%s", userID, tweetID)
			return nil
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		_, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), actorConn)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected unretweet error: %v", err)
		}
	})
}

func newRetweetRepos(t *testing.T) (*database.TweetRepo, *database.TimelineRepo) {
	t.Helper()
	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	if err != nil {
		t.Fatalf("local-store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.NewAuthRepo(db, "test").Authenticate("test", "test"); err != nil {
		t.Fatalf("auth: %v", err)
	}
	return database.NewTweetRepo(db, nil), database.NewTimelineRepo(db)
}

func timelineIds(t *testing.T, repo *database.TimelineRepo, userId string) []string {
	t.Helper()
	tweets, _, err := repo.GetTimeline(userId, nil, nil)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	ids := make([]string, 0, len(tweets))
	for _, tw := range tweets {
		ids = append(ids, tw.Id)
	}
	return ids
}

func TestUnretweetRemovesRetweetFromTimeline(t *testing.T) {
	tweetRepo, timelineRepo := newRetweetRepos(t)

	retweeter := "bob"
	_, conn := authorStream(t)
	actorNode := conn.Conn().RemotePeer().String()
	users := stubRetweetUserRepo{getFn: func(userId string) (domain.User, error) {
		if userId == retweeter {
			return domain.User{Id: userId, NodeId: actorNode}, nil
		}
		return domain.User{}, database.ErrUserNotFound
	}}
	streamer := stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: retweeter}}

	rt := retweeter
	source := domain.Tweet{Id: "tweet-1", UserId: "alice", Text: "original", RetweetedBy: &rt, CreatedAt: time.Now()}
	retweet := StreamNewReTweetHandler(users, tweetRepo, timelineRepo, stubModerationNotifier{}, streamer)
	unretweet := StreamUnretweetHandler(tweetRepo, users, timelineRepo, streamer)

	for round := 1; round <= 2; round++ {
		if _, err := retweet(marshal(t, event.NewRetweetEvent(source)), conn); err != nil {
			t.Fatalf("round %d retweet: %v", round, err)
		}
		if ids := timelineIds(t, timelineRepo, retweeter); len(ids) != 1 || ids[0] != source.Id {
			t.Fatalf("round %d: expected the retweet in timeline, got %v", round, ids)
		}
		if count, err := tweetRepo.RetweetsCount(source.Id); err != nil || count != 1 {
			t.Fatalf("round %d: expected 1 retweet, got %d (%v)", round, count, err)
		}

		if _, err := unretweet(marshal(t, event.UnretweetEvent{TweetId: source.Id, RetweeterId: retweeter}), conn); err != nil {
			t.Fatalf("round %d unretweet: %v", round, err)
		}
		if ids := timelineIds(t, timelineRepo, retweeter); len(ids) != 0 {
			t.Fatalf("round %d: expected empty timeline after unretweet, got %v", round, ids)
		}
		if _, err := tweetRepo.Get(retweeter, source.Id); !errors.Is(err, database.ErrTweetNotFound) {
			t.Fatalf("round %d: expected the retweet gone from the profile, got %v", round, err)
		}
		if count, err := tweetRepo.RetweetsCount(source.Id); err != nil || count != 0 {
			t.Fatalf("round %d: expected 0 retweets, got %d (%v)", round, count, err)
		}
	}
}

func TestUnretweetOfOwnTweetKeepsTheOriginal(t *testing.T) {
	tweetRepo, timelineRepo := newRetweetRepos(t)

	owner := "alice"
	_, conn := authorStream(t)
	actorNode := conn.Conn().RemotePeer().String()
	users := stubRetweetUserRepo{getFn: func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: actorNode}, nil
	}}
	streamer := stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}, genericStreamFn: failOnStream(t)}

	original, err := tweetRepo.Create(owner, domain.Tweet{UserId: owner, Text: "original", CreatedAt: time.Now()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := timelineRepo.AddTweetToTimeline(owner, original); err != nil {
		t.Fatalf("timeline: %v", err)
	}
	retweetId := domain.RetweetPrefix + original.Id

	for _, unretweetId := range []string{original.Id, retweetId} {
		rt := owner
		source := original
		source.RetweetedBy = &rt
		retweet := StreamNewReTweetHandler(users, tweetRepo, timelineRepo, stubModerationNotifier{}, streamer)
		if _, err := retweet(marshal(t, event.NewRetweetEvent(source)), conn); err != nil {
			t.Fatalf("retweet: %v", err)
		}
		if ids := timelineIds(t, timelineRepo, owner); len(ids) != 2 {
			t.Fatalf("expected the original and the retweet in timeline, got %v", ids)
		}

		unretweet := StreamUnretweetHandler(tweetRepo, users, timelineRepo, streamer)
		if _, err := unretweet(marshal(t, event.UnretweetEvent{TweetId: unretweetId, RetweeterId: owner}), conn); err != nil {
			t.Fatalf("unretweet %q: %v", unretweetId, err)
		}

		if _, err := tweetRepo.Get(owner, original.Id); err != nil {
			t.Fatalf("unretweet %q deleted the original: %v", unretweetId, err)
		}
		if _, err := tweetRepo.Get(owner, retweetId); !errors.Is(err, database.ErrTweetNotFound) {
			t.Fatalf("unretweet %q left the retweet: %v", unretweetId, err)
		}
		if ids := timelineIds(t, timelineRepo, owner); len(ids) != 1 || ids[0] != original.Id {
			t.Fatalf("unretweet %q: expected only the original in timeline, got %v", unretweetId, ids)
		}
		if count, err := tweetRepo.RetweetsCount(original.Id); err != nil || count != 0 {
			t.Fatalf("unretweet %q: expected 0 retweets, got %d (%v)", unretweetId, count, err)
		}
	}
}

func TestUnretweetKeepsFollowedAuthorsTweetInTimeline(t *testing.T) {
	tweetRepo, timelineRepo := newRetweetRepos(t)

	retweeter := "bob"
	_, conn := authorStream(t)
	actorNode := conn.Conn().RemotePeer().String()
	users := stubRetweetUserRepo{getFn: func(userId string) (domain.User, error) {
		if userId == retweeter {
			return domain.User{Id: userId, NodeId: actorNode}, nil
		}
		return domain.User{}, database.ErrUserNotFound
	}}
	streamer := stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: retweeter}}

	followed := domain.Tweet{Id: "tweet-1", UserId: "alice", Text: "original", CreatedAt: time.Now().Add(-time.Hour)}
	if err := timelineRepo.AddTweetToTimeline(retweeter, followed); err != nil {
		t.Fatalf("timeline: %v", err)
	}

	rt := retweeter
	source := followed
	source.RetweetedBy = &rt
	source.CreatedAt = time.Now()
	retweet := StreamNewReTweetHandler(users, tweetRepo, timelineRepo, stubModerationNotifier{}, streamer)
	if _, err := retweet(marshal(t, event.NewRetweetEvent(source)), conn); err != nil {
		t.Fatalf("retweet: %v", err)
	}

	unretweet := StreamUnretweetHandler(tweetRepo, users, timelineRepo, streamer)
	if _, err := unretweet(marshal(t, event.UnretweetEvent{TweetId: source.Id, RetweeterId: retweeter}), conn); err != nil {
		t.Fatalf("unretweet: %v", err)
	}

	tweets, _, err := timelineRepo.GetTimeline(retweeter, nil, nil)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if len(tweets) != 1 || tweets[0].Id != followed.Id || tweets[0].RetweetedBy != nil {
		t.Fatalf("expected only alice's own tweet in timeline, got %+v", tweets)
	}
}

func TestEditOfSelfRetweetedTweetKeepsTheTweet(t *testing.T) {
	tweetRepo, timelineRepo := newRetweetRepos(t)

	owner := "alice"
	original, err := tweetRepo.Create(owner, domain.Tweet{UserId: owner, Text: "original", CreatedAt: time.Now()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rt := owner
	source := original
	source.RetweetedBy = &rt
	if _, err := tweetRepo.NewRetweet(source, true); err != nil {
		t.Fatalf("retweet: %v", err)
	}

	edit := StreamEditTweetHandler(tweetRepo, timelineRepo)
	if _, err := edit(marshal(t, event.EditTweetEvent{UserId: owner, TweetId: original.Id, Text: "edited"}), nil); err != nil {
		t.Fatalf("edit: %v", err)
	}

	edited, err := tweetRepo.Get(owner, original.Id)
	if err != nil {
		t.Fatalf("edit deleted the tweet: %v", err)
	}
	if edited.Text != "edited" {
		t.Fatalf("expected edited text, got %q", edited.Text)
	}
	if _, err := tweetRepo.Get(owner, domain.RetweetPrefix+original.Id); !errors.Is(err, database.ErrTweetNotFound) {
		t.Fatalf("expected the self-retweet cancelled, got %v", err)
	}
	if count, err := tweetRepo.RetweetsCount(original.Id); err != nil || count != 0 {
		t.Fatalf("expected 0 retweets, got %d (%v)", count, err)
	}
}

func TestStreamNewReTweetHandler_SponsoredSourceKeepsOnlyTheTeaser(t *testing.T) {
	retweeter := "owner-1"
	_, actorConn := authorStream(t)
	users := stubRetweetUserRepo{getFn: func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: actorConn.Conn().RemotePeer().String()}, nil
	}}
	var stored domain.Tweet
	repo := stubReTweetRepo{
		getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetID, UserId: userID, Price: &domain.Price{Amount: "1.5", Units: big.NewInt(1500000)}}, nil
		},
		newRetweetFn: func(tweet domain.Tweet) (domain.Tweet, error) {
			stored = tweet
			return tweet, nil
		},
	}
	h := StreamNewReTweetHandler(users, repo, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: retweeter}})

	video := "video-1"
	_, err := h(marshal(t, event.NewRetweetEvent{
		Id: "tweet-1", UserId: "author-1", Text: "unlocked text", ImageKeys: []string{"img-1"}, VideoKey: &video,
		RetweetedBy: &retweeter, CreatedAt: time.Now(),
	}), actorConn)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if stored.Text != "" || len(stored.ImageKeys) != 0 || stored.VideoKey != nil || !stored.IsSponsored() {
		t.Fatalf("a retweet of a sponsored tweet must carry only the teaser, got %+v", stored)
	}
}
