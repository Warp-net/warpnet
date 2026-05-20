//nolint:all
package database

import (
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type NotificationsRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *NotificationsRepo
}

func (s *NotificationsRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	authRepo := NewAuthRepo(s.db, "test")
	err = authRepo.Authenticate("test", "test")
	s.Require().NoError(err)

	s.repo = NewNotificationsRepo(s.db)
}

func (s *NotificationsRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *NotificationsRepoTestSuite) TestAddAndListNotifications() {
	userId := uuid.New().String()

	not1 := domain.Notification{
		Type:      domain.NotificationLikeType,
		Text:      "someone liked your tweet",
		UserId:    userId,
		IsRead:    false,
		CreatedAt: time.Now().Add(-2 * time.Second),
	}
	not2 := domain.Notification{
		Type:      domain.NotificationReplyType,
		Text:      "someone replied to your tweet",
		UserId:    userId,
		IsRead:    false,
		CreatedAt: time.Now().Add(-1 * time.Second),
	}
	not3 := domain.Notification{
		Type:      domain.NotificationFollowType,
		Text:      "someone followed you",
		UserId:    userId,
		IsRead:    true,
		CreatedAt: time.Now(),
	}

	err := s.repo.Add(not1)
	s.Require().NoError(err)
	err = s.repo.Add(not2)
	s.Require().NoError(err)
	err = s.repo.Add(not3)
	s.Require().NoError(err)

	limit := uint64(10)
	nots, cursor, err := s.repo.List(userId, &limit, nil)
	s.Require().NoError(err)
	s.Len(nots, 3)
	s.Equal("end", cursor)
}

func (s *NotificationsRepoTestSuite) TestUnreadCountWalksAllPages() {
	userId := uuid.New().String()

	// 7 unread + 3 read, more than the internal page size of 200 only if
	// expanded later; for now just verify the count is global across the
	// scan, not bound to the first item batch.
	for i := 0; i < 7; i++ {
		s.Require().NoError(s.repo.Add(domain.Notification{
			Type:   domain.NotificationLikeType,
			Text:   "unread",
			UserId: userId,
			IsRead: false,
		}))
	}
	for i := 0; i < 3; i++ {
		s.Require().NoError(s.repo.Add(domain.Notification{
			Type:   domain.NotificationReplyType,
			Text:   "read",
			UserId: userId,
			IsRead: true,
		}))
	}

	count, err := s.repo.UnreadCount(userId)
	s.Require().NoError(err)
	s.Equal(uint64(7), count)
}

func (s *NotificationsRepoTestSuite) TestUnreadCount_MissingUserId() {
	_, err := s.repo.UnreadCount("")
	s.Require().Error(err)
}

func (s *NotificationsRepoTestSuite) TestUnreadCount_EmptyUser() {
	count, err := s.repo.UnreadCount(uuid.New().String())
	s.Require().NoError(err)
	s.Equal(uint64(0), count)
}

func (s *NotificationsRepoTestSuite) TestAddNotification_MissingUserId() {
	not := domain.Notification{
		Type: domain.NotificationLikeType,
		Text: "test",
	}
	err := s.repo.Add(not)
	s.Error(err)
	s.Contains(err.Error(), "missing user id")
}

func (s *NotificationsRepoTestSuite) TestListNotifications_MissingUserId() {
	_, _, err := s.repo.List("", nil, nil)
	s.Error(err)
	s.Contains(err.Error(), "missing user id")
}

func (s *NotificationsRepoTestSuite) TestListNotifications_Empty() {
	userId := uuid.New().String()
	limit := uint64(10)
	nots, cursor, err := s.repo.List(userId, &limit, nil)
	s.Require().NoError(err)
	s.Empty(nots)
	s.Equal("end", cursor)
}

func (s *NotificationsRepoTestSuite) TestAddNotification_AutoGeneratesIdAndTime() {
	userId := uuid.New().String()
	not := domain.Notification{
		Type:   domain.NotificationRetweetType,
		Text:   "retweeted",
		UserId: userId,
	}

	err := s.repo.Add(not)
	s.Require().NoError(err)

	limit := uint64(10)
	nots, _, err := s.repo.List(userId, &limit, nil)
	s.Require().NoError(err)
	s.Len(nots, 1)
	s.NotEmpty(nots[0].Id)
	s.False(nots[0].CreatedAt.IsZero())
}

func (s *NotificationsRepoTestSuite) TestAddMultipleNotificationTypes() {
	userId := uuid.New().String()

	types := []domain.NotificationType{
		domain.NotificationLikeType,
		domain.NotificationReplyType,
		domain.NotificationRetweetType,
		domain.NotificationFollowType,
		domain.NotificationMentionType,
		domain.NotificationModerationType,
	}

	for i, notType := range types {
		not := domain.Notification{
			Type:      notType,
			Text:      "notification " + notType.String(),
			UserId:    userId,
			CreatedAt: time.Now().Add(-time.Duration(len(types)-i) * time.Second),
		}
		err := s.repo.Add(not)
		s.Require().NoError(err)
	}

	limit := uint64(20)
	nots, _, err := s.repo.List(userId, &limit, nil)
	s.Require().NoError(err)
	s.Len(nots, len(types))
}

func (s *NotificationsRepoTestSuite) TestListNotifications_Pagination() {
	userId := uuid.New().String()

	for i := 0; i < 5; i++ {
		not := domain.Notification{
			Type:      domain.NotificationLikeType,
			Text:      "notification",
			UserId:    userId,
			CreatedAt: time.Now().Add(-time.Duration(5-i) * time.Second),
		}
		err := s.repo.Add(not)
		s.Require().NoError(err)
	}

	limit := uint64(3)
	nots, cursor, err := s.repo.List(userId, &limit, nil)
	s.Require().NoError(err)
	s.Len(nots, 3)
	s.NotEqual("end", cursor)

	nots2, cursor2, err := s.repo.List(userId, &limit, &cursor)
	s.Require().NoError(err)
	s.Len(nots2, 2)
	s.Equal("end", cursor2)
}

func (s *NotificationsRepoTestSuite) TestGetNotification() {
	userId := uuid.New().String()
	not := domain.Notification{
		Id:        uuid.New().String(),
		Type:      domain.NotificationLikeType,
		Text:      "liked",
		UserId:    userId,
		CreatedAt: time.Now(),
	}
	err := s.repo.Add(not)
	s.Require().NoError(err)

	got, err := s.repo.Get(userId, not.Id)
	s.Require().NoError(err)
	s.Equal(not.Id, got.Id)
	s.Equal(not.Text, got.Text)
}

func (s *NotificationsRepoTestSuite) TestGetNotification_NotFound() {
	userId := uuid.New().String()
	_, err := s.repo.Get(userId, "missing-id")
	s.Error(err)
}

func (s *NotificationsRepoTestSuite) TestGetNotification_MissingUserId() {
	_, err := s.repo.Get("", "some-id")
	s.Error(err)
	s.Contains(err.Error(), "missing user id")
}

func (s *NotificationsRepoTestSuite) TestGetNotification_MissingNotificationId() {
	_, err := s.repo.Get("user-id", "")
	s.Error(err)
	s.Contains(err.Error(), "missing notification id")
}

func (s *NotificationsRepoTestSuite) TestNotificationsIsolatedByUser() {
	userId1 := uuid.New().String()
	userId2 := uuid.New().String()

	err := s.repo.Add(domain.Notification{
		Type:   domain.NotificationLikeType,
		Text:   "for user1",
		UserId: userId1,
	})
	s.Require().NoError(err)

	err = s.repo.Add(domain.Notification{
		Type:   domain.NotificationReplyType,
		Text:   "for user2",
		UserId: userId2,
	})
	s.Require().NoError(err)

	limit := uint64(10)
	nots1, _, err := s.repo.List(userId1, &limit, nil)
	s.Require().NoError(err)
	s.Len(nots1, 1)
	s.Equal("for user1", nots1[0].Text)

	nots2, _, err := s.repo.List(userId2, &limit, nil)
	s.Require().NoError(err)
	s.Len(nots2, 1)
	s.Equal("for user2", nots2[0].Text)
}

func TestNotificationsRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(NotificationsRepoTestSuite))
}
