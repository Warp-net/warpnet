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

package db

import (
	"context"
	"errors"

	"github.com/dgraph-io/badger/v4"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	format "github.com/ipfs/go-ipld-format"
)

type dagStore struct {
	ds *LocalDatastore
}

func (d *dagStore) Add(ctx context.Context, node format.Node) error {
	if !d.ds.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	id := node.Cid().String()
	data := node.RawData()
	return d.ds.Put(ctx, NewKey(id), data)
}
func (d *dagStore) Get(ctx context.Context, cid cid.Cid) (format.Node, error) {
	if !d.ds.isRunning.Load() {
		return nil, ErrNotRunning
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	data, err := d.ds.Get(ctx, NewKey(cid.String()))
	if errors.Is(err, badger.ErrKeyNotFound) || errors.Is(err, ds.ErrNotFound) {
		return nil, format.ErrNotFound{Cid: cid}
	}
	if err != nil {
		return nil, err
	}

	return merkledag.NewRawNode(data), nil
}

func (d *dagStore) GetMany(ctx context.Context, cids []cid.Cid) <-chan *format.NodeOption {
	if !d.ds.isRunning.Load() {
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}

	out := make(chan *format.NodeOption, len(cids))
	go func() {
		defer close(out)
		for _, c := range cids {
			data, err := d.ds.Get(ctx, NewKey(c.String()))
			if errors.Is(err, badger.ErrKeyNotFound) {
				err = format.ErrNotFound{Cid: c}
			}
			node := merkledag.NewRawNode(data)
			out <- &format.NodeOption{Node: node, Err: err}
		}
	}()
	return out
}

func (d *dagStore) AddMany(ctx context.Context, nodes []format.Node) error {
	if !d.ds.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	b, err := d.ds.Batch(ctx)
	if err != nil {
		return err
	}

	for _, n := range nodes {
		id := n.Cid().String()
		data := n.RawData()
		if err := b.Put(ctx, NewKey(id), data); err != nil {
			return err
		}
	}
	return b.Commit(ctx)
}

func (d *dagStore) Remove(ctx context.Context, cid cid.Cid) error {
	if !d.ds.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return d.ds.Delete(ctx, NewKey(cid.String()))
}

func (d *dagStore) RemoveMany(ctx context.Context, cids []cid.Cid) error {
	if !d.ds.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	b, err := d.ds.Batch(ctx)
	if err != nil {
		return err
	}

	for _, c := range cids {
		if err := b.Delete(ctx, NewKey(c.String())); err != nil {
			return err
		}
	}
	return b.Commit(ctx)
}
