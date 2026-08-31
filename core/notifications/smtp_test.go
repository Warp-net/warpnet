//nolint:all
package notifications

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/require"
)

// fakeSMTP speaks just enough SMTP for net/smtp to complete a plain,
// unauthenticated delivery on loopback.
type fakeSMTP struct {
	host string
	port int

	mx       sync.Mutex
	received []string
	listener net.Listener
}

func newFakeSMTP(t *testing.T, failAt string) *fakeSMTP {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	s := &fakeSMTP{host: host, port: port, listener: ln}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn, failAt)
		}
	}()
	return s
}

func (s *fakeSMTP) serve(conn net.Conn, failAt string) {
	defer func() { _ = conn.Close() }()

	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)
	say := func(line string) bool {
		if _, err := w.WriteString(line + "\r\n"); err != nil {
			return false
		}
		return w.Flush() == nil
	}

	if !say("220 fake ESMTP") {
		return
	}
	inData := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		s.mx.Lock()
		s.received = append(s.received, strings.TrimSpace(line))
		s.mx.Unlock()

		if inData {
			if strings.TrimSpace(line) == "." {
				inData = false
				if !say("250 OK") {
					return
				}
			}
			continue
		}

		verb := strings.ToUpper(strings.Fields(strings.TrimSpace(line) + " ")[0])
		if verb == failAt {
			_ = say("550 rejected")
			continue
		}
		switch verb {
		case "EHLO", "HELO":
			// no extensions advertised: no STARTTLS, no AUTH
			if !say("250 fake") {
				return
			}
		case "MAIL", "RCPT", "NOOP", "RSET":
			if !say("250 OK") {
				return
			}
		case "DATA":
			inData = true
			if !say("354 send it") {
				return
			}
		case "QUIT":
			_ = say("221 bye")
			return
		default:
			if !say("250 OK") {
				return
			}
		}
	}
}

func (s *fakeSMTP) lines() []string {
	s.mx.Lock()
	defer s.mx.Unlock()
	out := make([]string, len(s.received))
	copy(out, s.received)
	return out
}

func TestSMTPMailerValidation(t *testing.T) {
	m := NewSMTPMailer()

	require.ErrorIs(t, m.Send(domain.NotificationSettings{Recipient: "a@b.c"}, "s", "b"), ErrEmptySMTPHost)
	require.ErrorIs(t, m.Send(domain.NotificationSettings{SMTPHost: "localhost"}, "s", "b"), ErrEmptyRecipient)
}

func TestSMTPMailerDelivers(t *testing.T) {
	srv := newFakeSMTP(t, "")

	err := NewSMTPMailer().Send(domain.NotificationSettings{
		SMTPHost:  srv.host,
		SMTPPort:  srv.port,
		Recipient: "recipient@example.org",
	}, "Warpnet: reply", "someone replied to you")
	require.NoError(t, err)

	conversation := strings.Join(srv.lines(), "\n")
	require.Contains(t, conversation, "MAIL FROM:")
	require.Contains(t, conversation, "RCPT TO:<recipient@example.org>")
	require.Contains(t, conversation, "Subject: Warpnet: reply")
	require.Contains(t, conversation, "someone replied to you")
}

func TestSMTPMailerUsesTheAuthenticatedAccountAsEnvelopeSender(t *testing.T) {
	srv := newFakeSMTP(t, "")

	require.NoError(t, NewSMTPMailer().Send(domain.NotificationSettings{
		SMTPHost:     srv.host,
		SMTPPort:     srv.port,
		SMTPUsername: "sender@example.org",
		SMTPPassword: "secret",
		Recipient:    "recipient@example.org",
	}, "s", "b"))

	require.Contains(t, strings.Join(srv.lines(), "\n"), "MAIL FROM:<sender@example.org>")
}

func TestSMTPMailerReportsServerRejections(t *testing.T) {
	for _, verb := range []string{"MAIL", "RCPT", "DATA"} {
		t.Run(verb, func(t *testing.T) {
			srv := newFakeSMTP(t, verb)
			err := NewSMTPMailer().Send(domain.NotificationSettings{
				SMTPHost:  srv.host,
				SMTPPort:  srv.port,
				Recipient: "recipient@example.org",
			}, "s", "b")
			require.Error(t, err)
		})
	}
}

func TestSMTPMailerReportsDialFailure(t *testing.T) {
	// port 1 on loopback refuses connections
	err := NewSMTPMailer().Send(domain.NotificationSettings{
		SMTPHost:  "127.0.0.1",
		SMTPPort:  1,
		Recipient: "recipient@example.org",
	}, "s", "b")
	require.Error(t, err)
}

func TestSMTPMailerReportsTLSDialFailure(t *testing.T) {
	srv := newFakeSMTP(t, "")
	// the fake server speaks plain SMTP, so an implicit-TLS dial cannot complete
	err := NewSMTPMailer().Send(domain.NotificationSettings{
		SMTPHost:   srv.host,
		SMTPPort:   srv.port,
		SMTPUseTLS: true,
		Recipient:  "recipient@example.org",
	}, "s", "b")
	require.Error(t, err)
}
