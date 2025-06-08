package raft

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"log"
	"time"
)

func stream(
	parentCtx context.Context, n host.Host, peerId peer.ID, r protocol.ID, data any,
) ([]byte, error) {
	if n == nil || peerId.String() == "" || r == "" {
		return nil, errors.New("stream: parameters improperly configured")
	}

	if err := peerId.Validate(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(parentCtx, time.Second*5)
	defer cancel()

	s, err := n.NewStream(ctx, peerId, r)
	if err != nil {
		return nil, fmt.Errorf("stream: new: %v", err)
	}
	defer closeStream(s)

	var rw = bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
	if data != nil {
		bt, err := jsoniter.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("stream: marshal: %v", err)
		}
		err = nil
		_, err = rw.Write(bt)
		if err != nil {
			flush(rw)
			closeWrite(s)
			return nil, fmt.Errorf("stream: writing: %s", err)
		}
	}
	flush(rw)
	closeWrite(s)

	buf := bytes.NewBuffer(nil)
	num, err := buf.ReadFrom(rw)
	if err != nil {
		return nil, fmt.Errorf("stream: reading response from %s: %w", peerId.String(), err)
	}

	if num == 0 {
		return nil, fmt.Errorf("stream: protocol %s, peer ID %s: empty response", r, peerId.String())
	}
	return buf.Bytes(), nil
}

func closeStream(stream network.Stream) {
	if err := stream.Close(); err != nil {
		log.Printf("stream: closing: %s", err)
	}
}

func flush(rw *bufio.ReadWriter) {
	if err := rw.Flush(); err != nil {
		log.Printf("stream: flush: %s", err)
	}
}

func closeWrite(s network.Stream) {
	if err := s.CloseWrite(); err != nil {
		log.Printf("stream: close write: %s", err)
	}
}
