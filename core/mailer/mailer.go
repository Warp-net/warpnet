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

package mailer

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	"github.com/Warp-net/warpnet/domain"
	log "github.com/sirupsen/logrus"
)

// NotificationStore is the full notification-repo surface the decorator
// passes through. Add is overridden by NotifyingRepo; every other method
// is inherited unchanged so the wrapper is a drop-in replacement for the
// concrete repo in both reader and writer handlers.
type NotificationStore interface {
	Add(not domain.Notification) error
	MarkRead(userId, notificationId string) error
	MarkAllRead(userId string) error
	Get(userId, notificationId string) (domain.Notification, error)
	List(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error)
	ReverseList(userId string, cursor *string, limit *uint64) ([]domain.Notification, string, error)
	UnreadCount(userId string) (uint64, error)
}

// SettingsProvider resolves a user's notification settings.
type SettingsProvider interface {
	GetNotificationSettings(userId string) (domain.NotificationSettings, error)
}

// Sender delivers a single email using the caller-supplied SMTP settings.
type Sender interface {
	Send(cfg domain.NotificationSettings, subject, body string) error
}

// NotifyingRepo decorates a NotificationStore so that every stored
// notification is also, best-effort and asynchronously, mirrored to the
// owner's email channel when their settings opt in for that type.
type NotifyingRepo struct {
	NotificationStore
	settings    SettingsProvider
	sender      Sender
	ownerUserId string
}

func NewNotifyingRepo(
	store NotificationStore,
	settings SettingsProvider,
	sender Sender,
	ownerUserId string,
) *NotifyingRepo {
	return &NotifyingRepo{
		NotificationStore: store,
		settings:          settings,
		sender:            sender,
		ownerUserId:       ownerUserId,
	}
}

// Add persists the notification and, on success, fires an email in the
// background. Email delivery never blocks the caller and its failures are
// logged, not returned — mirroring the best-effort contract the in-app
// notification path already follows.
func (r *NotifyingRepo) Add(not domain.Notification) error {
	if err := r.NotificationStore.Add(not); err != nil {
		return err
	}
	go r.dispatchEmail(not)
	return nil
}

func (r *NotifyingRepo) dispatchEmail(not domain.Notification) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Errorf("mailer: dispatch panic: %v", rec)
		}
	}()
	if r.settings == nil || r.sender == nil {
		return
	}

	userId := not.UserId
	if userId == "" {
		userId = r.ownerUserId
	}

	cfg, err := r.settings.GetNotificationSettings(userId)
	if err != nil {
		log.Warnf("mailer: load settings: %v", err)
		return
	}
	if !cfg.EmailEnabled || cfg.Recipient == "" {
		return
	}
	if !cfg.Types[not.Type] {
		return
	}

	subject := "Warpnet: " + not.Type.String()
	if err := r.sender.Send(cfg, subject, not.Text); err != nil {
		log.Warnf("mailer: send email: %v", err)
	}
}

// SMTPMailer sends email over the user's own SMTP server.
type SMTPMailer struct{}

func NewSMTPMailer() *SMTPMailer {
	return &SMTPMailer{}
}

func (m *SMTPMailer) Send(cfg domain.NotificationSettings, subject, body string) error {
	if cfg.SMTPHost == "" {
		return fmt.Errorf("mailer: empty smtp host")
	}
	if cfg.Recipient == "" {
		return fmt.Errorf("mailer: empty recipient")
	}
	port := cfg.SMTPPort
	if port == 0 {
		port = 587
	}
	from := cfg.SMTPFrom
	if from == "" {
		from = cfg.SMTPUsername
	}
	addr := net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(port))

	var auth smtp.Auth
	if cfg.SMTPUsername != "" {
		auth = smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)
	}
	msg := buildMessage(from, cfg.Recipient, subject, body)

	// Implicit TLS (typically port 465) needs a TLS connection up front.
	// Otherwise smtp.SendMail negotiates STARTTLS opportunistically
	// (typically port 587 / 25).
	if cfg.SMTPUseTLS {
		return sendImplicitTLS(addr, cfg.SMTPHost, auth, from, cfg.Recipient, msg)
	}
	return smtp.SendMail(addr, auth, from, []string{cfg.Recipient}, msg)
}

func sendImplicitTLS(addr, host string, auth smtp.Auth, from, to string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host}) //nolint:gosec
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
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
