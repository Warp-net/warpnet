/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package consensus

import (
	"bytes"
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/hashicorp/raft"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
	"io"
	"sync"
)

const ErrConsensusRejection = warpnet.WarpError("consensus: quorum rejected your node. Try to delete database and update app version")

type KVState map[string]string

type fsm struct {
	state     *KVState
	prevState KVState

	mux *sync.Mutex

	validators []ConsensusValidatorFunc
}

type ConsensusValidatorFunc func(k, v string) error

func newFSM(validators ...ConsensusValidatorFunc) *fsm {
	state := KVState{"genesis": ""}
	return &fsm{
		state:      &state,
		prevState:  KVState{},
		mux:        new(sync.Mutex),
		validators: validators,
	}
}

func (fsm *fsm) AmendValidator(validator ConsensusValidatorFunc) {
	fsm.validators = append(fsm.validators, validator)
}

// Apply is invoked by Raft once a log entry is commited. Do not use directly.
func (fsm *fsm) Apply(rlog *raft.Log) (result interface{}) {
	fsm.mux.Lock()
	defer fsm.mux.Unlock()
	defer func() {
		if r := recover(); r != nil {
			*fsm.state = fsm.prevState
			result = warpnet.WarpError("consensus: fsm apply panic: rollback")
		}
	}()

	if rlog.Type != raft.LogCommand {
		return nil
	}

	var newState = make(KVState, 1)
	if err := msgpack.Unmarshal(rlog.Data, &newState); err != nil {
		log.Errorf("consensus: failed to decode log: %v", err)
		return fmt.Errorf("consensus: failed to decode log: %w", err)
	}

	for _, validator := range fsm.validators {
		for k, v := range newState {
			if err := validator(k, v); err != nil {
				return err
			}
		}
	}

	fsm.prevState = make(KVState, len(*fsm.state))
	for k, v := range *fsm.state {
		fsm.prevState[k] = v
	}

	for k, v := range newState {
		(*fsm.state)[k] = v
	}
	newState = nil
	return fsm.state
}

// Snapshot encodes the current state so that we can save a snapshot.
func (fsm *fsm) Snapshot() (raft.FSMSnapshot, error) {
	fsm.mux.Lock()
	defer fsm.mux.Unlock()

	buf := new(bytes.Buffer)
	err := msgpack.NewEncoder(buf).Encode(fsm.state)
	if err != nil {
		return nil, err
	}

	return &fsmSnapshot{state: buf}, nil
}

// Restore takes a snapshot and sets the current state from it.
func (fsm *fsm) Restore(reader io.ReadCloser) error {
	defer reader.Close()
	fsm.mux.Lock()
	defer fsm.mux.Unlock()

	err := msgpack.NewDecoder(reader).Decode(fsm.state)
	if err != nil {
		log.Errorf("consensus: fsm: decoding snapshot: %s", err)
		return err
	}

	fsm.prevState = make(map[string]string, len(*fsm.state))
	return nil
}

type fsmSnapshot struct {
	state *bytes.Buffer
}

// Persist writes the snapshot (a serialized state) to a raft.SnapshotSink.
func (snap *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	_, err := io.Copy(sink, snap.state)
	if err != nil {
		_ = sink.Cancel()
		return err
	}
	return sink.Close()
}

func (snap *fsmSnapshot) Release() {
	log.Debugln("consensus: fsm: releasing snapshot")
}
