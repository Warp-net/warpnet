package network

import (
	"time"

	"github.com/Arceliar/ironwood/types"
)

const pathfinderTrafficCache = true

// WARNING The pathfinder should only be used from within the router's actor, it's not threadsafe
type pathfinder struct {
	router *router
	info   pathNotifyInfo
	paths  map[publicKey]pathInfo
	rumors map[publicKey]pathRumor
	logger func(*pathLookup)
}

func (pf *pathfinder) init(r *router) {
	pf.router = r
	pf.info.sign(pf.router.core.crypto.privateKey)
	pf.paths = make(map[publicKey]pathInfo)
	pf.rumors = make(map[publicKey]pathRumor)
}

func (pf *pathfinder) _sendLookup(dest publicKey) {
	if info, isIn := pf.paths[dest]; isIn {
		if time.Since(info.reqTime) < pf.router.core.config.pathThrottle {
			// Don't flood with request, wait a bit
			return
		}
	}
	selfKey := pf.router.core.crypto.publicKey
	_, from := pf.router._getRootAndPath(selfKey)
	lookup := pathLookup{
		source: selfKey,
		dest:   dest,
		from:   from,
	}
	pf._handleLookup(lookup.source, &lookup)
}

func (pf *pathfinder) handleLookup(p *peer, lookup *pathLookup) {
	pf.router.Act(p, func() {
		if !pf.router.blooms._isOnTree(p.key) {
			return
		}
		pf._handleLookup(p.key, lookup)
	})
}

func (pf *pathfinder) _handleLookup(fromKey publicKey, lookup *pathLookup) {
	if pf.logger != nil {
		pf.logger(lookup)
	}
	// Continue the multicast
	pf.router.blooms._sendMulticast(lookup, fromKey, lookup.dest)
	// Check if we should send a response too
	dx := pf.router.blooms.xKey(lookup.dest)
	sx := pf.router.blooms.xKey(pf.router.core.crypto.publicKey)
	if dx == sx {
		// We match, send a response
		// TODO? throttle this per dest that we're sending a response to?
		_, path := pf.router._getRootAndPath(pf.router.core.crypto.publicKey)
		notify := pathNotify{
			path:      lookup.from,
			watermark: ^uint64(0),
			source:    pf.router.core.crypto.publicKey,
			dest:      lookup.source,
			info: pathNotifyInfo{
				seq:  uint64(time.Now().Unix()), //pf.info.seq,
				path: path,
			},
		}
		if !pf.info.equal(notify.info) {
			//notify.info.seq++
			notify.info.sign(pf.router.core.crypto.privateKey)
			pf.info = notify.info
		} else {
			notify.info = pf.info
		}
		pf._handleNotify(notify.source, &notify)
	}
}

func (pf *pathfinder) handleNotify(p *peer, notify *pathNotify) {
	pf.router.Act(p, func() {
		pf._handleNotify(p.key, notify)
	})
}

func (pf *pathfinder) _handleNotify(fromKey publicKey, notify *pathNotify) {
	if p := pf.router._lookup(notify.path, &notify.watermark); p != nil {
		p.sendPathNotify(pf.router, notify)
		return
	}
	// Check if we should accept this response
	if notify.dest != pf.router.core.crypto.publicKey {
		return
	}
	var info pathInfo
	var isIn bool
	// Note that we need to res.check() in every case (as soon as success is otherwise inevitable)
	if info, isIn = pf.paths[notify.source]; isIn {
		if notify.info.seq <= info.seq {
			// This isn't newer than the last seq we received, so drop it
			return
		}
		nfo := notify.info
		nfo.path = info.path
		if nfo.equal(notify.info) {
			// This doesn't actually add anything new, so skip it
			return
		}
		if !notify.check() {
			return
		}
		info.timer.Reset(pf.router.core.config.pathTimeout)
	} else {
		xform := pf.router.blooms.xKey(notify.source)
		if _, isIn := pf.rumors[xform]; !isIn {
			return
		}
		if !notify.check() {
			return
		}
		key := notify.source
		var timer *time.Timer
		timer = time.AfterFunc(pf.router.core.config.pathTimeout, func() {
			pf.router.Act(nil, func() {
				if info := pf.paths[key]; info.timer == timer {
					timer.Stop()
					delete(pf.paths, key)
					if info.traffic != nil {
						freeTraffic(info.traffic)
					}
				}
			})
		})
		info = pathInfo{
			reqTime: time.Now(),
			timer:   timer,
		}
		if rumor := pf.rumors[xform]; rumor.traffic != nil && rumor.traffic.dest == notify.source {
			info.traffic = rumor.traffic
			rumor.traffic = nil
			pf.rumors[xform] = rumor
		}
	}
	info.path = notify.info.path
	info.seq = notify.info.seq
	info.broken = false
	if info.traffic != nil {
		tr := info.traffic
		info.traffic = nil
		// We defer so it happens after we've store the updated info in the map
		defer pf._handleTraffic(tr)
	}
	pf.paths[notify.source] = info
	pf.router.core.config.pathNotify(notify.source.toEd())
}

func (pf *pathfinder) _rumorSendLookup(dest publicKey) {
	xform := pf.router.blooms.xKey(dest)
	if rumor, isIn := pf.rumors[xform]; isIn {
		if time.Since(rumor.sendTime) < pf.router.core.config.pathThrottle {
			return
		}
		rumor.sendTime = time.Now()
		rumor.timer.Reset(pf.router.core.config.pathTimeout)
		pf.rumors[xform] = rumor
	} else {
		var timer *time.Timer
		timer = time.AfterFunc(pf.router.core.config.pathTimeout, func() {
			pf.router.Act(nil, func() {
				if rumor := pf.rumors[xform]; rumor.timer == timer {
					delete(pf.rumors, xform)
					timer.Stop()
					if rumor.traffic != nil {
						freeTraffic(rumor.traffic)
					}
				}
			})
		})
		pf.rumors[xform] = pathRumor{
			sendTime: time.Now(),
			timer:    timer,
		}
	}
	pf._sendLookup(dest)
}

func (pf *pathfinder) _handleTraffic(tr *traffic) {
	const cache = pathfinderTrafficCache // TODO make this unconditional, this is just to easily toggle the cache on/off for now
	if info, isIn := pf.paths[tr.dest]; isIn {
		tr.path = append(tr.path[:0], info.path...)
		_, from := pf.router._getRootAndPath(pf.router.core.crypto.publicKey)
		tr.from = append(tr.from[:0], from...)
		if cache {
			if info.traffic != nil {
				freeTraffic(info.traffic)
			}
			info.traffic = allocTraffic()
			info.traffic.copyFrom(tr)
			pf.paths[tr.dest] = info
		}
		pf.router.handleTraffic(nil, tr)
	} else {
		pf._rumorSendLookup(tr.dest)
		if cache {
			xform := pf.router.blooms.xKey(tr.dest)
			if rumor, isIn := pf.rumors[xform]; isIn {
				if rumor.traffic != nil {
					freeTraffic(rumor.traffic)
				}
				rumor.traffic = tr
				pf.rumors[xform] = rumor
			} else {
				panic("this should never happen")
			}
		}
	}
}

func (pf *pathfinder) _doBroken(tr *traffic) {
	broken := pathBroken{
		path:      append([]peerPort(nil), tr.from...),
		watermark: ^uint64(0),
		source:    tr.source,
		dest:      tr.dest,
	}
	pf._handleBroken(&broken)
}

func (pf *pathfinder) _handleBroken(broken *pathBroken) {
	// Hack using traffic to do routing
	if p := pf.router._lookup(broken.path, &broken.watermark); p != nil {
		p.sendPathBroken(pf.router, broken)
		return
	}
	// Check if we should accept this pathBroken
	if broken.source != pf.router.core.crypto.publicKey {
		return
	}
	if info, isIn := pf.paths[broken.dest]; isIn {
		info.broken = true
		pf.paths[broken.dest] = info
		pf._sendLookup(broken.dest) // Throttled inside this function
	}
}

func (pf *pathfinder) handleBroken(p *peer, broken *pathBroken) {
	pf.router.Act(p, func() {
		pf._handleBroken(broken)
	})
}

func (pf *pathfinder) _resetTimeout(key publicKey) {
	// Note: We should call this when we receive a packet from this destination
	// We should *not* reset just because we tried to send a packet
	// We need things to time out eventually if e.g. a node restarts and resets its seqs
	if info, isIn := pf.paths[key]; isIn && !info.broken {
		info.timer.Reset(pf.router.core.config.pathTimeout)
	}
}

/************
 * pathInfo *
 ************/

type pathInfo struct {
	path    []peerPort // *not* zero terminated (and must be free of zeros)
	seq     uint64
	reqTime time.Time   // Time a request was last sent (to prevent spamming)
	timer   *time.Timer // time.AfterFunc(cleanup...), reset whenever we receive traffic from this node
	traffic *traffic
	broken  bool // Set to true if we receive a pathBroken, which prevents the timer from being reset (we must get a new notify to clear)
}

/*************
 * pathRumor *
 *************/

type pathRumor struct {
	traffic  *traffic
	sendTime time.Time   // Time we last sent a rumor (to prevnt spamming)
	timer    *time.Timer // time.AfterFunc(cleanup...)
}

/**************
 * pathLookup *
 **************/

type pathLookup struct {
	source publicKey
	dest   publicKey
	from   []peerPort
}

func (lookup *pathLookup) size() int {
	size := len(lookup.source)
	size += len(lookup.dest)
	size += wireSizePath(lookup.from)
	return size
}

func (lookup *pathLookup) encode(out []byte) ([]byte, error) {
	start := len(out)
	out = append(out, lookup.source[:]...)
	out = append(out, lookup.dest[:]...)
	out = wireAppendPath(out, lookup.from)
	end := len(out)
	if end-start != lookup.size() {
		panic("this should never happen")
	}
	return out, nil
}

func (lookup *pathLookup) decode(data []byte) error {
	var tmp pathLookup
	orig := data
	if !wireChopSlice(tmp.source[:], &orig) {
		return types.ErrDecode
	} else if !wireChopSlice(tmp.dest[:], &orig) {
		return types.ErrDecode
	} else if !wireChopPath(&tmp.from, &orig) {
		return types.ErrDecode
	} else if len(orig) != 0 {
		return types.ErrDecode
	}
	*lookup = tmp
	return nil
}

// Needed for pqPacket interface

func (lookup *pathLookup) wireType() wirePacketType {
	return wireProtoPathLookup
}

func (lookup *pathLookup) sourceKey() publicKey {
	return lookup.source
}

func (lookup *pathLookup) destKey() publicKey {
	return lookup.dest
}

/******************
 * pathNotifyInfo *
 ******************/

type pathNotifyInfo struct {
	seq  uint64
	path []peerPort // Path from root to source, aka coords, zero-terminated
	sig  signature  // signature from the source key
}

// equal returns true if the pathResponseInfos are equal, inspecting the contents of the path and ignoring the sig
func (info *pathNotifyInfo) equal(cmp pathNotifyInfo) bool {
	if info.seq != cmp.seq {
		return false
	} else if len(info.path) != len(cmp.path) {
		return false
	}
	for idx := range info.path {
		if info.path[idx] != cmp.path[idx] {
			return false
		}
	}
	return true
}

func (info *pathNotifyInfo) bytesForSig() []byte {
	var out []byte
	out = wireAppendUint(out, info.seq)
	out = wireAppendPath(out, info.path)
	return out
}

func (info *pathNotifyInfo) sign(key privateKey) {
	info.sig = key.sign(info.bytesForSig())
}

func (info *pathNotifyInfo) size() int {
	size := wireSizeUint(info.seq)
	size += wireSizePath(info.path)
	size += len(info.sig)
	return size
}

func (info *pathNotifyInfo) encode(out []byte) ([]byte, error) {
	start := len(out)
	out = wireAppendUint(out, info.seq)
	out = wireAppendPath(out, info.path)
	out = append(out, info.sig[:]...)
	end := len(out)
	if end-start != info.size() {
		panic("this should never happen")
	}
	return out, nil
}

func (info *pathNotifyInfo) decode(data []byte) error {
	var tmp pathNotifyInfo
	orig := data
	if !wireChopUint(&tmp.seq, &orig) {
		return types.ErrDecode
	} else if !wireChopPath(&tmp.path, &orig) {
		return types.ErrDecode
	} else if !wireChopSlice(tmp.sig[:], &orig) {
		return types.ErrDecode
	} else if len(orig) != 0 {
		return types.ErrDecode
	}
	*info = tmp
	return nil
}

/**************
 * pathNotify *
 **************/

type pathNotify struct {
	path      []peerPort
	watermark uint64
	source    publicKey // who sent the response, not who resquested it
	dest      publicKey // exact key we are sending response to
	info      pathNotifyInfo
}

func (notify *pathNotify) check() bool {
	return notify.source.verify(notify.info.bytesForSig(), &notify.info.sig)
}

func (notify *pathNotify) size() int {
	size := wireSizePath(notify.path)
	size += wireSizeUint(notify.watermark)
	size += len(notify.source)
	size += len(notify.dest)
	size += notify.info.size()
	return size
}

func (notify *pathNotify) encode(out []byte) ([]byte, error) {
	start := len(out)
	out = wireAppendPath(out, notify.path)
	out = wireAppendUint(out, notify.watermark)
	out = append(out, notify.source[:]...)
	out = append(out, notify.dest[:]...)
	var err error
	if out, err = notify.info.encode(out); err != nil {
		return nil, err
	}
	end := len(out)
	if end-start != notify.size() {
		panic("this should never happen")
	}
	return out, nil
}

func (notify *pathNotify) decode(data []byte) error {
	var tmp pathNotify
	orig := data
	if !wireChopPath(&tmp.path, &orig) {
		return types.ErrDecode
	} else if !wireChopUint(&tmp.watermark, &orig) {
		return types.ErrDecode
	} else if !wireChopSlice(tmp.source[:], &orig) {
		return types.ErrDecode
	} else if !wireChopSlice(tmp.dest[:], &orig) {
		return types.ErrDecode
	} else if err := tmp.info.decode(orig); err != nil {
		return err
	}
	*notify = tmp
	return nil
}

func (notify *pathNotify) wireType() wirePacketType {
	return wireProtoPathNotify
}

func (notify *pathNotify) sourceKey() publicKey {
	return notify.source
}

func (notify *pathNotify) destKey() publicKey {
	return notify.dest
}

/**************
 * pathBroken *
 **************/

type pathBroken struct {
	path      []peerPort
	watermark uint64
	source    publicKey
	dest      publicKey
}

func (broken *pathBroken) size() int {
	size := wireSizePath(broken.path)
	size += wireSizeUint(broken.watermark)
	size += len(broken.source)
	size += len(broken.dest)
	return size
}

func (broken *pathBroken) encode(out []byte) ([]byte, error) {
	start := len(out)
	out = wireAppendPath(out, broken.path)
	out = wireAppendUint(out, broken.watermark)
	out = append(out, broken.source[:]...)
	out = append(out, broken.dest[:]...)
	end := len(out)
	if end-start != broken.size() {
		panic("this should never happen")
	}
	return out, nil
}

func (broken *pathBroken) decode(data []byte) error {
	var tmp pathBroken
	orig := data
	if !wireChopPath(&tmp.path, &orig) {
		return types.ErrDecode
	} else if !wireChopUint(&tmp.watermark, &orig) {
		return types.ErrDecode
	} else if !wireChopSlice(tmp.source[:], &orig) {
		return types.ErrDecode
	} else if !wireChopSlice(tmp.dest[:], &orig) {
		return types.ErrDecode
	} else if len(orig) != 0 {
		return types.ErrDecode
	}
	*broken = tmp
	return nil
}

func (broken *pathBroken) wireType() wirePacketType {
	return wireProtoPathBroken
}

func (broken *pathBroken) sourceKey() publicKey {
	return broken.source
}

func (broken *pathBroken) destKey() publicKey {
	return broken.dest
}
