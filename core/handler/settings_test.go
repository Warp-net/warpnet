//nolint:all
package handler

import (
	"errors"
	"testing"

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

type stubMailer struct {
	sendFn func(cfg domain.NotificationSettings, subject, body string) error
}

func (s stubMailer) Send(cfg domain.NotificationSettings, subject, body string) error {
	if s.sendFn != nil {
		return s.sendFn(cfg, subject, body)
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
		out := resp.(event.GetNotificationSettingsResponse)
		if out.Recipient != "x@y.z" {
			t.Fatalf("expected echoed settings, got %+v", out)
		}
	})
}

func TestStreamTestEmailHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		h := StreamTestEmailHandler(stubMailer{})
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty recipient", func(t *testing.T) {
		h := StreamTestEmailHandler(stubMailer{})
		if _, err := h(marshal(t, event.TestEmailEvent{SMTPHost: "h"}), nil); err == nil {
			t.Fatal("expected empty-recipient error")
		}
	})

	t.Run("empty smtp host", func(t *testing.T) {
		h := StreamTestEmailHandler(stubMailer{})
		if _, err := h(marshal(t, event.TestEmailEvent{Recipient: "a@b.c"}), nil); err == nil {
			t.Fatal("expected empty-host error")
		}
	})

	t.Run("sender error surfaces", func(t *testing.T) {
		sendErr := errors.New("smtp down")
		h := StreamTestEmailHandler(stubMailer{sendFn: func(domain.NotificationSettings, string, string) error { return sendErr }})
		if _, err := h(marshal(t, event.TestEmailEvent{Recipient: "a@b.c", SMTPHost: "h"}), nil); !errors.Is(err, sendErr) {
			t.Fatalf("expected sender error, got %v", err)
		}
	})

	t.Run("happy path", func(t *testing.T) {
		var called bool
		h := StreamTestEmailHandler(stubMailer{sendFn: func(domain.NotificationSettings, string, string) error {
			called = true
			return nil
		}})
		resp, err := h(marshal(t, event.TestEmailEvent{Recipient: "a@b.c", SMTPHost: "h"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !called {
			t.Fatal("expected sender to be called")
		}
		if resp != event.Accepted {
			t.Fatalf("expected Accepted, got %v", resp)
		}
	})
}
