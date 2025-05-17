package libp2praft

import (
	"bytes"
	"io"

	consensus "github.com/libp2p/go-libp2p-consensus"
	"github.com/ugorji/go/codec"
)

// encodeOp serializes an op
func encodeOp(op consensus.Op) ([]byte, error) {
	var buf bytes.Buffer
	err := encode(op, &buf)
	return buf.Bytes(), err
}

// decodeOp deserializes an op
func decodeOp(bs []byte, op consensus.Op) (err error) {
	buf := bytes.NewBuffer(bs)
	return decode(op, buf)
}

func encode(v interface{}, w io.Writer) error {
	if marshable, ok := v.(Marshable); ok {
		return marshable.Marshal(w)
	}

	// Use default encoding using Msgpack otherwise
	enc := codec.NewEncoder(w, &codec.MsgpackHandle{})
	return enc.Encode(v)
}

func decode(v interface{}, r io.Reader) error {
	if marshable, ok := v.(Marshable); ok {
		return marshable.Unmarshal(r)
	}

	h := &codec.MsgpackHandle{}
	h.ErrorIfNoField = true
	dec := codec.NewDecoder(r, h)
	return dec.Decode(v)
}

// EncodeSnapshot serializes a state and is used by our raft's FSM
// implementation to describe the format raft stores snapshots on
// disk. In the state implements Marshable, it will use the
// Marshal function.
func EncodeSnapshot(state consensus.State, w io.Writer) error {
	return encode(state, w)
}

// DecodeSnapshot de-serializes a state encoded with EncodeSnapshot onto the
// given state. It is used by our raft's FSM implementation which allows raft
// to read snapshots. If state implements Marshable, it will use the
// Unmarshal function. Please make sure that the state's underlying type
// is a pointer.
func DecodeSnapshot(state consensus.State, r io.Reader) error {
	return decode(state, r)
}
