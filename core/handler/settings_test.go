//nolint:all
package handler

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/ratelimit"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubSettingsRepo struct {
	getFn func(userId string) (domain.NotificationSettings, error)
	setFn func(userId string, s domain.NotificationSettings) error
}

func (s stubSettingsRepo) GetNotificationSettings(userId string) (domain.NotificationSettings, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.NotificationSettings{}, nil
}

func (s stubSettingsRepo) SetNotificationSettings(userId string, ns domain.NotificationSettings) error {
	if s.setFn != nil {
		return s.setFn(userId, ns)
	}
	return nil
}

func TestStreamGetNotificationSettingsHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("returns saved settings", func(t *testing.T) {
		h := StreamGetNotificationSettingsHandler(stubSettingsRepo{
			getFn: func(userId string) (domain.NotificationSettings, error) {
				if userId != owner {
					t.Fatalf("expected owner id %q, got %q", owner, userId)
				}
				return domain.NotificationSettings{EmailEnabled: true, Recipient: "a@b.c"}, nil
			},
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h([]byte("{}"), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		s := resp.(event.GetNotificationSettingsResponse)
		if !s.EmailEnabled || s.Recipient != "a@b.c" {
			t.Fatalf("unexpected settings: %+v", s)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamGetNotificationSettingsHandler(stubSettingsRepo{
			getFn: func(string) (domain.NotificationSettings, error) { return domain.NotificationSettings{}, repoErr },
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		if _, err := h([]byte("{}"), nil); !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
	})
}

func TestStreamUpdateNotificationSettingsHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUpdateNotificationSettingsHandler(stubSettingsRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty owner", func(t *testing.T) {
		h := StreamUpdateNotificationSettingsHandler(stubSettingsRepo{}, stubAuth{owner: domain.Owner{}})
		if _, err := h(marshal(t, event.UpdateNotificationSettingsEvent{}), nil); err == nil {
			t.Fatal("expected empty-owner error")
		}
	})

	t.Run("persists and echoes settings", func(t *testing.T) {
		var saved domain.NotificationSettings
		h := StreamUpdateNotificationSettingsHandler(stubSettingsRepo{
			setFn: func(userId string, s domain.NotificationSettings) error {
				if userId != owner {
					t.Fatalf("expected owner id %q, got %q", owner, userId)
				}
				saved = s
				return nil
			},
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		in := event.UpdateNotificationSettingsEvent{EmailEnabled: true, Recipient: "x@y.z"}
		resp, err := h(marshal(t, in), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !saved.EmailEnabled || saved.Recipient != "x@y.z" {
			t.Fatalf("unexpected saved: %+v", saved)
		}
		if resp.(event.GetNotificationSettingsResponse).Recipient != "x@y.z" {
			t.Fatalf("expected echoed settings, got %+v", resp)
		}
	})

	t.Run("repo error surfaces", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamUpdateNotificationSettingsHandler(stubSettingsRepo{
			setFn: func(string, domain.NotificationSettings) error { return repoErr },
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		if _, err := h(marshal(t, event.UpdateNotificationSettingsEvent{}), nil); !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
	})
}

type stubGatewayRepo struct {
	getFn func() (domain.GatewaySettings, error)
	setFn func(s domain.GatewaySettings) error
}

func (s stubGatewayRepo) GetGatewaySettings() (domain.GatewaySettings, error) {
	if s.getFn != nil {
		return s.getFn()
	}
	return domain.GatewaySettings{}, nil
}

func (s stubGatewayRepo) SetGatewaySettings(gs domain.GatewaySettings) error {
	if s.setFn != nil {
		return s.setFn(gs)
	}
	return nil
}

func TestStreamGetGatewaySettingsHandler(t *testing.T) {
	t.Run("returns saved node id", func(t *testing.T) {
		h := StreamGetGatewaySettingsHandler(stubGatewayRepo{
			getFn: func() (domain.GatewaySettings, error) {
				return domain.GatewaySettings{NodeID: "12D3KooWCustom"}, nil
			},
		})
		resp, err := h([]byte("{}"), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.GetGatewaySettingsResponse).NodeID != "12D3KooWCustom" {
			t.Fatalf("unexpected settings: %+v", resp)
		}
	})

	t.Run("defaults when unset", func(t *testing.T) {
		h := StreamGetGatewaySettingsHandler(stubGatewayRepo{})
		resp, err := h([]byte("{}"), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.GetGatewaySettingsResponse).NodeID != fediverse.DefaultGatewayNodeID {
			t.Fatalf("expected default node id, got %+v", resp)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamGetGatewaySettingsHandler(stubGatewayRepo{
			getFn: func() (domain.GatewaySettings, error) { return domain.GatewaySettings{}, repoErr },
		})
		if _, err := h([]byte("{}"), nil); !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
	})
}

func TestStreamUpdateGatewaySettingsHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUpdateGatewaySettingsHandler(stubGatewayRepo{})
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("persists and echoes node id", func(t *testing.T) {
		var saved domain.GatewaySettings
		h := StreamUpdateGatewaySettingsHandler(stubGatewayRepo{
			setFn: func(gs domain.GatewaySettings) error {
				saved = gs
				return nil
			},
		})
		resp, err := h(marshal(t, event.UpdateGatewaySettingsEvent{NodeID: "12D3KooWCustom"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if saved.NodeID != "12D3KooWCustom" {
			t.Fatalf("unexpected saved: %+v", saved)
		}
		if resp.(event.GetGatewaySettingsResponse).NodeID != "12D3KooWCustom" {
			t.Fatalf("expected echoed settings, got %+v", resp)
		}
	})

	t.Run("empty node id falls back to default", func(t *testing.T) {
		var saved domain.GatewaySettings
		h := StreamUpdateGatewaySettingsHandler(stubGatewayRepo{
			setFn: func(gs domain.GatewaySettings) error { saved = gs; return nil },
		})
		if _, err := h(marshal(t, event.UpdateGatewaySettingsEvent{NodeID: ""}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if saved.NodeID != fediverse.DefaultGatewayNodeID {
			t.Fatalf("expected default node id persisted, got %+v", saved)
		}
	})

	t.Run("repo error surfaces", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamUpdateGatewaySettingsHandler(stubGatewayRepo{
			setFn: func(domain.GatewaySettings) error { return repoErr },
		})
		if _, err := h(marshal(t, event.UpdateGatewaySettingsEvent{NodeID: "x"}), nil); !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
	})
}

type stubRateLimitRepo struct {
	getFn func() (ratelimit.Settings, error)
	setFn func(s ratelimit.Settings) error
}

func (s stubRateLimitRepo) GetRateLimitSettings() (ratelimit.Settings, error) {
	if s.getFn != nil {
		return s.getFn()
	}
	return ratelimit.Settings{}, nil
}

func (s stubRateLimitRepo) SetRateLimitSettings(rs ratelimit.Settings) error {
	if s.setFn != nil {
		return s.setFn(rs)
	}
	return nil
}

func TestStreamGetRateLimitSettingsHandler(t *testing.T) {
	t.Run("returns saved limits", func(t *testing.T) {
		h := StreamGetRateLimitSettingsHandler(stubRateLimitRepo{
			getFn: func() (ratelimit.Settings, error) {
				return ratelimit.Settings{NetworkLowWater: 10, NetworkHighWater: 100}, nil
			},
		})
		resp, err := h([]byte("{}"), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		s := resp.(ratelimit.Settings)
		if s.NetworkLowWater != 10 || s.NetworkHighWater != 100 {
			t.Fatalf("unexpected settings: %+v", s)
		}
	})

	t.Run("defaults when unset", func(t *testing.T) {
		h := StreamGetRateLimitSettingsHandler(stubRateLimitRepo{})
		resp, err := h([]byte("{}"), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(ratelimit.Settings) != ratelimit.Defaults {
			t.Fatalf("expected default limits, got %+v", resp)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamGetRateLimitSettingsHandler(stubRateLimitRepo{
			getFn: func() (ratelimit.Settings, error) { return ratelimit.Settings{}, repoErr },
		})
		if _, err := h([]byte("{}"), nil); !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
	})
}

func TestStreamUpdateRateLimitSettingsHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUpdateRateLimitSettingsHandler(stubRateLimitRepo{})
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("persists and echoes limits", func(t *testing.T) {
		var saved ratelimit.Settings
		h := StreamUpdateRateLimitSettingsHandler(stubRateLimitRepo{
			setFn: func(rs ratelimit.Settings) error {
				saved = rs
				return nil
			},
		})
		in := ratelimit.Defaults
		in.StreamWritePerMinute = 600
		resp, err := h(marshal(t, in), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if saved.StreamWritePerMinute != 600 {
			t.Fatalf("unexpected saved: %+v", saved)
		}
		if resp.(ratelimit.Settings).StreamWritePerMinute != 600 {
			t.Fatalf("expected echoed settings, got %+v", resp)
		}
	})

	t.Run("unset limits fall back to defaults", func(t *testing.T) {
		var saved ratelimit.Settings
		h := StreamUpdateRateLimitSettingsHandler(stubRateLimitRepo{
			setFn: func(rs ratelimit.Settings) error { saved = rs; return nil },
		})
		if _, err := h(marshal(t, ratelimit.Settings{DiscoveryBurst: 64}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if saved.DiscoveryBurst != 64 || saved.StreamReadBurst != ratelimit.Defaults.StreamReadBurst {
			t.Fatalf("expected defaults for unset limits, got %+v", saved)
		}
	})

	t.Run("high water below low water", func(t *testing.T) {
		h := StreamUpdateRateLimitSettingsHandler(stubRateLimitRepo{
			setFn: func(ratelimit.Settings) error {
				t.Fatal("must not persist invalid limits")
				return nil
			},
		})
		in := ratelimit.Defaults
		in.NetworkLowWater, in.NetworkHighWater = 100, 10
		if _, err := h(marshal(t, in), nil); err == nil {
			t.Fatal("expected water mark error")
		}
	})

	t.Run("repo error surfaces", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamUpdateRateLimitSettingsHandler(stubRateLimitRepo{
			setFn: func(ratelimit.Settings) error { return repoErr },
		})
		if _, err := h(marshal(t, ratelimit.Settings{}), nil); !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
	})
}

type stubSponsoredSettingsRepo struct {
	saved domain.SponsoredSettings
}

func (s *stubSponsoredSettingsRepo) GetSponsoredSettings() (domain.SponsoredSettings, error) {
	return s.saved, nil
}

func (s *stubSponsoredSettingsRepo) SetSponsoredSettings(ss domain.SponsoredSettings) error {
	s.saved = ss
	return nil
}

func TestStreamSponsoredSettingsHandlers(t *testing.T) {
	repo := &stubSponsoredSettingsRepo{}

	resp, err := StreamGetSponsoredSettingsHandler(repo)([]byte("{}"), nil)
	if err != nil || resp.(event.GetSponsoredSettingsResponse).MinStakeTRX != 0 {
		t.Fatalf("an author who set nothing asks no stake: %+v, %v", resp, err)
	}

	resp, err = StreamUpdateSponsoredSettingsHandler(repo)(marshal(t, event.UpdateSponsoredSettingsEvent{MinStakeTRX: 500}), nil)
	if err != nil || resp.(event.GetSponsoredSettingsResponse).MinStakeTRX != 500 {
		t.Fatalf("update = %+v, %v", resp, err)
	}
	saved := repo.saved
	if saved.MinStakeTRX != 500 || saved.PreviousMinStakeTRX != 0 || time.Since(saved.ChangedAt) > time.Minute {
		t.Fatalf("saved = %+v, want 500 with the old 0 kept for claims", saved)
	}

	if _, err := StreamUpdateSponsoredSettingsHandler(repo)(marshal(t, event.UpdateSponsoredSettingsEvent{MinStakeTRX: 800}), nil); err != nil {
		t.Fatal(err)
	}
	if again := repo.saved; again.MinStakeTRX != 800 || again.PreviousMinStakeTRX != 0 {
		t.Fatalf("saved = %+v: a second raise within the window keeps the lowest threshold a claim may still be held to", again)
	}
	before := repo.saved
	if _, err := StreamUpdateSponsoredSettingsHandler(repo)(marshal(t, event.UpdateSponsoredSettingsEvent{MinStakeTRX: 800}), nil); err != nil {
		t.Fatal(err)
	}
	if repo.saved != before {
		t.Fatalf("saving the same threshold moved its history: %+v -> %+v", before, repo.saved)
	}

	resp, err = StreamGetSponsoredSettingsHandler(repo)([]byte("{}"), nil)
	if err != nil || resp.(event.GetSponsoredSettingsResponse).MinStakeTRX != 800 {
		t.Fatalf("get after update = %+v, %v", resp, err)
	}

	if _, err := StreamUpdateSponsoredSettingsHandler(repo)([]byte("{"), nil); err == nil {
		t.Fatal("expected a payload error")
	}
}

type stubNetworkSettingsRepo struct {
	saved domain.NetworkSettings
	err   error
}

func (s *stubNetworkSettingsRepo) GetNetworkSettings() (domain.NetworkSettings, error) {
	return s.saved, s.err
}

func (s *stubNetworkSettingsRepo) SetNetworkSettings(ns domain.NetworkSettings) error {
	s.saved = ns
	return s.err
}

func TestStreamNetworkSettingsHandlers(t *testing.T) {
	const bootstrapAddr = "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"
	defaults := domain.NetworkSettings{HostV4: "0.0.0.0", HostV6: "::", Port: "4001", Bootstrap: []string{bootstrapAddr}}
	t.Run("get", func(t *testing.T) {
		repo := &stubNetworkSettingsRepo{saved: domain.NetworkSettings{Port: "4555"}}
		resp, err := StreamGetNetworkSettingsHandler(repo, defaults)([]byte("{}"), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		got := resp.(event.GetNetworkSettingsResponse)
		if got.Port != "4555" || got.HostV4 != defaults.HostV4 || len(got.Bootstrap) != 1 {
			t.Fatalf("expected the saved port and default rest, got %+v", got)
		}
	})

	t.Run("get repo error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		repo := &stubNetworkSettingsRepo{err: repoErr}
		if _, err := StreamGetNetworkSettingsHandler(repo, defaults)([]byte("{}"), nil); !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
	})

	t.Run("update", func(t *testing.T) {
		repo := &stubNetworkSettingsRepo{}
		in := domain.NetworkSettings{HostV4: "0.0.0.0", HostV6: "::", Port: "4555", Bootstrap: []string{bootstrapAddr}}
		resp, err := StreamUpdateNetworkSettingsHandler(repo)(marshal(t, in), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !reflect.DeepEqual(repo.saved, in) {
			t.Fatalf("unexpected saved: %+v", repo.saved)
		}
		if !reflect.DeepEqual(domain.NetworkSettings(resp.(event.GetNetworkSettingsResponse)), in) {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("dns bootstrap peer", func(t *testing.T) {
		repo := &stubNetworkSettingsRepo{}
		in := domain.NetworkSettings{Bootstrap: []string{"/dns/relay.example.org/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"}}
		if _, err := StreamUpdateNetworkSettingsHandler(repo)(marshal(t, in), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(repo.saved.Bootstrap) != 1 {
			t.Fatalf("unexpected saved: %+v", repo.saved)
		}
	})

	t.Run("invalid payload", func(t *testing.T) {
		if _, err := StreamUpdateNetworkSettingsHandler(&stubNetworkSettingsRepo{})([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	invalid := map[string]domain.NetworkSettings{
		"ipv6 in the ipv4 field": {HostV4: "::1"},
		"ipv4 in the ipv6 field": {HostV6: "127.0.0.1"},
		"garbage host":           {HostV4: "localhost"},
		"port zero":              {Port: "0"},
		"port too big":           {Port: "65536"},
		"port not a number":      {Port: "abc"},
		"bootstrap without id":   {Bootstrap: []string{"/ip4/1.2.3.4/tcp/4001"}},
	}
	for name, in := range invalid {
		t.Run(name, func(t *testing.T) {
			repo := &stubNetworkSettingsRepo{}
			if _, err := StreamUpdateNetworkSettingsHandler(repo)(marshal(t, in), nil); err == nil {
				t.Fatalf("expected validation error for %+v", in)
			}
			if !reflect.DeepEqual(repo.saved, domain.NetworkSettings{}) {
				t.Fatalf("invalid settings must not be saved, got %+v", repo.saved)
			}
		})
	}
}

type stubWalletSettingsRepo struct {
	saved domain.WalletSettings
	err   error
}

func (s *stubWalletSettingsRepo) GetWalletSettings() (domain.WalletSettings, error) {
	return s.saved, s.err
}

func (s *stubWalletSettingsRepo) SetWalletSettings(ws domain.WalletSettings) error {
	s.saved = ws
	return s.err
}

func TestStreamWalletSettingsHandlers(t *testing.T) {
	defaults := domain.WalletSettings{Endpoint: "https://api.trongrid.io"}
	t.Run("get", func(t *testing.T) {
		repo := &stubWalletSettingsRepo{saved: domain.WalletSettings{BinaryPath: "/opt/engine"}}
		resp, err := StreamGetWalletSettingsHandler(repo, defaults)([]byte("{}"), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got := resp.(event.GetWalletSettingsResponse); got.Endpoint != defaults.Endpoint || got.BinaryPath != "/opt/engine" {
			t.Fatalf("unexpected settings: %+v", resp)
		}
	})

	t.Run("update", func(t *testing.T) {
		repo := &stubWalletSettingsRepo{}
		in := domain.WalletSettings{Endpoint: "https://nile.trongrid.io", BinaryPath: "/opt/engine"}
		resp, err := StreamUpdateWalletSettingsHandler(repo)(marshal(t, in), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if repo.saved != in || domain.WalletSettings(resp.(event.GetWalletSettingsResponse)) != in {
			t.Fatalf("unexpected saved %+v, response %+v", repo.saved, resp)
		}
	})

	for name, endpoint := range map[string]string{"without host": "https://", "not http": "ftp://api.trongrid.io"} {
		t.Run(name, func(t *testing.T) {
			repo := &stubWalletSettingsRepo{}
			in := domain.WalletSettings{Endpoint: endpoint}
			if _, err := StreamUpdateWalletSettingsHandler(repo)(marshal(t, in), nil); err == nil {
				t.Fatalf("expected validation error for %q", endpoint)
			}
			if repo.saved != (domain.WalletSettings{}) {
				t.Fatalf("invalid settings must not be saved, got %+v", repo.saved)
			}
		})
	}
}
