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

package handler

import (
	"errors"
	"fmt"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	ErrNoObjectID warpnet.WarpError = "no object id found"
	ErrNoUserID   warpnet.WarpError = "no user id found"
)

type ModerationNotifier interface {
	Add(not domain.Notification) error
}

type ModerationTweetUpdater interface {
	Update(tweet domain.Tweet) error
}

type ModerationUserUpdater interface {
	Get(userId string) (domain.User, error)
	Update(userId string, user domain.User) (domain.User, error)
}

type ModerationTimelelineDeleter interface {
	DeleteTweetFromTimeline(userID, tweetID string) error
}

// StreamModerationResultHandler receives a verdict from a moderator and
// applies it locally so this node's view of the offending object is
// downgraded. Three design notes:
//
//   - Isolation is shadow-style: the offender's own node never receives
//     this stream (the moderator only publishes the verdict to the
//     followers/observers pubsub topic, see IsolationProtocol). The
//     offender must never see a moderation notification.
//
//   - ModerationUserType marks the user-level moderation flag so clients
//     hide bio/displayName/url/website on the next render. The user row
//     stays on disk, only the Moderation sidecar is set.
//
//   - The reporter — and only the reporter — is told the outcome. The
//     moderator re-sends the verdict straight to the reporter's node with
//     ReporterID set; the followers/observers broadcast leaves it empty and
//     so never notifies. The notification is raised before the isolation
//     switch because that switch early-returns for users not cached locally,
//     and the reporter need not follow the offender.
func StreamModerationResultHandler(
	notifier ModerationNotifier,
	tweetRepo ModerationTweetUpdater,
	userRepo ModerationUserUpdater,
	timelineRepo ModerationTimelelineDeleter,
	authRepo NotifierAuthStorer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.ModerationResultEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}

		// Verdicts now travel via pubsub → SelfStream, so the stream
		// connection's RemotePeer is the local node, not the moderator.
		// Attribution must come from the payload itself.
		moderatorId := ev.ModeratorID

		log.Infof("moderation: result type=%s user=%s result=%t reporter=%s",
			ev.Type.String(), ev.UserID, bool(ev.Result), ev.ReporterID)

		notifyReporter(notifier, authRepo, ev)

		switch ev.Type {
		case domain.ModerationTweetType:
			if ev.ObjectID == nil {
				return nil, ErrNoObjectID
			}
			if ev.UserID == "" {
				return nil, ErrNoUserID
			}

			tweet := domain.Tweet{
				Id:     *ev.ObjectID,
				UserId: ev.UserID,
				Moderation: &domain.TweetModeration{
					ModeratorID: moderatorId,
					Model:       ev.Model,
					IsOk:        ev.Result,
					Reason:      ev.Reason,
					TimeAt:      time.Now(),
				},
			}

			if err := tweetRepo.Update(tweet); err != nil {
				log.Errorf("moderation: failed to update tweet: %v", err)
			}
			if err := timelineRepo.DeleteTweetFromTimeline(ev.UserID, *ev.ObjectID); err != nil {
				log.Errorf("moderation: failed to delete timeline: %v", err)
			}

		case domain.ModerationUserType:
			if ev.UserID == "" {
				return nil, ErrNoUserID
			}
			if userRepo == nil {
				log.Warn("moderation: no user repo wired")
				return event.Accepted, nil
			}

			user, err := userRepo.Get(ev.UserID)
			if errors.Is(err, database.ErrUserNotFound) {
				// Nothing local to mark; observers without the user
				// cached can drop the verdict silently.
				return event.Accepted, nil
			}
			if err != nil {
				log.Errorf("moderation: failed to fetch user: %v", err)
				return event.Accepted, nil
			}
			user.Moderation = &domain.UserModeration{
				IsModerated: true,
				Model:       ev.Model,
				IsOk:        bool(ev.Result),
				Reason:      ev.Reason,
				TimeAt:      time.Now(),
			}
			if _, err := userRepo.Update(ev.UserID, user); err != nil {
				log.Errorf("moderation: failed to update user: %v", err)
			}

		default:
			log.Errorf("moderation handler: unknown event type %s", ev.Type.String())
			return event.Accepted, nil
		}

		// Offenders and plain observers are never notified (shadow
		// isolation); they simply re-render with the moderation flag set.
		// Only the reporter is notified, handled above by notifyReporter.
		return event.Accepted, nil
	}
}

// notifyReporter raises a notification for the user who filed the report,
// and only them. The moderator stamps ReporterID when it delivers the
// verdict directly to the reporter's node; the followers/observers
// broadcast leaves it empty, so this is a no-op there.
func notifyReporter(notifier ModerationNotifier, authRepo NotifierAuthStorer, ev event.ModerationResultEvent) {
	if notifier == nil || authRepo == nil || ev.ReporterID == "" {
		return
	}
	owner := authRepo.GetOwner()
	if owner.UserId == "" || owner.UserId != ev.ReporterID {
		return
	}
	// Best-effort, matching the like/follow handlers: a local storage
	// failure is logged but doesn't fail the moderator's delivery.
	if err := notifier.Add(domain.Notification{
		Type:   domain.NotificationModerationType,
		Text:   reportResultText(ev),
		UserId: owner.UserId,
	}); err != nil {
		log.Errorf("moderation: notify reporter: %v", err)
	}
}

// reportResultText is the line shown in the reporter's notifications. The
// moderator only delivers actioned (FAIL) verdicts, so this describes a
// moderated object; the engine's reason (a Llama Guard hazard label) is
// appended when present.
func reportResultText(ev event.ModerationResultEvent) string {
	subject := "content"
	switch ev.Type {
	case domain.ModerationTweetType:
		subject = "tweet"
	case domain.ModerationUserType:
		subject = "profile"
	}
	if ev.Reason != nil && *ev.Reason != "" {
		return fmt.Sprintf("The %s you reported was moderated: %s", subject, *ev.Reason)
	}
	return fmt.Sprintf("The %s you reported was moderated", subject)
}
