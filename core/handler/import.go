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
	"fmt"
	"html"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

// maxTweetImages mirrors the 4-image limit the composer and the upload
// handler enforce; an X tweet never carries more than four photos either.
const maxTweetImages = 4

// archiveTimeLayout is the timestamp format X uses in tweets.js
// ("Fri May 29 20:52:08 +0000 2026").
const archiveTimeLayout = "Mon Jan 02 15:04:05 -0700 2006"

type ImportTweetStorer interface {
	Create(_ string, tweet domain.Tweet) (domain.Tweet, error)
}

// StreamImportTweetHandler stores one pre-parsed original tweet streamed from
// the client (business browser dashboard or desktop member node). The client
// unzips and filters the X archive itself — dropping retweets, replies, GIFs
// and videos — and streams only the kept tweets one by one, so the node never
// buffers the whole archive. Photos arrive as raw base64 and go through the
// same EXIF/ownership pipeline as a fresh upload.
func StreamImportTweetHandler(
	info MediaNodeInformer,
	tweetRepo ImportTweetStorer,
	mediaRepo MediaStorer,
	userRepo MediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.ImportTweetEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.Id == "" {
			return nil, warpnet.WarpError("import: empty tweet id")
		}

		encryptedMeta, ownerUser, err := buildEncryptedImageMeta(info, userRepo)
		if err != nil {
			return nil, fmt.Errorf("import: %w", err)
		}

		var resp event.ImportTwitterArchiveResponse
		imageKeys := make([]string, 0, len(ev.Images))
		for i, img := range ev.Images {
			if i >= maxTweetImages {
				break
			}
			key, err := processAndStoreImage(imagePrefix+img, encryptedMeta, ownerUser.Id, mediaRepo)
			if err != nil {
				log.Warnf("import: storing photo for tweet %s: %v", ev.Id, err)
				continue
			}
			imageKeys = append(imageKeys, key)
			resp.ImportedImages++
		}

		text := html.UnescapeString(ev.Text)
		if text == "" && len(imageKeys) == 0 {
			resp.SkippedTweets++
			return resp, nil
		}

		if _, err := tweetRepo.Create(ownerUser.Id, domain.Tweet{
			Id:        ev.Id,
			Text:      text,
			UserId:    ownerUser.Id,
			Username:  ownerUser.Username,
			CreatedAt: parseArchiveTime(ev.CreatedAt),
			ImageKeys: imageKeys,
		}); err != nil {
			log.Errorf("import: creating tweet %s: %v", ev.Id, err)
			return nil, err
		}
		resp.ImportedTweets++
		return resp, nil
	}
}

// parseArchiveTime parses X's tweet timestamp, returning the zero time on
// failure (the repo then stamps the current time, preserving the import
// rather than dropping the tweet).
func parseArchiveTime(s string) time.Time {
	t, err := time.Parse(archiveTimeLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
