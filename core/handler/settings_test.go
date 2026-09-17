//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/fediverse"
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
	getFn func(userId string) (domain.GatewaySettings, error)
	setFn func(userId string, s domain.GatewaySettings) error
}

func (s stubGatewayRepo) GetGatewaySettings(userId string) (domain.GatewaySettings, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.GatewaySettings{}, nil
}

func (s stubGatewayRepo) SetGatewaySettings(userId string, gs domain.GatewaySettings) error {
	if s.setFn != nil {
		return s.setFn(userId, gs)
	}
	return nil
}

func TestStreamGetGatewaySettingsHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("returns saved node id", func(t *testing.T) {
		h := StreamGetGatewaySettingsHandler(stubGatewayRepo{
			getFn: func(string) (domain.GatewaySettings, error) {
				return domain.GatewaySettings{NodeID: "12D3KooWCustom"}, nil
			},
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h([]byte("{}"), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.GetGatewaySettingsResponse).NodeID != "12D3KooWCustom" {
			t.Fatalf("unexpected settings: %+v", resp)
		}
	})

	t.Run("defaults when unset", func(t *testing.T) {
		h := StreamGetGatewaySettingsHandler(stubGatewayRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
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
			getFn: func(string) (domain.GatewaySettings, error) { return domain.GatewaySettings{}, repoErr },
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		if _, err := h([]byte("{}"), nil); !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
	})
}

func TestStreamUpdateGatewaySettingsHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUpdateGatewaySettingsHandler(stubGatewayRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty owner", func(t *testing.T) {
		h := StreamUpdateGatewaySettingsHandler(stubGatewayRepo{}, stubAuth{owner: domain.Owner{}})
		if _, err := h(marshal(t, event.UpdateGatewaySettingsEvent{NodeID: "x"}), nil); err == nil {
			t.Fatal("expected empty-owner error")
		}
	})

	t.Run("persists and echoes node id", func(t *testing.T) {
		var saved domain.GatewaySettings
		h := StreamUpdateGatewaySettingsHandler(stubGatewayRepo{
			setFn: func(userId string, gs domain.GatewaySettings) error {
				if userId != owner {
					t.Fatalf("expected owner id %q, got %q", owner, userId)
				}
				saved = gs
				return nil
			},
		}, stubAuth{owner: domain.Owner{UserId: owner}})
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
			setFn: func(_ string, gs domain.GatewaySettings) error { saved = gs; return nil },
		}, stubAuth{owner: domain.Owner{UserId: owner}})
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
			setFn: func(string, domain.GatewaySettings) error { return repoErr },
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		if _, err := h(marshal(t, event.UpdateGatewaySettingsEvent{NodeID: "x"}), nil); !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
	})
}

type stubRateLimitRepo struct {
	getFn func(userId string) (event.RateLimitSettings, error)
	setFn func(userId string, s event.RateLimitSettings) error
}

func (s stubRateLimitRepo) GetRateLimitSettings(userId string) (event.RateLimitSettings, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return event.RateLimitSettings{}, nil
}

func (s stubRateLimitRepo) SetRateLimitSettings(userId string, rs event.RateLimitSettings) error {
	if s.setFn != nil {
		return s.setFn(userId, rs)
	}
	return nil
}

func TestStreamGetRateLimitSettingsHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("returns saved limits", func(t *testing.T) {
		h := StreamGetRateLimitSettingsHandler(stubRateLimitRepo{
			getFn: func(userId string) (event.RateLimitSettings, error) {
				if userId != owner {
					t.Fatalf("expected owner id %q, got %q", owner, userId)
				}
				return event.RateLimitSettings{NetworkLowWater: 10, NetworkHighWater: 100}, nil
			},
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h([]byte("{}"), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		s := resp.(event.GetRateLimitSettingsResponse)
		if s.NetworkLowWater != 10 || s.NetworkHighWater != 100 {
			t.Fatalf("unexpected settings: %+v", s)
		}
	})

	t.Run("defaults when unset", func(t *testing.T) {
		h := StreamGetRateLimitSettingsHandler(stubRateLimitRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h([]byte("{}"), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.GetRateLimitSettingsResponse) != event.DefaultRateLimits {
			t.Fatalf("expected default limits, got %+v", resp)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamGetRateLimitSettingsHandler(stubRateLimitRepo{
			getFn: func(string) (event.RateLimitSettings, error) { return event.RateLimitSettings{}, repoErr },
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		if _, err := h([]byte("{}"), nil); !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
	})
}

func TestStreamUpdateRateLimitSettingsHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUpdateRateLimitSettingsHandler(stubRateLimitRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty owner", func(t *testing.T) {
		h := StreamUpdateRateLimitSettingsHandler(stubRateLimitRepo{}, stubAuth{owner: domain.Owner{}})
		if _, err := h(marshal(t, event.UpdateRateLimitSettingsEvent{}), nil); err == nil {
			t.Fatal("expected empty-owner error")
		}
	})

	t.Run("persists and echoes limits", func(t *testing.T) {
		var saved event.RateLimitSettings
		h := StreamUpdateRateLimitSettingsHandler(stubRateLimitRepo{
			setFn: func(userId string, rs event.RateLimitSettings) error {
				if userId != owner {
					t.Fatalf("expected owner id %q, got %q", owner, userId)
				}
				saved = rs
				return nil
			},
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		in := event.DefaultRateLimits
		in.StreamWritePerMinute = 600
		resp, err := h(marshal(t, event.UpdateRateLimitSettingsEvent(in)), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if saved.StreamWritePerMinute != 600 {
			t.Fatalf("unexpected saved: %+v", saved)
		}
		if resp.(event.GetRateLimitSettingsResponse).StreamWritePerMinute != 600 {
			t.Fatalf("expected echoed settings, got %+v", resp)
		}
	})

	t.Run("unset limits fall back to defaults", func(t *testing.T) {
		var saved event.RateLimitSettings
		h := StreamUpdateRateLimitSettingsHandler(stubRateLimitRepo{
			setFn: func(_ string, rs event.RateLimitSettings) error { saved = rs; return nil },
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		if _, err := h(marshal(t, event.UpdateRateLimitSettingsEvent{DiscoveryBurst: 64}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if saved.DiscoveryBurst != 64 || saved.StreamReadBurst != event.DefaultRateLimits.StreamReadBurst {
			t.Fatalf("expected defaults for unset limits, got %+v", saved)
		}
	})

	t.Run("high water below low water", func(t *testing.T) {
		h := StreamUpdateRateLimitSettingsHandler(stubRateLimitRepo{
			setFn: func(string, event.RateLimitSettings) error {
				t.Fatal("must not persist invalid limits")
				return nil
			},
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		in := event.DefaultRateLimits
		in.NetworkLowWater, in.NetworkHighWater = 100, 10
		if _, err := h(marshal(t, event.UpdateRateLimitSettingsEvent(in)), nil); err == nil {
			t.Fatal("expected water mark error")
		}
	})

	t.Run("repo error surfaces", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamUpdateRateLimitSettingsHandler(stubRateLimitRepo{
			setFn: func(string, event.RateLimitSettings) error { return repoErr },
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		if _, err := h(marshal(t, event.UpdateRateLimitSettingsEvent{}), nil); !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
	})
}
