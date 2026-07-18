//nolint:all
package notifications

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/domain"
)

type fakeStore struct {
	added []domain.Notification
	err   error
}

func (f *fakeStore) Add(n domain.Notification) error {
	f.added = append(f.added, n)
	return f.err
}

type fakeSettings struct {
	cfg domain.NotificationSettings
	err error
}

func (f fakeSettings) GetNotificationSettings(string) (domain.NotificationSettings, error) {
	return f.cfg, f.err
}

type fakeSender struct {
	mu   sync.Mutex
	sent int
	ch   chan struct{}
}

func newFakeSender() *fakeSender { return &fakeSender{ch: make(chan struct{}, 8)} }

func (f *fakeSender) Send(domain.NotificationSettings, string, string) error {
	f.mu.Lock()
	f.sent++
	f.mu.Unlock()
	f.ch <- struct{}{}
	return nil
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent
}

func (f *fakeSender) waitSend(t *testing.T) {
	t.Helper()
	select {
	case <-f.ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for email send")
	}
}

func TestService_AddFansOutAndJoinsErrors(t *testing.T) {
	store := &fakeStore{err: errors.New("store down")}
	// A second store channel that succeeds, to prove fan-out continues past a failure.
	ok := &fakeStore{}
	svc := New(NewStoreChannel(store), NewStoreChannel(ok))

	err := svc.Add(domain.Notification{Type: domain.NotificationFollowType, UserId: "u"})
	if err == nil {
		t.Fatal("expected joined error from failing channel")
	}
	if len(store.added) != 1 || len(ok.added) != 1 {
		t.Fatalf("expected both channels invoked, got %d and %d", len(store.added), len(ok.added))
	}
}

func TestStoreChannel_Persists(t *testing.T) {
	store := &fakeStore{}
	svc := New(NewStoreChannel(store))
	if err := svc.Add(domain.Notification{Type: domain.NotificationLikeType, UserId: "u"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if len(store.added) != 1 {
		t.Fatalf("expected 1 stored, got %d", len(store.added))
	}
}

func TestEmailChannel_SendsWhenOptedIn(t *testing.T) {
	sender := newFakeSender()
	settings := fakeSettings{cfg: domain.NotificationSettings{
		EmailEnabled: true,
		Recipient:    "a@b.c",
		Types:        map[domain.NotificationType]bool{domain.NotificationNewUserType: true},
	}}
	svc := New(NewEmailChannel(settings, sender))

	if err := svc.Add(domain.Notification{Type: domain.NotificationNewUserType, UserId: "u"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	sender.waitSend(t)
	if sender.count() != 1 {
		t.Fatalf("expected 1 email, got %d", sender.count())
	}
}

func TestEmailChannel_SkipsWhenNotOptedIn(t *testing.T) {
	cases := map[string]domain.NotificationSettings{
		"disabled":     {EmailEnabled: false, Recipient: "a@b.c", Types: map[domain.NotificationType]bool{domain.NotificationLikeType: true}},
		"no recipient": {EmailEnabled: true, Types: map[domain.NotificationType]bool{domain.NotificationLikeType: true}},
		"type off":     {EmailEnabled: true, Recipient: "a@b.c", Types: map[domain.NotificationType]bool{domain.NotificationFollowType: true}},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			sender := newFakeSender()
			svc := New(NewEmailChannel(fakeSettings{cfg: cfg}, sender))
			if err := svc.Add(domain.Notification{Type: domain.NotificationLikeType, UserId: "u"}); err != nil {
				t.Fatalf("add: %v", err)
			}
			time.Sleep(100 * time.Millisecond)
			if sender.count() != 0 {
				t.Fatalf("expected no email, got %d", sender.count())
			}
		})
	}
}
