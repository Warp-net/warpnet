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

	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/ipfs/go-ds-crdt/pb"
	"google.golang.org/protobuf/proto"
)

// checkedDelta is the delta every received block is read into: before the
// block is merged it keeps only the elements isValid accepts for the
// block's own DAG, and no tombstones, since these stores never delete. A
// write that fails the check never takes part in choosing a key's value.
type checkedDelta struct {
	*pb.Delta

	isValid func(dagName, key string, value []byte) bool
}

var _ crdt.Delta = (*checkedDelta)(nil)

func (d *checkedDelta) Unmarshal(b []byte) error {
	if err := proto.Unmarshal(b, d.Delta); err != nil {
		return err
	}
	d.Tombstones = nil
	d.Elements = slices.DeleteFunc(d.Elements, func(e *pb.Element) bool {
		return !d.isValid(d.GetDagName(), e.GetKey(), e.GetValue())
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
	if d == nil {
		return 0
	}
	return proto.Size(d.Delta)
}

func (d *checkedDelta) GetDagName() string {
	return d.Delta.GetDagName()
}

func (d *checkedDelta) SetDagName(n string) {
	d.DagName = n
}

func (d *checkedDelta) IsEmpty() bool {
	return d == nil || (len(d.Tombstones)+len(d.Elements) == 0)
}

func (d *checkedDelta) Marshal() ([]byte, error) {
	if d != nil {
		return proto.Marshal(d.Delta)
	}
	return nil, nil
}
