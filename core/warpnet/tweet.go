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

package warpnet

import (
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/Warp-net/warpnet/domain"
)

const (
	TweetCharLimit      = 280
	SponsoredPriceLimit = 1_000_000_000_000
	PollMinOptions      = 2
	PollMaxOptions      = 4
	PollOptionRuneLimit = 25
)

func ValidateTweet(tweet domain.Tweet) error {
	if tweet.UserId == "" {
		return WarpError("empty user id")
	}
	if tweet.Text == "" && !tweet.IsSponsored() {
		return WarpError("empty tweet text")
	}
	if utf8.RuneCountInString(tweet.Text) > TweetCharLimit {
		return WarpError("tweet text is too long")
	}
	if tweet.IsSponsored() {
		if !tweet.Price.IsPositive() {
			return WarpError("sponsored tweet: price must be positive")
		}
		if tweet.Price.Units.Cmp(big.NewInt(SponsoredPriceLimit)) > 0 {
			return WarpError("sponsored tweet: price is above 1000000 USDT")
		}
		if tweet.Poll != nil {
			return WarpError("sponsored tweet: poll is not allowed")
		}
	}
	return validatePoll(tweet.Poll)
}

func validatePoll(p *domain.Poll) error {
	if p == nil {
		return nil
	}
	if len(p.Options) < PollMinOptions {
		return WarpError("poll: too few options")
	}
	if len(p.Options) > PollMaxOptions {
		return WarpError("poll: too many options")
	}
	if p.ExpiresAt.IsZero() {
		return WarpError("poll: empty expiration time")
	}
	for _, opt := range p.Options {
		if strings.TrimSpace(opt) == "" {
			return WarpError("poll: empty option")
		}
		if utf8.RuneCountInString(opt) > PollOptionRuneLimit {
			return WarpError("poll: option is too long")
		}
	}
	return nil
}
