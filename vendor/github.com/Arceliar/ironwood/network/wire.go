package network

import "encoding/binary"

type wirePacketType byte

const (
	wireDummy wirePacketType = iota // unused
	wireKeepAlive
	wireProtoSigReq
	wireProtoSigRes
	wireProtoAnnounce
	wireProtoBloomFilter
	wireProtoPathLookup
	wireProtoPathNotify
	wireProtoPathBroken
	wireTraffic
)

func wireChopSlice(out []byte, data *[]byte) bool {
	if len(*data) < len(out) {
		return false
	}
	copy(out, *data)
	*data = (*data)[len(out):]
	return true
}

func wireChopBytes(out *[]byte, data *[]byte, size int) bool {
	if len(*data) < size {
		return false
	}
	*out = append(*out, (*data)[:size]...)
	*data = (*data)[size:]
	return true
}

func wireChopUint(out *uint64, data *[]byte) bool {
	var u uint64
	var l int
	if u, l = binary.Uvarint(*data); l <= 0 {
		return false
	}
	*out, *data = u, (*data)[l:]
	return true
}

func wireSizeUint(u uint64) int {
	var b [10]byte
	return binary.PutUvarint(b[:], u)
}

func wireAppendUint(out []byte, u uint64) []byte {
	return binary.AppendUvarint(out, u)
}

type wireEncodeable interface {
	size() int
	encode(out []byte) ([]byte, error)
}

func wireEncode(out []byte, pType uint8, obj wireEncodeable) ([]byte, error) {
	out = append(out, pType)
	var err error
	if out, err = obj.encode(out); err != nil {
		return nil, err
	}
	return out, nil
}

func wireSizePath(path []peerPort) int {
	var size int
	for _, port := range path {
		size += wireSizeUint(uint64(port))
	}
	size += wireSizeUint(0)
	return size
}

func wireAppendPath(dest []byte, path []peerPort) []byte {
	for _, port := range path {
		dest = wireAppendUint(dest, uint64(port))
	}
	dest = wireAppendUint(dest, 0)
	return dest
}

func wireDecodePath(source []byte) (path []peerPort, length int) {
	bs := source
	for {
		var u uint64
		if !wireChopUint(&u, &bs) {
			return nil, -1 // TODO correct value
		}
		if u == 0 {
			break
		}
		path = append(path, peerPort(u))
	}
	length = len(source) - len(bs)
	return
}

func wireChopPath(out *[]peerPort, data *[]byte) bool {
	path, length := wireDecodePath(*data)
	if length < 0 {
		return false
	}
	*out = append(*out, path...)
	*data = (*data)[length:]
	return true
}
