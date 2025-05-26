package network

import (
	"container/heap"
	"time"
)

type pqPacket interface {
	wireEncodeable
	wireType() wirePacketType
	sourceKey() publicKey
	destKey() publicKey
}

type pqPacketInfo struct {
	packet pqPacket
	size   uint64
	time   time.Time
}

type pqSource struct {
	key   publicKey
	infos []pqPacketInfo
	size  uint64
}

type pqDest struct {
	key     publicKey
	sources []pqSource
	size    uint64
}

type packetQueue struct {
	dests []pqDest
	size  uint64
}

// drop will remove a packet from the queue
// the packet removed will be the oldest packet from the longest stream to the largest destination queue
// returns true if a packet was removed, false otherwise
func (q *packetQueue) drop() bool {
	if q.size == 0 {
		return false
	}
	var dIdx int
	for idx := range q.dests {
		if q.dests[idx].size > q.dests[dIdx].size {
			dIdx = idx
		}
	}
	dest := q.dests[dIdx]
	var sIdx int
	for idx := range dest.sources {
		if dest.sources[idx].size > dest.sources[sIdx].size {
			sIdx = idx
		}
	}
	source := dest.sources[sIdx]
	info := source.infos[0]
	source.size -= info.size
	if len(source.infos) > 0 {
		source.infos = source.infos[1:]
	}
	dest.sources[sIdx] = source
	if source.size > 0 {
		heap.Fix(&dest, sIdx)
	} else {
		heap.Remove(&dest, sIdx)
	}
	dest.size -= info.size
	q.dests[dIdx] = dest
	if dest.size > 0 {
		heap.Fix(q, dIdx)
	} else {
		heap.Remove(q, dIdx)
	}
	q.size -= info.size
	switch p := info.packet.(type) {
	case *traffic:
		freeTraffic(p)
	default:
		// Nothing to do
	}
	return true
}

// push adds a packet with the provided size to a queue for the provided source and destination keys
// a new queue will be created if needed
func (q *packetQueue) push(packet pqPacket) {
	sKey := packet.sourceKey()
	dKey := packet.destKey()
	size := packet.size()
	info := pqPacketInfo{packet: packet, size: uint64(size), time: time.Now()}
	sIdx, dIdx := -1, -1
	source, dest := pqSource{key: sKey}, pqDest{key: dKey}
	for idx, d := range q.dests {
		if d.key.equal(dKey) {
			dIdx, dest = idx, d
			break
		}
	}
	for idx, s := range dest.sources {
		if s.key.equal(sKey) {
			sIdx, source = idx, s
			break
		}
	}
	source.infos = append(source.infos, info)
	source.size += info.size
	if sIdx < 0 {
		dest.sources = append(dest.sources, source)
	} else {
		dest.sources[sIdx] = source
	}
	dest.size += info.size
	if dIdx < 0 {
		q.dests = append(q.dests, dest)
	} else {
		q.dests[dIdx] = dest
	}
	q.size += info.size
}

// pop removes and returns the oldest packet (from across all source/destination pairs)
func (q *packetQueue) pop() (info pqPacketInfo, ok bool) {
	if q.size > 0 {
		dest := q.dests[0]
		source := dest.sources[0]
		info = source.infos[0]
		source.size -= info.size
		dest.size -= info.size
		q.size -= info.size
		if len(source.infos) > 1 {
			source.infos = source.infos[1:]
			dest.sources[0] = source
			heap.Fix(&dest, 0)
		} else {
			dest.sources[0] = source
			heap.Remove(&dest, 0)
		}
		if len(dest.sources) > 0 {
			q.dests[0] = dest
			heap.Fix(q, 0)
		} else {
			q.dests[0] = dest
			heap.Remove(q, 0)
		}
		return info, true
	}
	return
}

func (q *packetQueue) peek() (info pqPacketInfo, ok bool) {
	if len(q.dests) > 0 {
		return q.dests[0].sources[0].infos[0], true
	}
	return
}

////////////////////////////////////////////////////////////////////////////////

// Interface methods for packetQueue to satisfy heap.Interface

func (q *packetQueue) Len() int {
	return len(q.dests)
}

func (q *packetQueue) Less(i, j int) bool {
	return q.dests[i].sources[0].infos[0].time.Before(q.dests[j].sources[0].infos[0].time)
}

func (q *packetQueue) Swap(i, j int) {
	q.dests[i], q.dests[j] = q.dests[j], q.dests[i]
}

func (q *packetQueue) Push(x interface{}) {
	dest := x.(pqDest)
	q.dests = append(q.dests, dest)
	q.size += dest.size
}

func (q *packetQueue) Pop() interface{} {
	idx := len(q.dests) - 1
	dest := q.dests[idx]
	q.dests = q.dests[:idx]
	q.size -= dest.size
	return dest
}

// Interface methods for pqDest to satisfy heap.Interface

func (d *pqDest) Len() int {
	return len(d.sources)
}

func (d *pqDest) Less(i, j int) bool {
	return d.sources[i].infos[0].time.Before(d.sources[j].infos[0].time)
}

func (d *pqDest) Swap(i, j int) {
	d.sources[i], d.sources[j] = d.sources[j], d.sources[i]
}

func (d *pqDest) Push(x interface{}) {
	source := x.(pqSource)
	d.sources = append(d.sources, source)
	d.size += source.size
}

func (d *pqDest) Pop() interface{} {
	idx := len(d.sources) - 1
	source := d.sources[idx]
	d.sources = d.sources[:idx]
	d.size -= source.size
	return source
}
