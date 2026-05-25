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

WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package isolation

import (
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	log "github.com/sirupsen/logrus"
)

// Publisher is the slice of moderator pubsub the isolation protocol
// needs: publish a verdict onto the offender's followers topic so every
// observer re-renders the object with the moderation flag set. The
// implementation marshals `body` once on the way out — callers MUST
// pass a struct (or any non-[]byte value) so the result is a real JSON
// object on the wire, not a base64-encoded blob.
type Publisher interface {
	PublishUpdateToFollowers(ownerId, dest string, body any) (err error)
}

// IsolationProtocol implements shadow-ban semantics. The offender's own
// node never receives the verdict — it is published only on the
// followers/observers pubsub topic. The offender therefore cannot detect
// or resist isolation; their local view stays unchanged while everyone
// else hides the offending object.
type IsolationProtocol struct {
	pub Publisher
}

func NewIsolationProtocol(pub Publisher) *IsolationProtocol {
	return &IsolationProtocol{pub: pub}
}

// IsolateTweet broadcasts a tweet moderation verdict on the offender's
// followers topic. Subscribers apply the verdict via
// StreamModerationResultHandler and re-render with the moderation flag.
func (ip *IsolationProtocol) IsolateTweet(t *domain.Tweet, m *domain.TweetModeration) {
	if t == nil || m == nil {
		return
	}

	result := event.ModerationResultEvent{
		Type:     domain.ModerationTweetType,
		UserID:   t.UserId,
		ObjectID: &t.Id,
		Reason:   m.Reason,
		Model:    m.Model,
		Result:   m.IsOk,
	}

	if err := ip.pub.PublishUpdateToFollowers(
		t.UserId,
		event.PUBLIC_POST_MODERATION_RESULT,
		result,
	); err != nil {
		log.Errorf("isolation: publish tweet verdict: %v", err)
	}
}

// IsolateUser broadcasts a profile-level moderation verdict on the
// offender's followers topic. Followers cache the user with
// Moderation.IsModerated = true and clients hide bio / name / website /
// custom fields on the next render. The user row stays on disk;
// nothing is wiped.
func (ip *IsolationProtocol) IsolateUser(u *domain.User, m *domain.UserModeration) {
	if u == nil || m == nil {
		return
	}
	result := event.ModerationResultEvent{
		Type:   domain.ModerationUserType,
		UserID: u.Id,
		Reason: m.Reason,
		Model:  m.Model,
		Result: domain.ModerationResult(m.IsOk),
	}

	if err := ip.pub.PublishUpdateToFollowers(
		u.Id,
		event.PUBLIC_POST_MODERATION_RESULT,
		result,
	); err != nil {
		log.Errorf("isolation: publish user verdict: %v", err)
	}
}
