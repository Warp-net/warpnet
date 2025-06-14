package raft

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/hashicorp/raft"
	"io"
	"sync"
)

var ErrStateNotInitialized = errors.New("FSM is not initialized")

type KVState map[string]string // the easiest and most understandable type of state

type fsm struct {
	state     *KVState
	prevState KVState // just in case of "fire"

	mux *sync.Mutex

	codec EncodeDecoder

	validators []ConsensusValidatorFunc
}

type ConsensusValidatorFunc func(k, v string) (bool, error)

func NewFSM(codec EncodeDecoder, validators ...ConsensusValidatorFunc) *fsm {
	state := KVState{}
	return &fsm{
		state:      &state,
		prevState:  KVState{},
		mux:        new(sync.Mutex),
		validators: validators,
		codec:      codec,
	}
}

func (fsm *fsm) GetCurrentState() *KVState {
	if fsm == nil || fsm.state == nil {
		return nil
	}
	fsm.mux.Lock()
	cp := make(KVState, len(*fsm.state))
	for k, v := range *fsm.state {
		cp[k] = v
	}
	fsm.mux.Unlock()
	return &cp
}

func (fsm *fsm) AmendValidator(validator ConsensusValidatorFunc) {
	if fsm == nil || fsm.state == nil {
		return
	}
	fsm.validators = append(fsm.validators, validator)
}

// Apply is invoked by Raft once a log entry is commited. Do not use directly.
func (fsm *fsm) Apply(rlog *raft.Log) (result interface{}) {
	if fsm == nil || fsm.state == nil {
		return ErrStateNotInitialized
	}
	defer func() {
		if r := recover(); r != nil {
			*fsm.state = fsm.prevState
			result = errors.New("consensus: fsm apply panic: rollback")
		}
	}()

	fsm.mux.Lock()
	defer fsm.mux.Unlock()

	if rlog.Type != raft.LogCommand {
		return nil
	}

	var newState = make(KVState, 1)
	if err := fsm.codec.Unmarshal(rlog.Data, &newState); err != nil {
		return fmt.Errorf("consensus: failed to decode log: %w", err)
	}

	for _, validator := range fsm.validators {
		for k, v := range newState {
			isValidatorApplied, err := validator(k, v)
			if err != nil {
				return err
			}
			// if it's only a validator - skip state update
			if isValidatorApplied {
				return fsm.state
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
	if fsm == nil || fsm.state == nil {
		return nil, ErrStateNotInitialized
	}
	fsm.mux.Lock()
	defer fsm.mux.Unlock()

	buf := new(bytes.Buffer)
	err := fsm.codec.Encode(fsm.state, buf)
	if err != nil {
		return nil, err
	}

	return &fsmSnapshot{state: buf}, nil
}

// Restore takes a snapshot and sets the current state from it.
func (fsm *fsm) Restore(reader io.ReadCloser) error {
	defer reader.Close()

	if fsm == nil || fsm.state == nil {
		return ErrStateNotInitialized
	}

	fsm.mux.Lock()
	defer fsm.mux.Unlock()

	err := fsm.codec.Decode(fsm.state, reader)
	if err != nil {
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
	if snap == nil {
		return nil
	}
	_, err := io.Copy(sink, snap.state)
	if err != nil {
		_ = sink.Cancel()
		return err
	}
	return sink.Close()
}

func (snap *fsmSnapshot) Release() {}
