package network

import "github.com/Arceliar/ironwood/types"

/***********
 * traffic *
 ***********/

type traffic struct {
	path      []peerPort // *not* zero terminated
	from      []peerPort
	source    publicKey
	dest      publicKey
	watermark uint64
	payload   []byte
}

func (tr *traffic) copyFrom(original *traffic) {
	tmp := *tr
	*tr = *original
	tr.path = append(tmp.path[:0], tr.path...)
	tr.from = append(tmp.from[:0], tr.from...)
	tr.payload = append(tmp.payload[:0], tr.payload...)
}

func (tr *traffic) size() int {
	size := wireSizePath(tr.path)
	size += wireSizePath(tr.from)
	size += len(tr.source)
	size += len(tr.dest)
	size += wireSizeUint(tr.watermark)
	size += len(tr.payload)
	return size
}

func (tr *traffic) encode(out []byte) ([]byte, error) {
	start := len(out)
	out = wireAppendPath(out, tr.path)
	out = wireAppendPath(out, tr.from)
	out = append(out, tr.source[:]...)
	out = append(out, tr.dest[:]...)
	out = wireAppendUint(out, tr.watermark)
	out = append(out, tr.payload...)
	end := len(out)
	if end-start != tr.size() {
		panic("this should never happen")
	}
	return out, nil
}

func (tr *traffic) decode(data []byte) error {
	var tmp traffic
	tmp.path = tr.path[:0]
	tmp.from = tr.from[:0]
	if !wireChopPath(&tmp.path, &data) {
		return types.ErrDecode
	} else if !wireChopPath(&tmp.from, &data) {
		return types.ErrDecode
	} else if !wireChopSlice(tmp.source[:], &data) {
		return types.ErrDecode
	} else if !wireChopSlice(tmp.dest[:], &data) {
		return types.ErrDecode
	} else if !wireChopUint(&tmp.watermark, &data) {
		return types.ErrDecode
	}
	tmp.payload = append(tr.payload[:0], data...)
	*tr = tmp
	return nil
}

// Functions needed for pqPacket

func (tr *traffic) wireType() wirePacketType {
	return wireTraffic
}

func (tr *traffic) sourceKey() publicKey {
	return tr.source
}

func (tr *traffic) destKey() publicKey {
	return tr.dest
}
