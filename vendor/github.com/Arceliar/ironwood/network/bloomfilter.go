package network

import (
	"encoding/binary"

	bfilter "github.com/bits-and-blooms/bloom/v3"

	"github.com/Arceliar/phony"

	"github.com/Arceliar/ironwood/types"
)

const (
	bloomFilterF = 16               // number of bytes used for flags in the wire format, should be bloomFilterU / 8, rounded up
	bloomFilterU = bloomFilterF * 8 // number of uint64s in the backing array
	bloomFilterB = bloomFilterU * 8 // number of bytes in the backing array
	bloomFilterM = bloomFilterB * 8 // number of bits in teh backing array
	bloomFilterK = 8                // number of hashes to use per inserted key
)

// bloom is bloomFilterM bits long bloom filter uses bloomFilterK hash functions.
// Maybe this should be a *bfilter.BloomFilter directly, no struct?
type bloom struct {
	filter *bfilter.BloomFilter
}

func newBloom() *bloom {
	return &bloom{
		filter: bfilter.New(bloomFilterM, bloomFilterK),
	}
}

func (b *bloom) addKey(key publicKey) {
	b.filter.Add(key[:])
}

func (b *bloom) addFilter(f *bfilter.BloomFilter) {
	b.filter.Merge(f)
}

func (b *bloom) size() int {
	size := bloomFilterF // Flags for chunks that are all 0 bits
	size += bloomFilterF // Flags for chunks that are all 1 bits
	us := b.filter.BitSet().Bytes()
	for _, u := range us {
		if u != 0 && u != ^uint64(0) {
			size += 8
		}
	}
	return size
}

func (b *bloom) encode(out []byte) ([]byte, error) {
	start := len(out)
	var flags0, flags1 [bloomFilterF]byte
	keep := make([]uint64, 0, bloomFilterU)
	us := b.filter.BitSet().Bytes()
	for idx, u := range us {
		if u == 0 {
			flags0[idx/8] |= 0x80 >> (uint64(idx) % 8)
			continue
		}
		if u == ^uint64(0) {
			flags1[idx/8] |= 0x80 >> (uint64(idx) % 8)
			continue
		}
		keep = append(keep, u)
	}
	out = append(out, flags0[:]...)
	out = append(out, flags1[:]...)
	var buf [8]byte
	for _, u := range keep {
		binary.BigEndian.PutUint64(buf[:], u)
		out = append(out, buf[:]...)
	}
	end := len(out)
	if end-start != b.size() {
		panic("this should never happen")
	}
	return out, nil
}

func (b *bloom) decode(data []byte) error {
	var tmp bloom
	var usArray [bloomFilterU]uint64
	us := usArray[:0]
	var flags0, flags1 [bloomFilterF]byte
	if !wireChopSlice(flags0[:], &data) {
		return types.ErrDecode
	} else if !wireChopSlice(flags1[:], &data) {
		return types.ErrDecode
	}
	for idx := 0; idx < bloomFilterU; idx++ {
		flag0 := flags0[idx/8] & (0x80 >> (uint64(idx) % 8))
		flag1 := flags1[idx/8] & (0x80 >> (uint64(idx) % 8))
		if flag0 != 0 && flag1 != 0 {
			return types.ErrDecode
		} else if flag0 != 0 {
			us = append(us, 0)
		} else if flag1 != 0 {
			us = append(us, ^uint64(0))
		} else if len(data) >= 8 {
			u := binary.BigEndian.Uint64(data[:8])
			us = append(us, u)
			data = data[8:]
		} else {
			return types.ErrDecode
		}
	}
	if len(data) != 0 {
		return types.ErrDecode
	}
	tmp.filter = bfilter.From(us, bloomFilterK)
	*b = tmp
	return nil
}

/*****************************
 * router bloom filter stuff *
 *****************************/

type blooms struct {
	router *router
	blooms map[publicKey]bloomInfo
	// TODO? add some kind of timeout and keepalive timer to force an update/send
}

type bloomInfo struct {
	send   bloom
	recv   bloom
	seq    uint16 // used to force a filter to resend once/hour in _sendAllBlooms, TODO something better
	onTree bool
	zDirty bool
}

func (bs *blooms) init(r *router) {
	bs.router = r
	bs.blooms = make(map[publicKey]bloomInfo)
}

func (bs *blooms) _isOnTree(key publicKey) bool {
	return bs.blooms[key].onTree //|| key == bs.router.core.crypto.publicKey
}

func (bs *blooms) _fixOnTree() {
	selfKey := bs.router.core.crypto.publicKey
	if selfInfo, isIn := bs.router.infos[selfKey]; isIn {
		for pk, pbi := range bs.blooms {
			wasOn := pbi.onTree
			pbi.onTree = false
			if selfInfo.parent == pk {
				pbi.onTree = true
			} else if info, isIn := bs.router.infos[pk]; isIn {
				if info.parent == selfKey {
					pbi.onTree = true
				}
			} else {
				// They must not have sent us their info yet
			}
			if wasOn && !pbi.onTree {
				// We dropped them from the tree, so we need to send a blank update
				// That way, if the link returns to the tree, we don't start with false positives
				b := newBloom()
				pbi.send = *b
				for p := range bs.router.peers[pk] {
					p.sendBloom(bs.router, b)
				}
			}
			bs.blooms[pk] = pbi
		}
	} else {
		panic("this should never happen")
	}
}

func (bs *blooms) xKey(key publicKey) publicKey {
	k := key
	xfed := bs.router.core.config.bloomTransform(k.toEd())
	var xform publicKey
	copy(xform[:], xfed)
	return xform
}

func (bs *blooms) _addInfo(key publicKey) {
	bs.blooms[key] = bloomInfo{
		send: *newBloom(),
		recv: *newBloom(),
	}
}

func (bs *blooms) _removeInfo(key publicKey) {
	delete(bs.blooms, key)
	// We'll need to send updated blooms, but this can happen during regular maintenance
}

func (bs *blooms) handleBloom(fromPeer *peer, b *bloom) {
	bs.router.Act(fromPeer, func() {
		bs._handleBloom(fromPeer, b)
	})
}

func (bs blooms) _handleBloom(fromPeer *peer, b *bloom) {
	pbi, isIn := bs.blooms[fromPeer.key]
	if !isIn {
		return
	}
	pbi.recv = *b
	bs.blooms[fromPeer.key] = pbi
}

func (bs *blooms) _doMaintenance() {
	bs._fixOnTree()
	bs._sendAllBlooms()
}

func (bs *blooms) _getBloomFor(key publicKey, keepOnes bool) (*bloom, bool) {
	// getBloomFor increments the sequence number, even if we only send it to 1 peer
	// this means we may sometimes unnecessarily send a bloom when we get a new peer link to an existing peer node
	pbi, isIn := bs.blooms[key]
	if !isIn {
		panic("this should never happen")
	}
	b := newBloom()
	xform := bs.xKey(bs.router.core.crypto.publicKey)
	b.addKey(xform)
	for k, pbi := range bs.blooms {
		if !pbi.onTree {
			continue
		}
		if k == key {
			continue
		}
		b.addFilter(bs.blooms[k].recv.filter)
	}
	if keepOnes {
		// Don't reset existing 1 bits, we'll set anything unnecessairy to 0 next time
		// Ensures that 1s travel faster than 0s, to help prevent flapping
		if !pbi.zDirty {
			c := b.filter.Copy()
			b.addFilter(pbi.send.filter)
			if !b.filter.Equal(c) {
				// We're keeping unnecessairy 1 bits, so set the dirty flag
				pbi.zDirty = true
			}
		} else {
			b.addFilter(pbi.send.filter)
		}
	} else {
		pbi.zDirty = false
	}
	isNew := true
	if b.filter.Equal(pbi.send.filter) {
		*b = pbi.send
		isNew = false
	} else {
		pbi.send = *b
		bs.blooms[key] = pbi
	}
	return b, isNew
}

func (bs *blooms) _sendBloom(p *peer) {
	// Just send whatever our most recently sent bloom is
	// For new or off-tree nodes, this is the empty bloom filter
	b := bs.blooms[p.key].send
	p.sendBloom(bs.router, &b)
}

func (bs *blooms) _sendAllBlooms() {
	for k, pbi := range bs.blooms {
		if !pbi.onTree {
			continue
		}
		keepOnes := !pbi.zDirty
		b, isNew := bs._getBloomFor(k, keepOnes)
		pbi = bs.blooms[k] // Changed during call to getBloomFor
		pbi.seq++
		if isNew || pbi.seq >= 3600 {
			if ps, isIn := bs.router.peers[k]; isIn {
				for p := range ps {
					p.sendBloom(bs.router, b)
				}
			} else {
				panic("this should never happen")
			}
			pbi.seq = 0
		}
		bs.blooms[k] = pbi
	}
}

func (bs *blooms) sendMulticast(from phony.Actor, packet pqPacket, fromKey publicKey, toKey publicKey) {
	// Ideally we need a way to detect duplicate packets from multiple links to the same peer, so we can drop them
	// I.e. we need to sequence number all multicast packets... This can maybe be part of the framing, along side the packet length, or something
	// For now, we just send to 1 peer (possibly at random)
	bs.router.Act(from, func() {
		bs._sendMulticast(packet, fromKey, toKey)
	})
}

func (bs *blooms) _sendMulticast(packet pqPacket, fromKey publicKey, toKey publicKey) {
	// TODO make very sure this can't loop, even temporarily due to network state changes being delayed
	//  Does the onTree state stay safe, even when we're delaying maintenance from message updates?...
	xform := bs.xKey(toKey)
	for k, pbi := range bs.blooms {
		if !pbi.onTree {
			// This is not on the tree, so skip it
			continue
		}
		if k == fromKey {
			// From this key, so don't send it back
			continue
		}
		if !pbi.recv.filter.Test(xform[:]) {
			// The bloom filter tells us this peer definitely doesn't carea bout this xformed toKey
			continue
		}
		// Send this broadcast packet to the peer
		var bestPeer *peer
		for p := range bs.router.peers[k] {
			if bestPeer == nil || p.prio < bestPeer.prio {
				bestPeer = p
			}
		}
		if bestPeer == nil {
			panic("this should never happen")
		}
		bestPeer.sendQueued(bs.router, packet)
	}
}
