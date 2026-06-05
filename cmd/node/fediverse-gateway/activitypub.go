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

// Minimal ActivityPub / WebFinger document shapes for the Phase-1 skeleton.
// Inbound activities are parsed loosely into map[string]any (see inbox.go);
// these typed shapes are for the documents we emit.

// ActivityPub activity/object type names the gateway emits or matches.
const (
	typeCreate   = "Create"
	typeLike     = "Like"
	typeFollow   = "Follow"
	typeUndo     = "Undo"
	typeNote     = "Note"
	typeAnnounce = "Announce"
	typeDelete   = "Delete"
	typeDocument = "Document"
)

type webFingerJRD struct {
	Subject string          `json:"subject"`
	Links   []webFingerLink `json:"links"`
}

type webFingerLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`
}

type publicKey struct {
	ID           string `json:"id"`
	Owner        string `json:"owner"`
	PublicKeyPEM string `json:"publicKeyPem"`
}

type actorEndpoints struct {
	SharedInbox string `json:"sharedInbox"`
}

type actor struct {
	Context           any             `json:"@context"`
	ID                string          `json:"id"`
	Type              string          `json:"type"`
	PreferredUsername string          `json:"preferredUsername"`
	Name              string          `json:"name,omitempty"`
	Summary           string          `json:"summary,omitempty"`
	Inbox             string          `json:"inbox"`
	Outbox            string          `json:"outbox"`
	Followers         string          `json:"followers"`
	Following         string          `json:"following"`
	PublicKey         publicKey       `json:"publicKey"`
	Endpoints         *actorEndpoints `json:"endpoints,omitempty"`
}

// activity is the envelope we emit for outbound activities (e.g. Accept).
// Object is left as any so the original inbound activity can be echoed back
// verbatim, which is what Accept(Follow) requires.
type activity struct {
	Context any      `json:"@context,omitempty"`
	ID      string   `json:"id,omitempty"`
	Type    string   `json:"type"`
	Actor   string   `json:"actor,omitempty"`
	Object  any      `json:"object,omitempty"`
	To      []string `json:"to,omitempty"`
	Cc      []string `json:"cc,omitempty"`
}

// note is the ActivityPub Note emitted inside a Create when a Warpnet tweet is
// federated outbound, and served standalone at /users/{user}/statuses/{id}
// (Context is set only when served standalone).
type note struct {
	Context      any          `json:"@context,omitempty"`
	ID           string       `json:"id"`
	Type         string       `json:"type"`
	AttributedTo string       `json:"attributedTo"`
	Content      string       `json:"content"`
	Published    string       `json:"published"`
	InReplyTo    string       `json:"inReplyTo,omitempty"`
	To           []string     `json:"to,omitempty"`
	Cc           []string     `json:"cc,omitempty"`
	Attachment   []attachment `json:"attachment,omitempty"`
}

// attachment is an ActivityPub media attachment (image) on a Note. mediaType is
// omitted — the gateway's /media endpoint sets the Content-Type and Mastodon
// reads it from there.
type attachment struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type orderedCollection struct {
	Context      any    `json:"@context"`
	ID           string `json:"id"`
	Type         string `json:"type"`
	TotalItems   int    `json:"totalItems"`
	OrderedItems []any  `json:"orderedItems"`
}

type nodeInfoLinks struct {
	Links []nodeInfoLink `json:"links"`
}

type nodeInfoLink struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}
