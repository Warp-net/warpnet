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
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"path"
	"strings"
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

// mediaTypePhoto is the extended_entities media type for a still photo;
// "animated_gif" and "video" are the types we deliberately skip.
const mediaTypePhoto = "photo"

// ImportTweetStorer is the slice of the tweet repo the importer needs:
// Get to skip already-imported tweets, Create to store a new one. Create
// writes straight to the repo (no follower broadcast), so a bulk import
// does not flood the network with historical tweets.
type ImportTweetStorer interface {
	Get(userID, tweetID string) (tweet domain.Tweet, err error)
	Create(_ string, tweet domain.Tweet) (domain.Tweet, error)
}

// archiveTweet is the subset of an X archive tweet record we read. The
// archive double-lists media: entities.media labels a GIF/video thumbnail
// as "photo", while extended_entities.media carries the real type
// ("animated_gif"/"video"). We therefore trust extended_entities.
type archiveTweet struct {
	IDStr                string          `json:"id_str"`
	FullText             string          `json:"full_text"`
	CreatedAt            string          `json:"created_at"`
	InReplyToStatusIDStr string          `json:"in_reply_to_status_id_str"`
	Entities             archiveEntities `json:"entities"`
	ExtendedEntities     archiveEntities `json:"extended_entities"`
}

type archiveEntities struct {
	Media []archiveMedia `json:"media"`
}

type archiveMedia struct {
	Type          string `json:"type"`
	MediaURLHTTPS string `json:"media_url_https"`
}

// isRetweet reports whether the tweet is the user's own retweet. X archives
// store these as a tweet whose text begins "RT @". Retweets are out of scope.
func (at archiveTweet) isRetweet() bool {
	return strings.HasPrefix(at.FullText, "RT @")
}

// isReply reports whether the tweet is a reply (to anyone, including the
// user themselves). Replies are out of scope for this import.
func (at archiveTweet) isReply() bool {
	return at.InReplyToStatusIDStr != ""
}

// photoMedia returns only the still-image attachments, dropping animated
// GIFs and videos. extended_entities is authoritative for the media type;
// entities is the fallback for the rare record without it.
func (at archiveTweet) photoMedia() []archiveMedia {
	media := at.ExtendedEntities.Media
	if len(media) == 0 {
		media = at.Entities.Media
	}
	photos := make([]archiveMedia, 0, len(media))
	for _, m := range media {
		if m.Type == mediaTypePhoto && m.MediaURLHTTPS != "" {
			photos = append(photos, m)
		}
	}
	return photos
}

// StreamImportTwitterArchiveHandler imports the user's own original tweets
// from an X (Twitter) data archive .zip sitting on local disk. It reads
// data/tweets.js, skips retweets and replies, stores attached photos
// (ignoring GIFs/videos) through the existing media pipeline, and writes
// each tweet straight to the tweet repo under the owner's id. The original
// X id is preserved so a re-run is idempotent. Likes, direct messages and
// profile data in the archive are ignored.
func StreamImportTwitterArchiveHandler(
	info MediaNodeInformer,
	tweetRepo ImportTweetStorer,
	mediaRepo MediaStorer,
	userRepo MediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.ImportTwitterArchiveEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.ArchivePath == "" {
			return nil, warpnet.WarpError("import: empty archive path")
		}

		zr, err := zip.OpenReader(ev.ArchivePath)
		if err != nil {
			return nil, fmt.Errorf("import: opening archive: %w", err)
		}
		defer func() { _ = zr.Close() }()

		tweetFiles, mediaByName := indexArchive(zr.File)
		if len(tweetFiles) == 0 {
			return nil, warpnet.WarpError("import: tweets.js not found in archive")
		}

		// The owner's ownership EXIF blob is built once and embedded into
		// every imported photo, exactly like a fresh upload.
		encryptedMeta, ownerUser, err := buildEncryptedImageMeta(info, userRepo)
		if err != nil {
			return nil, fmt.Errorf("import: %w", err)
		}

		var resp event.ImportTwitterArchiveResponse
		for _, tf := range tweetFiles {
			raw, err := readZipFile(tf)
			if err != nil {
				return nil, fmt.Errorf("import: reading %s: %w", tf.Name, err)
			}
			tweets, err := parseArchiveTweets(raw)
			if err != nil {
				return nil, fmt.Errorf("import: parsing %s: %w", tf.Name, err)
			}
			for _, at := range tweets {
				importOneTweet(at, mediaByName, encryptedMeta, ownerUser, tweetRepo, mediaRepo, &resp)
			}
		}

		log.Infof(
			"import: twitter archive done: imported=%d images=%d skipped=%d",
			resp.ImportedTweets, resp.ImportedImages, resp.SkippedTweets,
		)
		return resp, nil
	}
}

// importOneTweet applies the scope rules to a single archive tweet and,
// when in scope, stores its photos and the tweet itself, updating resp.
func importOneTweet(
	at archiveTweet,
	mediaByName map[string]*zip.File,
	encryptedMeta []byte,
	ownerUser domain.User,
	tweetRepo ImportTweetStorer,
	mediaRepo MediaStorer,
	resp *event.ImportTwitterArchiveResponse,
) {
	if at.IDStr == "" {
		return
	}
	// Retweets and replies are out of scope (so are likes/DMs/profile,
	// which live in other archive files we never read).
	if at.isRetweet() || at.isReply() {
		resp.SkippedTweets++
		return
	}
	// Idempotent re-import: a tweet already stored under the owner is left
	// untouched.
	if _, err := tweetRepo.Get(ownerUser.Id, at.IDStr); err == nil {
		resp.SkippedTweets++
		return
	}

	imageKeys, imgCount := importTweetPhotos(at, mediaByName, encryptedMeta, ownerUser.Id, mediaRepo)
	resp.ImportedImages += imgCount

	text := html.UnescapeString(at.FullText)
	if text == "" && len(imageKeys) == 0 {
		resp.SkippedTweets++
		return
	}

	tweet := domain.Tweet{
		Id:        at.IDStr,
		Text:      text,
		UserId:    ownerUser.Id,
		Username:  ownerUser.Username,
		CreatedAt: parseArchiveTime(at.CreatedAt),
		ImageKeys: imageKeys,
	}
	if _, err := tweetRepo.Create(ownerUser.Id, tweet); err != nil {
		log.Errorf("import: creating tweet %s: %v", at.IDStr, err)
		resp.SkippedTweets++
		return
	}
	resp.ImportedTweets++
}

// importTweetPhotos stores every bundled photo of a tweet and returns the
// resulting media keys. A photo whose file is not present in the archive
// (X did not bundle it) is silently skipped — the tweet is still imported.
func importTweetPhotos(
	at archiveTweet,
	mediaByName map[string]*zip.File,
	encryptedMeta []byte,
	userId string,
	mediaRepo MediaStorer,
) (keys []string, count int) {
	for _, m := range at.photoMedia() {
		filename := at.IDStr + "-" + path.Base(m.MediaURLHTTPS)
		f, ok := mediaByName[filename]
		if !ok {
			continue
		}
		raw, err := readZipFile(f)
		if err != nil {
			log.Warnf("import: reading media %s: %v", filename, err)
			continue
		}
		// processAndStoreImage wants a data-URL; the declared mime is
		// irrelevant (it decodes by sniffing the bytes).
		dataURL := imagePrefix + base64.StdEncoding.EncodeToString(raw)
		key, err := processAndStoreImage(dataURL, encryptedMeta, userId, mediaRepo)
		if err != nil {
			log.Warnf("import: storing media %s: %v", filename, err)
			continue
		}
		keys = append(keys, key)
		count++
		if len(keys) >= maxTweetImages {
			break
		}
	}
	return keys, count
}

// indexArchive splits the archive entries into the tweet payload file(s)
// and a base-filename → entry map of bundled tweet media.
func indexArchive(files []*zip.File) (tweetFiles []*zip.File, mediaByName map[string]*zip.File) {
	mediaByName = make(map[string]*zip.File)
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		switch {
		case isTweetsFile(f.Name):
			tweetFiles = append(tweetFiles, f)
		case strings.Contains(f.Name, "data/tweets_media/"):
			mediaByName[path.Base(f.Name)] = f
		}
	}
	return tweetFiles, mediaByName
}

// isTweetsFile matches the tweet payload file(s): tweets.js and, for large
// exports split across parts, tweets-part1.js, tweets-part2.js, … It must
// not match tweet-headers.js, deleted-tweets.js, note-tweet.js, etc.
func isTweetsFile(name string) bool {
	base := path.Base(name)
	if base == "tweets.js" {
		return true
	}
	return strings.HasPrefix(base, "tweets-part") && strings.HasSuffix(base, ".js")
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}

// parseArchiveTweets strips the "window.YTD.tweets.partN = " assignment that
// wraps the JSON array in tweets.js and decodes the tweet records.
func parseArchiveTweets(raw []byte) ([]archiveTweet, error) {
	arr, err := extractTweetsArrayJSON(raw)
	if err != nil {
		return nil, err
	}
	var wrapper []struct {
		Tweet archiveTweet `json:"tweet"`
	}
	if err := json.Unmarshal(arr, &wrapper); err != nil {
		return nil, fmt.Errorf("import: unmarshal tweets: %w", err)
	}
	tweets := make([]archiveTweet, 0, len(wrapper))
	for _, w := range wrapper {
		tweets = append(tweets, w.Tweet)
	}
	return tweets, nil
}

// extractTweetsArrayJSON returns the JSON array starting at the first '['.
// The file is a JS assignment whose right-hand side is a plain JSON array,
// so everything from the first bracket onward is valid JSON.
func extractTweetsArrayJSON(raw []byte) ([]byte, error) {
	idx := bytes.IndexByte(raw, '[')
	if idx < 0 {
		return nil, warpnet.WarpError("import: tweets payload has no JSON array")
	}
	return raw[idx:], nil
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
