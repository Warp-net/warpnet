package consensus

type State interface {
}

type Consensus interface {
	// GetCurrentState returns the current agreed upon state of the system
	GetCurrentState() (State, error)

	// CommitState attempts to set the current state of the system. It returns
	// the new state of the system if the call succeeds.
	CommitState(State) (State, error)

	// SetActor sets the underlying actor that will actually be performing the
	// operations of setting and changing the state.
	SetActor(Actor)
}

type Actor interface {
	// SetState attempts to set the state of the cluster to the state
	// represented by the given Node
	SetState(State) (State, error)
}

type Op interface {
	ApplyTo(State) (State, error)
}

type OpLogConsensus interface {
	// CommitOp takes the current state and applies the given operation to it, then
	// sets the new state as the clusters 'current' state.
	CommitOp(Op) (State, error)

	SetActor(Actor)

	// GetLogHead returns a node that represents the current state of the cluster
	GetLogHead() (State, error)

	// Rollback rolls the state of the cluster back to the given node. The
	// passed in node should be a node that was previously a state in the chain of
	// states. Although some discussion will need to be had as to whether or not
	// this is truly necessary
	Rollback(State) error
}
