package libp2praft

import (
	"errors"
	"time"

	"github.com/hashicorp/raft"
	consensus "github.com/libp2p/go-libp2p-consensus"
	"github.com/libp2p/go-libp2p/core/peer"
)

// SetStateTimeout specifies how long before giving up on setting a state
var SetStateTimeout = 1 * time.Second

// Actor implements a consensus.Actor, allowing to SetState
// in a libp2p Consensus system. In order to do this it uses hashicorp/raft
// implementation of the Raft algorithm.
type Actor struct {
	Raft *raft.Raft
}

// NewActor returns a new actor given a hashicorp/raft node.
func NewActor(r *raft.Raft) *Actor {
	return &Actor{
		Raft: r,
	}
}

// SetState attempts to set the state of the cluster to the state
// represented by the given Node. It will block until the state is
// commited, and will then return then the new state.
//
// This does not mean that the new state is already available in all
// the nodes in the cluster, but that it will be at some point because
// it is part of the authoritative log.
//
// Only the Raft leader can set the state. Otherwise, an error will
// be returned.
func (actor *Actor) SetState(newState consensus.State) (consensus.State, error) {
	// figure out if this is an Op.
	op, ok := newState.(consensus.Op)
	if ok {
		return actor.commitOp(op)
	}
	return actor.commitOp(&stateOp{newState})
}

// commitOp actually does the job of setting the state, which is simply
// an opConsensus operation with the new state. Everything stated for SetState
// applies here.
func (actor *Actor) commitOp(op consensus.Op) (consensus.State, error) {
	//log.Debug("actor is applying state")
	if actor.Raft == nil {
		return nil, errors.New("this actor does not have a raft instance")
	}

	if !actor.IsLeader() {
		return nil, errors.New("this actor is not the leader")
	}

	bs, err := encodeOp(op)
	if err != nil {
		return nil, err
	}

	applyFuture := actor.Raft.Apply(bs, SetStateTimeout)

	// Error blocks until apply future is "considered commited"
	// which means "commited to the local FSM"
	err = applyFuture.Error()

	futureResp := applyFuture.Response()
	//log.Debugf("apply future log entry index: %d", applyFuture.Index())
	return futureResp, err
}

// IsLeader returns of the current actor is Raft leader
func (actor *Actor) IsLeader() bool {
	if actor.Raft != nil {
		return actor.Raft.State() == raft.Leader
	}
	return false
}

// Leader returns the LibP2P ID of the Raft leader or an
// error if there is no leader.
func (actor *Actor) Leader() (peer.ID, error) {
	// Leader as returned by Libp2pTransport.LocalAddr()
	leaderAddr, _ := actor.Raft.LeaderWithID()
	peerID, err := peer.Decode(string(leaderAddr))
	if err != nil {
		return "", errors.New("leader unknown or not existing yet")
	}
	return peerID, nil
}
