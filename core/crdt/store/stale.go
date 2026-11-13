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
	"encoding/binary"
	"reflect"

	datastore "github.com/ipfs/go-datastore"
	log "github.com/sirupsen/logrus"
)

// go-ds-crdt keeps, under its set namespace, each key's winning value at
// k/<key>/v and that value's priority at k/<key>/p (vendored set.go).
const (
	setKeysNs      = "k"
	setValueSuffix = "v"
	setPrioSuffix  = "p"
)

// readSetNamespace finds where go-ds-crdt keeps the set: the namespace is an
// unexported field, so it is read by reflection. Any surprise in the
// library's layout only turns the stale-version check off.
func (m *MerkleCRDT) readSetNamespace() (ns datastore.Key, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("crdt: reading the set namespace: %v", r)
			ns, ok = datastore.Key{}, false
		}
	}()
	set := reflect.ValueOf(m.MerkleCRDT.Datastore).Elem().FieldByName("set")
	if !set.IsValid() || set.Kind() != reflect.Pointer || set.IsNil() {
		return ns, false
	}
	field := set.Elem().FieldByName("namespace")
	if !field.IsValid() || field.Type() != reflect.TypeFor[datastore.Key]() || field.NumField() != 1 {
		return ns, false
	}
	return datastore.RawKey(field.Field(0).String()), true
}

// current is the value go-ds-crdt has chosen for key and the priority it
// was chosen with.
func (m *MerkleCRDT) current(key string) (value []byte, priority uint64, ok bool) {
	ns := m.setNamespace.Load()
	if ns == nil {
		return nil, 0, false
	}
	keyNs := ns.ChildString(setKeysNs).ChildString(key)
	ctx := context.Background()
	value, err := m.data.Get(ctx, keyNs.ChildString(setValueSuffix))
	if err != nil {
		return nil, 0, false
	}
	raw, err := m.data.Get(ctx, keyNs.ChildString(setPrioSuffix))
	if err != nil {
		return nil, 0, false
	}
	stored, n := binary.Uvarint(raw)
	if n <= 0 || stored == 0 {
		return nil, 0, false
	}
	return value, stored - 1, true
}
