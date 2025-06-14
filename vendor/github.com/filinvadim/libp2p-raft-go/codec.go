package raft

import (
	"github.com/vmihailenco/msgpack/v5"
	"io"
)

type EncodeDecoder interface {
	Decode(v interface{}, r io.Reader) error
	Encode(v interface{}, w io.Writer) error
	Unmarshal(data []byte, v interface{}) error
	Marshal(v interface{}) ([]byte, error)
}

type DefaultCodec struct{}

func (c *DefaultCodec) Encode(v interface{}, w io.Writer) error {
	enc := msgpack.NewEncoder(w)
	return enc.Encode(v)
}

func (c *DefaultCodec) Decode(v interface{}, r io.Reader) error {
	dec := msgpack.NewDecoder(r)
	return dec.Decode(v)
}

func (c *DefaultCodec) Unmarshal(data []byte, v interface{}) error {
	return msgpack.Unmarshal(data, v)
}

func (c *DefaultCodec) Marshal(v interface{}) ([]byte, error) {
	return msgpack.Marshal(v)
}
