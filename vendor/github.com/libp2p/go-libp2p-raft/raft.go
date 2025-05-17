// Package libp2praft implements the go-libp2p-consensus interface
// wrapping the github.com/hashicorp/raft implementation, providing
// a custom generic FSM to handle states and generic operations and
// giving the user a go-libp2p-based network transport to use.
//
// The main entry points to this library are the Consensus and Actor
// types. Usually, the first step is to create the Consensus, then
// use the *Consensus.FSM() object to initialize a Raft instance, along with
// the Libp2pTransport. With a Raft instance, an Actor can be
// created and then used with Consensus.SetActor(). From this point,
// the consensus system is ready to use.
//
// It is IMPORTANT to make a few notes about the types of objects
// to be used as consensus.State and consensus.Op, since the
// go-libp2p-consensus interface does not make many assumptions
// about them (consensus.State being an empty interface).
//
// Raft will need to send serialized version of the state and the operation
// objects. Default serialization uses MsgPack and requires that relevant
// fields are exported. Unexported fields will not be serialized and therefore
// not received in other nodes. Their local value will never change
// either. This includes the fields from children structs etc.  Therefore, it
// is recommended to simplify user defined types like consensus.Op and
// consensus.State as much as possible and declare all relevant fields as
// exported.
//
// Alternative, it is possible to use a custom serialization and
// deserialization mechanism by having consensus.State and consensus.Op
// implement the Marshable interface. This provides full control about
// how things are sent on the wire.
//
// A consensus.Op ApplyTo() operation may return an error. This
// means that, while the operation is agreed-upon, the resulting
// state cannot be produced. This marks the state in that node
// as dirty but does not removes the operation itself.
// See CommitOp() for more details.
//
// The underlying state for consensus.State should be a pointer,
// otherwise some operations won't work. Once provided, the state
// should only be modifed by this library.
package libp2praft

import (
	logging "github.com/ipfs/go-log/v2"
)

var (
	logger = logging.Logger("libp2p-raft")
)
