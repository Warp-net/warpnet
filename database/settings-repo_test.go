//nolint:all
package database

import (
	"testing"

	"github.com/Warp-net/warpnet/config"
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
	user := uuid.New().String()
	got, err := s.repo.GetGatewaySettings(user)
	s.Require().NoError(err)
	s.Empty(got.NodeID)
}

func (s *SettingsRepoTestSuite) TestGatewaySetGet() {
	user := uuid.New().String()
	want := domain.GatewaySettings{NodeID: "12D3KooWCustomGateway"}
	s.Require().NoError(s.repo.SetGatewaySettings(user, want))

	got, err := s.repo.GetGatewaySettings(user)
	s.Require().NoError(err)
	s.Equal(want.NodeID, got.NodeID)

	// Gateway settings must not collide with notification settings for the same user.
	ns, err := s.repo.GetNotificationSettings(user)
	s.Require().NoError(err)
	s.False(ns.EmailEnabled)
}

func (s *SettingsRepoTestSuite) TestGatewayEmptyUserId() {
	_, err := s.repo.GetGatewaySettings("")
	s.Error(err)
	s.Error(s.repo.SetGatewaySettings("", domain.GatewaySettings{}))
}

func (s *SettingsRepoTestSuite) TestRateLimitDefaultsWhenUnset() {
	user := uuid.New().String()
	got, err := s.repo.GetRateLimitSettings(user)
	s.Require().NoError(err)
	s.Equal(ratelimit.Settings{}, got)
}

func (s *SettingsRepoTestSuite) TestRateLimitSetGet() {
	user := uuid.New().String()
	want := ratelimit.Defaults
	want.NetworkHighWater = 500
	s.Require().NoError(s.repo.SetRateLimitSettings(user, want))

	got, err := s.repo.GetRateLimitSettings(user)
	s.Require().NoError(err)
	s.Equal(want, got)

	gs, err := s.repo.GetGatewaySettings(user)
	s.Require().NoError(err)
	s.Empty(gs.NodeID)
}

func (s *SettingsRepoTestSuite) TestRateLimitEmptyUserId() {
	_, err := s.repo.GetRateLimitSettings("")
	s.Error(err)
	s.Error(s.repo.SetRateLimitSettings("", ratelimit.Settings{}))
}

func (s *SettingsRepoTestSuite) TestSponsoredSetGet() {
	user := uuid.New().String()
	got, err := s.repo.GetSponsoredSettings(user)
	s.Require().NoError(err)
	s.Zero(got.MinStakeTRX, "an author who set nothing asks no stake")

	s.Require().NoError(s.repo.SetSponsoredSettings(user, domain.SponsoredSettings{MinStakeTRX: 500}))
	got, err = s.repo.GetSponsoredSettings(user)
	s.Require().NoError(err)
	s.Equal(uint64(500), got.MinStakeTRX)

	gs, err := s.repo.GetGatewaySettings(user)
	s.Require().NoError(err)
	s.Empty(gs.NodeID)

	_, err = s.repo.GetSponsoredSettings("")
	s.Error(err)
	s.Error(s.repo.SetSponsoredSettings("", domain.SponsoredSettings{}))
}

func (s *SettingsRepoTestSuite) TestNetworkSetGet() {
	defaults := config.Config().Node
	user := uuid.New().String()
	got, err := s.repo.GetNetworkSettings(user)
	s.Require().NoError(err)
	s.Equal(domain.NetworkSettings{
		HostV4:    defaults.HostV4,
		HostV6:    defaults.HostV6,
		Port:      defaults.Port,
		Bootstrap: defaults.Bootstrap,
	}, got, "nothing saved: the config values")

	bootstrap := []string{"/ip4/1.2.3.4/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"}
	s.Require().NoError(s.repo.SetNetworkSettings(user, domain.NetworkSettings{Port: "4555", Bootstrap: bootstrap}))
	got, err = s.repo.GetNetworkSettings(user)
	s.Require().NoError(err)
	s.Equal("4555", got.Port)
	s.Equal(bootstrap, got.Bootstrap)
	s.Equal(defaults.HostV4, got.HostV4, "an empty field falls back to the config value")

	ws, err := s.repo.GetWalletSettings(user)
	s.Require().NoError(err)
	s.Equal(config.Config().Wallet.Endpoint, ws.Endpoint, "network and wallet settings live under separate keys")

	_, err = s.repo.GetNetworkSettings("")
	s.Error(err)
	s.Error(s.repo.SetNetworkSettings("", domain.NetworkSettings{}))
}

func (s *SettingsRepoTestSuite) TestWalletSetGet() {
	defaults := config.Config().Wallet
	user := uuid.New().String()
	got, err := s.repo.GetWalletSettings(user)
	s.Require().NoError(err)
	s.Equal(domain.WalletSettings{Endpoint: defaults.Endpoint, BinaryPath: defaults.BinaryPath}, got)

	s.Require().NoError(s.repo.SetWalletSettings(user, domain.WalletSettings{BinaryPath: "/opt/engine"}))
	got, err = s.repo.GetWalletSettings(user)
	s.Require().NoError(err)
	s.Equal(domain.WalletSettings{Endpoint: defaults.Endpoint, BinaryPath: "/opt/engine"}, got)

	_, err = s.repo.GetWalletSettings("")
	s.Error(err)
	s.Error(s.repo.SetWalletSettings("", domain.WalletSettings{}))
}

func TestSettingsRepoTestSuite(t *testing.T) {
	suite.Run(t, new(SettingsRepoTestSuite))
}
