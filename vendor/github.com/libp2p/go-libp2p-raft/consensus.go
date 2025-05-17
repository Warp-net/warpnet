package libp2praft

import (
	"errors"
	"io"

	consensus "github.com/libp2p/go-libp2p-consensus"
)

// Consensus implements both the go-libp2p-consensus Consensus
// and the OpLogConsensus interfaces. This is because the Consensus
// interface is just a particular case of the OpLog interface,
// where the operation applied holds the new version of the state
// and replaces the current one with it.
type Consensus struct {
	fsm   *FSM
	actor consensus.Actor
}

// Marshable is an interface to be implemented by consensus.States and
// consensus.Op objects that wish to choose their serialization format for
// Raft snapshots.  When needing to serialize States or Ops, Marhsal and
// Unmarshal methods will be used when provided. Otherwise, a default
// serialization strategy will be used (using Msgpack).
type Marshable interface {
	// Marshal serializes the state and writes it in the Writer.
	Marshal(io.Writer) error
	// Unmarshal deserializes the state from the given Reader. The
	// unmarshaled object must fully replace the original data. We
	// recommend that Unmarshal errors when the object being deserialized
	// produces a non-consistent result (for example by trying to
	// deserialized a struct with wrong fields onto a different struct.
	// Rollback operations will attempt to deserialize State objects on Op
	// objects first, thus this case must error.
	Unmarshal(io.Reader) error
}

// stateOp: A Consensus is a particular case of OpLogConsensus where the
// Operation is itself the state and every new operation just replaces the
// previous state. For this case, we use this operation. It tracks the state
// itself. It is also used for performing rollbacks to a given state.
type stateOp struct {
	State consensus.State
}

// ApplyTo just returns the state in the operation and discards the
// "old" one.
func (sop *stateOp) ApplyTo(st consensus.State) (consensus.State, error) {
	return sop.State, nil
}

// Marshal uses the underlying state Marshaling
func (sop *stateOp) Marshal(w io.Writer) error {
	return EncodeSnapshot(sop.State, w)
}

// Unmarshal uses the underlying state Unmarshaling
func (sop *stateOp) Unmarshal(r io.Reader) error {
	return DecodeSnapshot(sop.State, r)
}

// NewConsensus returns a new consensus. The state is provided so that
// the appropiate internal structures can be initialized. The underlying
// State type should be a pointer, otherwise some operations will not work.
//
// Note that this initial state is not agreed upon in the cluster and that
// GetCurrentState() will return an error until a state is agreed upon. Only
// states submitted via CommitState() are agreed upon.
//
// The state can optionally implement the Marshable interface. The methods
// will be used to serialize and deserialize Raft snapshots. If Marshable is
// not implemented, the state will be [de]serialized using Msgpack.
//
// We recommend using OpLog when handling very big states, as otherwise the
// full state will need to be dump into memory on every commit, before being
// sent on the wire as a single Raft log entry.
func NewConsensus(state consensus.State) *Consensus {
	return NewOpLog(state, &stateOp{State: state})
}

// NewOpLog returns a new OpLog. Because the State and the Ops
// are generic, and we know nothing about them, they need to
// be provided in order to initialize internal structures with the
// right types. Both the state and the op parameters are not used nor
// agreed upon just yet. Both the state and the op underlying types should
// be pointers to something, otherwise expect some misbehaviours.
//
// It is important to note that the system agrees on an operation log,
// but the resulting state can only be considered agreed-upon if all
// the operations in the log can be or were successfully applied to it
// (with Op.ApplyTo()).
// See the notes in CommitOp() for more information.
//
// The state and the op can optionally implement the Marshable interface
// which allows user-provided object to decide how they are serialized
// and deserialized.
//
// We recommend keeping operations as small as possible. Note that operations
// need to be fully serialized copied on memory before being sent (due to Raft
// requirements).
func NewOpLog(state consensus.State, op consensus.Op) *Consensus {
	return &Consensus{
		fsm: &FSM{
			state:        state,
			op:           op,
			initialized:  false,
			inconsistent: false,
			subscriberCh: nil,
		},
		actor: nil,
	}
}

// FSM returns the raft.FSM implementation provided by go-libp2p-raft. It is
// necessary to initialize raft with this FSM.
func (c *Consensus) FSM() *FSM {
	return c.fsm
}

// SetActor changes the actor in charge of submitting new states to the system.
func (c *Consensus) SetActor(actor consensus.Actor) {
	c.actor = actor
}

// GetCurrentState returns the upon-agreed state of the system. It will
// return an error when no state has been agreed upon or when the state
// cannot be ensured to be that on which the rest of the system has
// agreed-upon. The underlying state will be the same as provided
// initialization.
func (c *Consensus) GetCurrentState() (consensus.State, error) {
	return c.GetLogHead()
}

// CommitOp submits a new operation to the system. If the operation is
// agreed-upon, then and only then will it call ApplyTo() and modify the
// current state (log head).
//
// If ApplyTo() fails, the operation stays nevertheless
// in the log, but the state of the system is marked as invalid and
// nil is returned. From that point the state is marked as inconsistent
// and calls to GetLogHead() will fail for this node, even though updates
// will be processed as usual.
//
// Inconsistent states can be rescued using Rollback().
// The underlying object to the returned State will be the one provided
// during initialization.
func (opLog *Consensus) CommitOp(op consensus.Op) (consensus.State, error) {
	if opLog.actor == nil {
		return nil, errors.New("no actor set to commit the new state")
	}
	newSt, err := opLog.actor.SetState(op)
	if err != nil {
		return nil, err
	}
	return newSt, nil
}

// GetLogHead returns the newest known state of the log. It will
// return an error when no state has been agreed upon or when the state
// cannot be ensured to be that on which the rest of the system has
// agreed-upon.
func (opLog *Consensus) GetLogHead() (consensus.State, error) {
	return opLog.fsm.getState()
}

// CommitState pushes a new state to the system and returns
// the state the system has agreed upon. It will block until
// that happens.
//
// Note that only the Raft leader can commit a state.
func (c *Consensus) CommitState(state consensus.State) (consensus.State, error) {
	return c.CommitOp(&stateOp{state})
}

// Rollback hammers the provided state into the system. It does not un-do any
// operations nor checks that the given state was previously agreed-upon. A
// special rollback operation gets added to the log, like any other operation.
// A successful rollback marks an inconsistent state as valid again.
//
// Note that the full state needs to be loaded onto memory (like an operation)
// so this is potentially dangerous with very large states.
func (opLog *Consensus) Rollback(state consensus.State) error {
	_, err := opLog.CommitState(state)
	return err
}

// Subscribe returns a channel which is notified on every state update.
func (c *Consensus) Subscribe() <-chan struct{} {
	return c.fsm.subscribe()
}

// Unsubscribe closes the channel returned upon Subscribe() (if any).
func (c *Consensus) Unsubscribe() {
	c.fsm.unsubscribe()
}
