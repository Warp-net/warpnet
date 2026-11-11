//nolint:all
package database

import (
	"testing"

	"github.com/Warp-net/warpnet/core/ratelimit"
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

func (s *SettingsRepoTestSuite) SetupTest() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	authRepo := NewAuthRepo(s.db, "test")
	s.Require().NoError(authRepo.Authenticate("test", "test"))
	s.repo = NewSettingsRepo(s.db)
}

func (s *SettingsRepoTestSuite) TearDownTest() {
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
			domain.NotificationNewUserType:  true,
			domain.NotificationReactionType: false,
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
	s.False(got.Types[domain.NotificationReactionType])
}

func (s *SettingsRepoTestSuite) TestEmptyUserId() {
	_, err := s.repo.GetNotificationSettings("")
	s.Error(err)
	s.Error(s.repo.SetNotificationSettings("", domain.NotificationSettings{}))
}

func (s *SettingsRepoTestSuite) TestGatewayDefaultsWhenUnset() {
	got, err := s.repo.GetGatewaySettings()
	s.Require().NoError(err)
	s.Empty(got.NodeID)
}

func (s *SettingsRepoTestSuite) TestGatewaySetGet() {
	want := domain.GatewaySettings{NodeID: "12D3KooWCustomGateway"}
	s.Require().NoError(s.repo.SetGatewaySettings(want))

	got, err := s.repo.GetGatewaySettings()
	s.Require().NoError(err)
	s.Equal(want.NodeID, got.NodeID)

	ns, err := s.repo.GetNotificationSettings(uuid.New().String())
	s.Require().NoError(err)
	s.False(ns.EmailEnabled)
}

func (s *SettingsRepoTestSuite) TestRateLimitDefaultsWhenUnset() {
	got, err := s.repo.GetRateLimitSettings()
	s.Require().NoError(err)
	s.Equal(ratelimit.Settings{}, got)
}

func (s *SettingsRepoTestSuite) TestRateLimitSetGet() {
	want := ratelimit.Defaults
	want.NetworkHighWater = 500
	s.Require().NoError(s.repo.SetRateLimitSettings(want))

	got, err := s.repo.GetRateLimitSettings()
	s.Require().NoError(err)
	s.Equal(want, got)

	gs, err := s.repo.GetGatewaySettings()
	s.Require().NoError(err)
	s.Empty(gs.NodeID)
}

func (s *SettingsRepoTestSuite) TestSponsoredSetGet() {
	got, err := s.repo.GetSponsoredSettings()
	s.Require().NoError(err)
	s.Zero(got.MinStakeTRX, "an author who set nothing asks no stake")

	s.Require().NoError(s.repo.SetSponsoredSettings(domain.SponsoredSettings{MinStakeTRX: 500}))
	got, err = s.repo.GetSponsoredSettings()
	s.Require().NoError(err)
	s.Equal(uint64(500), got.MinStakeTRX)

	gs, err := s.repo.GetGatewaySettings()
	s.Require().NoError(err)
	s.Empty(gs.NodeID)
}

func (s *SettingsRepoTestSuite) TestNetworkSetGet() {
	got, err := s.repo.GetNetworkSettings()
	s.Require().NoError(err)
	s.Equal(domain.NetworkSettings{}, got)

	want := domain.NetworkSettings{Port: "4555", Bootstrap: []string{"/ip4/1.2.3.4/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"}}
	s.Require().NoError(s.repo.SetNetworkSettings(want))
	got, err = s.repo.GetNetworkSettings()
	s.Require().NoError(err)
	s.Equal(want, got)

	ws, err := s.repo.GetWalletSettings()
	s.Require().NoError(err)
	s.Equal(domain.WalletSettings{}, ws, "network and wallet settings live under separate keys")
}

func (s *SettingsRepoTestSuite) TestWalletSetGet() {
	want := domain.WalletSettings{Endpoint: "https://nile.example"}
	s.Require().NoError(s.repo.SetWalletSettings(want))
	got, err := s.repo.GetWalletSettings()
	s.Require().NoError(err)
	s.Equal(want, got)

	ns, err := s.repo.GetNetworkSettings()
	s.Require().NoError(err)
	s.Equal(domain.NetworkSettings{}, ns)
}

func TestSettingsRepoTestSuite(t *testing.T) {
	suite.Run(t, new(SettingsRepoTestSuite))
}
