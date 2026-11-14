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
	"slices"

	"github.com/Warp-net/warpnet/core/warpnet"
	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/ipfs/go-ds-crdt/pb"
	"google.golang.org/protobuf/proto"
)

const ErrForeignRoot = warpnet.WarpError("crdt store: block is not filed under the DAG it was announced in")

// checkedDelta is the delta every received block is read into. Before the
// merge it refuses a block not filed under the DAG it was reached from, and
// keeps no tombstones and only the writes the store accepts.
type checkedDelta struct {
	*pb.Delta

	crdt *MerkleCRDT
}

var _ crdt.Delta = (*checkedDelta)(nil)

func (d *checkedDelta) Unmarshal(b []byte) error {
	if err := proto.Unmarshal(b, d.Delta); err != nil {
		return err
	}
	if !d.crdt.lineage.isRootedIn(b, d.GetDagName()) {
		return ErrForeignRoot
	}
	d.Tombstones = nil
	d.Elements = slices.DeleteFunc(d.Elements, func(e *pb.Element) bool {
		return d.crdt.validate(d.GetDagName(), e.GetKey(), e.GetValue()) != nil
	})
	return nil
}

func (d *checkedDelta) GetElements() ([]*pb.Element, error) {
	return d.Delta.GetElements(), nil
}

func (d *checkedDelta) GetTombstones() ([]*pb.Element, error) {
	return d.Delta.GetTombstones(), nil
}

func (d *checkedDelta) SetElements(elems []*pb.Element) {
	d.Elements = elems
}

func (d *checkedDelta) SetTombstones(tombs []*pb.Element) {
	d.Tombstones = tombs
}

func (d *checkedDelta) SetPriority(p uint64) {
	d.Priority = p
}

func (d *checkedDelta) Size() int {
	return proto.Size(d.Delta)
}

func (d *checkedDelta) SetDagName(n string) {
	d.DagName = n
}

func (d *checkedDelta) IsEmpty() bool {
	return len(d.Tombstones)+len(d.Elements) == 0
}

func (d *checkedDelta) Marshal() ([]byte, error) {
	return proto.Marshal(d.Delta)
}
