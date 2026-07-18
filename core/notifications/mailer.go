/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU Affero General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU Affero General Public License for more details.

 You should have received a copy of the GNU Affero General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package notifications

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/domain"
)

var (
	ErrEmptySMTPHost  = errors.New("mailer: empty smtp host")
	ErrEmptyRecipient = errors.New("mailer: empty recipient")
)

// fromAddress is the fixed visible sender of every Warpnet email.
const fromAddress = "noreply@warpnet.site"

// SMTPMailer sends email over the user's own SMTP server. It is the default
// Sender used by EmailChannel.
type SMTPMailer struct{}

func NewSMTPMailer() *SMTPMailer {
	return &SMTPMailer{}
}

func (m *SMTPMailer) Send(cfg domain.NotificationSettings, subject, body string) error {
	if cfg.SMTPHost == "" {
		return ErrEmptySMTPHost
	}
	if cfg.Recipient == "" {
		return ErrEmptyRecipient
	}
	port := cfg.SMTPPort
	if port == 0 {
		port = 587
	}
	// Envelope sender is the authenticated account — providers reject a
	// mismatched MAIL FROM. The visible From header is always fromAddress.
	envelopeFrom := cfg.SMTPUsername
	if envelopeFrom == "" {
		envelopeFrom = fromAddress
	}
	addr := net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(port))

	var auth smtp.Auth
	if cfg.SMTPUsername != "" {
		auth = smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)
	}
	msg := buildMessage(fromAddress, cfg.Recipient, subject, body)

	// Implicit TLS (typically port 465) needs a TLS connection up front.
	// Otherwise smtp.SendMail negotiates STARTTLS opportunistically
	// (typically port 587 / 25).
	if cfg.SMTPUseTLS {
		return sendImplicitTLS(addr, cfg.SMTPHost, auth, envelopeFrom, cfg.Recipient, msg)
	}
	return smtp.SendMail(addr, auth, envelopeFrom, []string{cfg.Recipient}, msg)
}

func sendImplicitTLS(addr, host string, auth smtp.Auth, from, to string, msg []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	d := tls.Dialer{Config: &tls.Config{ServerName: host}} //nolint:gosec
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	return []byte(b.String())
}
