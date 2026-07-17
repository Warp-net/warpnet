//nolint:all
package mailer

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/domain"
)

type fakeStore struct {
	added []domain.Notification
	err   error
}

func (f *fakeStore) Add(not domain.Notification) error {
	f.added = append(f.added, not)
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
	sent []string
	ch   chan struct{}
}

func newFakeSender() *fakeSender { return &fakeSender{ch: make(chan struct{}, 8)} }

func (f *fakeSender) Send(cfg domain.NotificationSettings, subject, body string) error {
	f.mu.Lock()
	f.sent = append(f.sent, subject)
	f.mu.Unlock()
	f.ch <- struct{}{}
	return nil
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

// waitSend blocks until an email is sent or the timeout elapses.
func (f *fakeSender) waitSend(t *testing.T) {
	t.Helper()
	select {
	case <-f.ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for email send")
	}
}

func TestNotifyingRepo_Add_EmailsWhenEnabled(t *testing.T) {
	store := &fakeStore{}
	sender := newFakeSender()
	settings := fakeSettings{cfg: domain.NotificationSettings{
		EmailEnabled: true,
		Recipient:    "a@b.c",
		Types:        map[domain.NotificationType]bool{domain.NotificationNewUserType: true},
	}}
	r := NewNotifyingRepo(store, settings, sender)

	if err := r.Add(domain.Notification{Type: domain.NotificationNewUserType, UserId: "owner-1", Text: "bob joined"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(store.added) != 1 {
		t.Fatalf("expected notification persisted, got %d", len(store.added))
	}
	sender.waitSend(t)
	if sender.count() != 1 {
		t.Fatalf("expected 1 email, got %d", sender.count())
	}
}

func TestNotifyingRepo_Add_NoEmailWhenTypeDisabled(t *testing.T) {
	store := &fakeStore{}
	sender := newFakeSender()
	settings := fakeSettings{cfg: domain.NotificationSettings{
		EmailEnabled: true,
		Recipient:    "a@b.c",
		Types:        map[domain.NotificationType]bool{domain.NotificationNewUserType: true},
	}}
	r := NewNotifyingRepo(store, settings, sender)

	// Like type not enabled in the Types map -> no email.
	if err := r.Add(domain.Notification{Type: domain.NotificationLikeType, UserId: "owner-1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Give the goroutine a chance to (not) send.
	time.Sleep(100 * time.Millisecond)
	if sender.count() != 0 {
		t.Fatalf("expected no email, got %d", sender.count())
	}
}

func TestNotifyingRepo_Add_NoEmailWhenDisabled(t *testing.T) {
	store := &fakeStore{}
	sender := newFakeSender()
	settings := fakeSettings{cfg: domain.NotificationSettings{EmailEnabled: false, Recipient: "a@b.c"}}
	r := NewNotifyingRepo(store, settings, sender)

	if err := r.Add(domain.Notification{Type: domain.NotificationNewUserType, UserId: "owner-1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if sender.count() != 0 {
		t.Fatalf("expected no email when disabled, got %d", sender.count())
	}
}

func TestBuildMessage(t *testing.T) {
	msg := string(buildMessage("from@x.y", "to@x.y", "subj", "hello"))
	for _, want := range []string{"From: from@x.y", "To: to@x.y", "Subject: subj", "hello"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q:\n%s", want, msg)
		}
	}
}
