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

package local

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	FixedKey      = "fixed"
	FixedRangeKey = "fixed"
	NoneRangeKey  = "none"
	Delimeter     = "/"
)

type (
	Namespace   string
	ParentLayer string
	RangeLayer  string
	IdLayer     string

	DatabaseKey string
)

// PrefixBuilder is a struct that holds a key and any potential error
type PrefixBuilder struct {
	namespace Namespace
}

// NewPrefixBuilder creates a new PrefixBuilder instance
func NewPrefixBuilder(mandatoryNamespace string) Namespace {
	if !strings.HasPrefix(mandatoryNamespace, "/") {
		panic("namespace must start with /")
	}
	if mandatoryNamespace == "" {
		panic("namespace must not be empty")
	}
	return Namespace(mandatoryNamespace)
}

func (ns Namespace) AddSubPrefix(p string) Namespace {
	if p == "" {
		panic("sub prefix must not be empty")
	}
	return Namespace(fmt.Sprintf("%s%s%s", ns, Delimeter, p))
}

func (ns Namespace) Build() DatabaseKey {
	return build(string(ns))
}

func (ns Namespace) AddRootID(mandatoryPrefix string) ParentLayer {
	if mandatoryPrefix == "" {
		panic("root prefix must not be empty")
	}
	key := fmt.Sprintf("%s%s%s", ns, Delimeter, mandatoryPrefix)
	return ParentLayer(key)
}

func (l ParentLayer) Build() DatabaseKey {
	return build(string(l))
}

type RangePrefix string

func (l ParentLayer) AddRange(mandatoryPrefix RangePrefix) RangeLayer {
	if mandatoryPrefix == "" {
		panic("range prefix must not be empty")
	}
	if mandatoryPrefix != FixedRangeKey && mandatoryPrefix != NoneRangeKey {
		_, err := strconv.ParseInt(string(mandatoryPrefix), 10, 64)
		if err != nil {
			panic(fmt.Sprintf("invalid range prefix %s", mandatoryPrefix))
		}
	}

	key := fmt.Sprintf("%s%s%s", l, Delimeter, mandatoryPrefix)
	return RangeLayer(key)
}

func (l ParentLayer) AddParentId(mandatoryPrefix string) IdLayer {
	if mandatoryPrefix == "" {
		panic("id prefix must not be empty")
	}
	key := fmt.Sprintf("%s%s%s", l, Delimeter, mandatoryPrefix)
	return IdLayer(key)
}

func (l ParentLayer) AddReversedTimestamp(tm time.Time) RangeLayer {
	key := string(l)
	key = fmt.Sprintf("%s%s%019d", key, Delimeter, math.MaxInt64-tm.Unix())
	return RangeLayer(key)
}
func (l RangeLayer) Build() DatabaseKey {
	return build(string(l))
}

func (l RangeLayer) AddParentId(mandatoryPrefix string) IdLayer {
	if mandatoryPrefix == "" {
		panic("id prefix must not be empty")
	}
	key := fmt.Sprintf("%s%s%s", l, Delimeter, mandatoryPrefix)
	return IdLayer(key)
}

func (l IdLayer) AddId(mandatoryPrefix string) IdLayer {
	if mandatoryPrefix == "" {
		panic("id prefix must not be empty")
	}
	key := fmt.Sprintf("%s%s%s", l, Delimeter, mandatoryPrefix)
	return IdLayer(key)
}
func (l IdLayer) AddReversedTimestamp(tm time.Time) RangeLayer {
	key := string(l)
	key = fmt.Sprintf("%s%s%019d", key, Delimeter, math.MaxInt64-tm.Unix())
	return RangeLayer(key)
}

func (l IdLayer) Build() DatabaseKey {
	return build(string(l))
}

func build(s string) DatabaseKey {
	return DatabaseKey(s)
}

func (k DatabaseKey) IsEmpty() bool {
	return string(k) == ""
}
func (k DatabaseKey) String() string {
	return string(k)
}
func (k DatabaseKey) Bytes() []byte {
	return []byte(k)
}
func (k DatabaseKey) DropId() string {
	key := string(k)
	lastColon := strings.LastIndex(key, Delimeter)
	if lastColon == -1 {
		// Если двоеточия нет, возвращаем оригинальную строку
		return key
	}
	return key[:lastColon]
}

// AddFollowedId adds a following ID segment to the key (reuses user ID validation)
func (pb *PrefixBuilder) AddWriterId(writerId string) *PrefixBuilder {
	// Skip processing if there's already an error
	//if pb.err != nil {
	//	return pb
	//}
	//
	//pb.key = fmt.Sprintf("%s/writer/%s", pb.key, writerId)
	return pb
}

func (pb *PrefixBuilder) AddReaderId(readerId string) *PrefixBuilder {
	// Skip processing if there's already an error
	//if pb.err != nil {
	//	return pb
	//}
	//
	//pb.key = fmt.Sprintf("%s/reader/%s", pb.key, readerId)
	return pb
}
