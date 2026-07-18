//nolint:all
package mailer

import (
	"errors"
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/domain"
)

func TestSMTPMailer_SendValidatesInput(t *testing.T) {
	m := NewSMTPMailer()

	if err := m.Send(domain.NotificationSettings{Recipient: "a@b.c"}, "s", "b"); !errors.Is(err, ErrEmptySMTPHost) {
		t.Fatalf("expected ErrEmptySMTPHost, got %v", err)
	}
	if err := m.Send(domain.NotificationSettings{SMTPHost: "h"}, "s", "b"); !errors.Is(err, ErrEmptyRecipient) {
		t.Fatalf("expected ErrEmptyRecipient, got %v", err)
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
