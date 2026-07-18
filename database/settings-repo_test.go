//nolint:all
package database

import (
	"testing"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type SettingsRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *SettingsRepo
}

func (s *SettingsRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	authRepo := NewAuthRepo(s.db, "test")
	s.Require().NoError(authRepo.Authenticate("test", "test"))
	s.repo = NewSettingsRepo(s.db)
}

func (s *SettingsRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *SettingsRepoTestSuite) TestGetDefaultsWhenUnset() {
	user := uuid.New().String()
	got, err := s.repo.GetNotificationSettings(user)
	s.Require().NoError(err)
	s.False(got.EmailEnabled)
	s.Empty(got.Recipient)
}

func (s *SettingsRepoTestSuite) TestSetGet() {
	user := uuid.New().String()
	want := domain.NotificationSettings{
		EmailEnabled: true,
		Recipient:    "me@example.com",
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPUsername: "user",
		SMTPPassword: "secret",
		SMTPUseTLS:   false,
		Types: map[domain.NotificationType]bool{
			domain.NotificationNewUserType: true,
			domain.NotificationLikeType:    false,
		},
	}
	s.Require().NoError(s.repo.SetNotificationSettings(user, want))

	got, err := s.repo.GetNotificationSettings(user)
	s.Require().NoError(err)
	s.Equal(want.EmailEnabled, got.EmailEnabled)
	s.Equal(want.Recipient, got.Recipient)
	s.Equal(want.SMTPHost, got.SMTPHost)
	s.Equal(want.SMTPPort, got.SMTPPort)
	s.Equal(want.SMTPPassword, got.SMTPPassword)
	s.True(got.Types[domain.NotificationNewUserType])
	s.False(got.Types[domain.NotificationLikeType])
}

func (s *SettingsRepoTestSuite) TestEmptyUserId() {
	_, err := s.repo.GetNotificationSettings("")
	s.Error(err)
	s.Error(s.repo.SetNotificationSettings("", domain.NotificationSettings{}))
}

func TestSettingsRepoTestSuite(t *testing.T) {
	suite.Run(t, new(SettingsRepoTestSuite))
}
