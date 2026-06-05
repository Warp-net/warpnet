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

package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
)

const mediaRefSep = "\x1f"

// encodeMediaRef packs a Warpnet (userId, image key) into one URL-safe path
// segment so an AP attachment can carry a fetchable, reversible reference.
func encodeMediaRef(userID, key string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(userID + mediaRefSep + key))
}

func decodeMediaRef(ref string) (userID, key string, ok bool) {
	bt, err := base64.RawURLEncoding.DecodeString(ref)
	if err != nil {
		return "", "", false
	}
	userID, key, ok = strings.Cut(string(bt), mediaRefSep)
	return userID, key, ok
}

// handleMedia proxies a Warpnet image to the Fediverse: it fetches the bytes
// from the node (PUBLIC_GET_IMAGE) and serves them so Mastodon can attach them.
// The gateway stores nothing; the node remains the media store.
func (g *gateway) handleMedia(w http.ResponseWriter, r *http.Request) {
	if g.req == nil {
		http.Error(w, "no node", http.StatusServiceUnavailable)
		return
	}
	userID, key, ok := decodeMediaRef(strings.TrimPrefix(r.URL.Path, pathMedia))
	if !ok || key == "" {
		http.NotFound(w, r)
		return
	}

	bt, err := g.req.request(routeGetImage, getImageEvent{UserId: userID, Key: key})
	if err != nil {
		log.Errorf("media: fetch %s/%s: %v", userID, key, err)
		http.Error(w, "fetch failed", http.StatusBadGateway)
		return
	}

	var resp getImageResponse
	if jerr := json.Unmarshal(bt, &resp); jerr != nil || resp.File == "" {
		http.NotFound(w, r)
		return
	}
	// File is "<mime>,<base64>" (see domain image keys).
	mime, data, found := strings.Cut(resp.File, ",")
	if !found {
		http.NotFound(w, r)
		return
	}
	bytes, derr := base64.StdEncoding.DecodeString(data)
	if derr != nil {
		log.Errorf("media: decode %s/%s: %v", userID, key, derr)
		http.Error(w, "decode failed", http.StatusBadGateway)
		return
	}

	w.Header().Set(headerContentType, mime)
	_, _ = w.Write(bytes)
}
