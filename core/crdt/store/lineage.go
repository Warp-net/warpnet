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

package store

import (
	"context"
	"crypto/sha256"
	"slices"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	dag "github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

const (
	lineageCapacity = 1 << 16
	lineageTTL      = time.Hour
)

// lineage follows every block the store fetches back to the heads it was
// announced under. go-ds-crdt hands the delta decoder a block's bytes only,
// so this is how a block is checked against its root's DAG name: a block
// filed under one day's DAG and announced under another is refused.
type lineage struct {
	ipld.DAGService

	byCID  *expirable.LRU[cid.Cid, []string]
	byData *expirable.LRU[[sha256.Size]byte, []string]
}

func newLineage(dags ipld.DAGService) *lineage {
	return &lineage{
		DAGService: dags,
		byCID:      expirable.NewLRU[cid.Cid, []string](lineageCapacity, nil, lineageTTL),
		byData:     expirable.NewLRU[[sha256.Size]byte, []string](lineageCapacity, nil, lineageTTL),
	}
}

// announce records that c heads the DAG dagName.
func (l *lineage) announce(c cid.Cid, dagName string) {
	names, _ := l.byCID.Get(c)
	if !slices.Contains(names, dagName) {
		l.byCID.Add(c, append(slices.Clone(names), dagName))
	}
}

// fetched passes a block's roots on to its data and to the blocks it links.
func (l *lineage) fetched(nd ipld.Node) {
	names, ok := l.byCID.Get(nd.Cid())
	if !ok {
		return
	}
	proto, ok := nd.(*dag.ProtoNode)
	if !ok {
		return
	}
	l.byData.Add(sha256.Sum256(proto.Data()), names)
	for _, link := range nd.Links() {
		for _, name := range names {
			l.announce(link.Cid, name)
		}
	}
}

// isRootedIn reports whether a block with these delta bytes may claim the
// DAG dagName: it was reached from a head of that DAG, or its roots are no
// longer known.
func (l *lineage) isRootedIn(data []byte, dagName string) bool {
	names, ok := l.byData.Get(sha256.Sum256(data))
	return !ok || slices.Contains(names, dagName)
}

func (l *lineage) Get(ctx context.Context, c cid.Cid) (ipld.Node, error) {
	nd, err := l.DAGService.Get(ctx, c)
	if err == nil {
		l.fetched(nd)
	}
	return nd, err
}

func (l *lineage) GetMany(ctx context.Context, cids []cid.Cid) <-chan *ipld.NodeOption {
	return l.follow(l.DAGService.GetMany(ctx, cids))
}

// Session keeps the inner DAG service's fetch sessions while still
// following what they fetch.
func (l *lineage) Session(ctx context.Context) ipld.NodeGetter {
	return &lineageSession{NodeGetter: dag.NewSession(ctx, l.DAGService), lineage: l}
}

func (l *lineage) follow(in <-chan *ipld.NodeOption) <-chan *ipld.NodeOption {
	out := make(chan *ipld.NodeOption, cap(in))
	go func() {
		defer close(out)
		for opt := range in {
			if opt.Err == nil {
				l.fetched(opt.Node)
			}
			out <- opt
		}
	}()
	return out
}

type lineageSession struct {
	ipld.NodeGetter

	lineage *lineage
}

func (s *lineageSession) Get(ctx context.Context, c cid.Cid) (ipld.Node, error) {
	nd, err := s.NodeGetter.Get(ctx, c)
	if err == nil {
		s.lineage.fetched(nd)
	}
	return nd, err
}

func (s *lineageSession) GetMany(ctx context.Context, cids []cid.Cid) <-chan *ipld.NodeOption {
	return s.lineage.follow(s.NodeGetter.GetMany(ctx, cids))
}
