package encrypted

import (
	"encoding/binary"
	"time"

	"github.com/Arceliar/phony"

	"github.com/Arceliar/ironwood/types"
)

/*

TODO:
  We either need to save the private keys for sent inits (so we can make a session when we receive an ack...) *or* we need to send an ack back when we receive an ack and we create a session because of it

*/

const (
	sessionTimeout            = time.Minute
	sessionTrafficOverheadMin = 1 + 1 + 1 + 1 + boxOverhead + boxPubSize // header, seq, seq, nonce
	sessionTrafficOverhead    = sessionTrafficOverheadMin + 9 + 9 + 9
	sessionInitSize           = 1 + boxPubSize + boxOverhead + edSigSize + boxPubSize + boxPubSize + 8 + 8
	sessionAckSize            = sessionInitSize
)

const (
	sessionTypeDummy = iota
	sessionTypeInit
	sessionTypeAck
	sessionTypeTraffic
)

/******************
 * sessionManager *
 ******************/

type sessionManager struct {
	phony.Inbox
	pc       *PacketConn
	sessions map[edPub]*sessionInfo
	buffers  map[edPub]*sessionBuffer
}

func (mgr *sessionManager) init(pc *PacketConn) {
	mgr.pc = pc
	mgr.sessions = make(map[edPub]*sessionInfo)
	mgr.buffers = make(map[edPub]*sessionBuffer)
}

func (mgr *sessionManager) _newSession(ed *edPub, recv, send boxPub, seq uint64) *sessionInfo {
	info := newSession(ed, recv, send, seq)
	info.Act(mgr, func() {
		info.mgr = mgr
		info._resetTimer()
	})
	mgr.sessions[info.ed] = info
	return info
}

func (mgr *sessionManager) _sessionForInit(pub *edPub, init *sessionInit) (*sessionInfo, *sessionBuffer) {
	var info *sessionInfo
	var buf *sessionBuffer
	if info = mgr.sessions[*pub]; info == nil {
		info = mgr._newSession(pub, init.current, init.next, init.seq)
		if buf = mgr.buffers[*pub]; buf != nil {
			buf.timer.Stop()
			delete(mgr.buffers, *pub)
			info.sendPub, info.sendPriv = buf.init.current, buf.currentPriv
			info.nextPub, info.nextPriv = buf.init.next, buf.nextPriv
			info._fixShared(0, 0)
			// The caller is responsible for sending buf.data when ready
		}
	}
	return info, buf
}

func (mgr *sessionManager) handleData(from phony.Actor, pub *edPub, data []byte) {
	mgr.Act(from, func() {
		if len(data) == 0 {
			return
		}
		switch data[0] {
		case sessionTypeDummy:
		case sessionTypeInit:
			init := new(sessionInit)
			if init.decrypt(&mgr.pc.secretBox, pub, data) {
				mgr._handleInit(pub, init)
			}
			freeBytes(data)
		case sessionTypeAck:
			ack := new(sessionAck)
			if ack.decrypt(&mgr.pc.secretBox, pub, data) {
				mgr._handleAck(pub, ack)
			}
			freeBytes(data)
		case sessionTypeTraffic:
			mgr._handleTraffic(pub, data)
		default:
		}
	})
}

func (mgr *sessionManager) _handleInit(pub *edPub, init *sessionInit) {
	if info, buf := mgr._sessionForInit(pub, init); info != nil {
		info.handleInit(mgr, init)
		if buf != nil && buf.data != nil {
			info.doSend(mgr, buf.data)
		}
	}
}

func (mgr *sessionManager) _handleAck(pub *edPub, ack *sessionAck) {
	_, isOld := mgr.sessions[*pub]
	if info, buf := mgr._sessionForInit(pub, &ack.sessionInit); info != nil {
		if isOld {
			info.handleAck(mgr, ack)
		} else {
			info.handleInit(mgr, &ack.sessionInit)
		}
		if buf != nil && buf.data != nil {
			info.doSend(mgr, buf.data)
		}
	}
}

func (mgr *sessionManager) _handleTraffic(pub *edPub, msg []byte) {
	if info := mgr.sessions[*pub]; info != nil {
		info.doRecv(mgr, msg)
	} else {
		// We don't know that the node really exists, it could be spoofed/replay
		// So we don't want to save session or a buffer based on this node
		// So we send an init with keys we'll forget
		// If they ack, we'll set up a session and let it self-heal...
		currentPub, _ := newBoxKeys()
		nextPub, _ := newBoxKeys()
		init := newSessionInit(&currentPub, &nextPub, 0)
		mgr.sendInit(pub, &init)
	}
}

func (mgr *sessionManager) writeTo(toKey edPub, msg []byte) {
	// WARNING: unsafe to call from within an actor, must only be exposed over the PacketConn functions (which are, themselves, unsafe for actors to call in most cases, since they may block)
	phony.Block(mgr, func() {
		if info := mgr.sessions[toKey]; info != nil {
			info.doSend(mgr, msg)
		} else {
			// Need to buffer the traffic
			mgr._bufferAndInit(toKey, msg)
		}
	})
}

func (mgr *sessionManager) _bufferAndInit(toKey edPub, msg []byte) {
	var buf *sessionBuffer
	if buf = mgr.buffers[toKey]; buf == nil {
		// Create a new buffer (including timer)
		buf = new(sessionBuffer)
		currentPub, currentPriv := newBoxKeys()
		nextPub, nextPriv := newBoxKeys()
		buf.init = newSessionInit(&currentPub, &nextPub, 0)
		buf.currentPriv = currentPriv
		buf.nextPriv = nextPriv
		buf.timer = time.AfterFunc(0, func() {})
		mgr.buffers[toKey] = buf
	}
	buf.data = msg
	buf.timer.Stop()
	mgr.sendInit(&toKey, &buf.init)
	buf.timer = time.AfterFunc(sessionTimeout, func() {
		mgr.Act(nil, func() {
			if b := mgr.buffers[toKey]; b == buf {
				b.timer.Stop()
				delete(mgr.buffers, toKey)
			}
		})
	})
}

func (mgr *sessionManager) sendInit(dest *edPub, init *sessionInit) {
	if bs, err := init.encrypt(&mgr.pc.secretEd, dest); err == nil {
		mgr.pc.PacketConn.WriteTo(bs, types.Addr(dest.asKey()))
	}
}

func (mgr *sessionManager) sendAck(dest *edPub, ack *sessionAck) {
	if bs, err := ack.encrypt(&mgr.pc.secretEd, dest); err == nil {
		mgr.pc.PacketConn.WriteTo(bs, types.Addr(dest.asKey()))
	}
}

/***************
 * sessionInfo *
 ***************/

type sessionInfo struct {
	phony.Inbox
	mgr            *sessionManager
	seq            uint64 // remote seq
	ed             edPub  // remote ed key
	remoteKeySeq   uint64 // signals rotation of current/next
	current        boxPub // send to this, expect to receive from it
	next           boxPub // if we receive from this, then rotate it to current
	localKeySeq    uint64 // signals rotation of recv/send/next
	recvPriv       boxPriv
	recvPub        boxPub
	recvShared     boxShared
	recvNonce      uint64
	sendPriv       boxPriv // becomes recvPriv when we ratchet forward
	sendPub        boxPub  // becomes recvPub
	sendShared     boxShared
	sendNonce      uint64
	nextPriv       boxPriv // becomes sendPriv
	nextPub        boxPub  // becomes sendPub
	timer          *time.Timer
	ack            *sessionAck
	since          time.Time
	rotated        time.Time // last time we rotated keys
	rx             uint64
	tx             uint64
	nextSendShared boxShared
	nextSendNonce  uint64
	nextRecvShared boxShared
	nextRecvNonce  uint64
}

func newSession(ed *edPub, current, next boxPub, seq uint64) *sessionInfo {
	info := new(sessionInfo)
	info.seq = seq - 1 // so the first update works
	info.ed = *ed
	info.current, info.next = current, next
	info.recvPub, info.recvPriv = newBoxKeys()
	info.sendPub, info.sendPriv = newBoxKeys()
	info.nextPub, info.nextPriv = newBoxKeys()
	info.since = time.Now()
	info._fixShared(0, 0)
	return info
}

// happens at session creation or after receiving an init/ack
func (info *sessionInfo) _fixShared(recvNonce, sendNonce uint64) {
	getShared(&info.recvShared, &info.current, &info.recvPriv)
	getShared(&info.sendShared, &info.current, &info.sendPriv)
	getShared(&info.nextSendShared, &info.next, &info.sendPriv)
	getShared(&info.nextRecvShared, &info.next, &info.recvPriv)
	info.nextSendNonce, info.nextRecvNonce = 0, 0
	info.recvNonce, info.sendNonce = recvNonce, sendNonce
}

func (info *sessionInfo) _resetTimer() {
	if info.timer != nil {
		info.timer.Stop()
	}
	info.timer = time.AfterFunc(sessionTimeout, func() {
		info.mgr.Act(nil, func() {
			if oldInfo := info.mgr.sessions[info.ed]; oldInfo == info {
				delete(info.mgr.sessions, info.ed)
			}
		})
	})
}

func (info *sessionInfo) handleInit(from phony.Actor, init *sessionInit) {
	info.Act(from, func() {
		if init.seq <= info.seq {
			return
		}
		info._handleUpdate(init)
		// Send a sessionAck
		info._sendAck()
	})
}

func (info *sessionInfo) handleAck(from phony.Actor, ack *sessionAck) {
	info.Act(from, func() {
		if ack.seq <= info.seq {
			return
		}
		info._handleUpdate(&ack.sessionInit)
	})
}

// return true if everything looks OK and the session was updated
func (info *sessionInfo) _handleUpdate(init *sessionInit) {
	info.current = init.current
	info.next = init.next
	info.seq = init.seq
	info.remoteKeySeq = init.keySeq
	// Advance our keys, since this counts as a response
	info.recvPub, info.recvPriv = info.sendPub, info.sendPriv
	info.sendPub, info.sendPriv = info.nextPub, info.nextPriv
	info.nextPub, info.nextPriv = newBoxKeys()
	info.localKeySeq++
	// Don't roll back sendNonce, just to be extra safe
	info._fixShared(0, info.sendNonce)
	info._resetTimer()
}

func (info *sessionInfo) doSend(from phony.Actor, msg []byte) {
	// TODO? some worker pool to multi-thread this
	info.Act(from, func() {
		defer freeBytes(msg)
		info.sendNonce += 1 // Advance the nonce before anything else
		if info.sendNonce == 0 {
			// Nonce overflowed, so rotate keys
			info.recvPub, info.recvPriv = info.sendPub, info.sendPriv
			info.sendPub, info.sendPriv = info.nextPub, info.nextPriv
			info.nextPub, info.nextPriv = newBoxKeys()
			info.localKeySeq++
			info._fixShared(0, 0)
		}
		bs := allocBytes(sessionTrafficOverhead + len(msg))
		defer freeBytes(bs)
		bs[0] = sessionTypeTraffic
		offset := 1
		offset += binary.PutUvarint(bs[offset:], info.localKeySeq)
		offset += binary.PutUvarint(bs[offset:], info.remoteKeySeq)
		offset += binary.PutUvarint(bs[offset:], info.sendNonce)
		bs = bs[:offset]
		// We need to include info.nextPub below the layer of encryption
		// That way the remote side knows it's us when we send from it later...
		tmp := allocBytes(len(info.nextPub) + len(msg))[:0]
		tmp = append(tmp, info.nextPub[:]...)
		tmp = append(tmp, msg...)
		bs = boxSeal(bs, tmp, info.sendNonce, &info.sendShared)
		freeBytes(tmp)
		// send
		info.mgr.pc.PacketConn.WriteTo(bs, types.Addr(info.ed[:]))
		info.tx += uint64(len(msg))
		info._resetTimer()
	})
}

func (info *sessionInfo) doRecv(from phony.Actor, msg []byte) {
	// TODO? some worker pool to multi-thread this
	info.Act(from, func() {
		orig := msg
		defer freeBytes(orig)
		if len(msg) < sessionTrafficOverheadMin || msg[0] != sessionTypeTraffic {
			return
		}
		offset := 1
		remoteKeySeq, rksLen := binary.Uvarint(msg[offset:])
		if rksLen <= 0 {
			return
		}
		offset += rksLen
		localKeySeq, lksLen := binary.Uvarint(msg[offset:])
		if lksLen <= 0 {
			return
		}
		offset += lksLen
		nonce, nonceLen := binary.Uvarint(msg[offset:])
		if nonceLen <= 0 {
			return
		}
		offset += nonceLen
		msg := msg[offset:]
		fromCurrent := remoteKeySeq == info.remoteKeySeq
		fromNext := remoteKeySeq == info.remoteKeySeq+1
		toRecv := localKeySeq+1 == info.localKeySeq
		toSend := localKeySeq == info.localKeySeq
		var sharedKey *boxShared
		var onSuccess func(boxPub)
		switch {
		case fromCurrent && toRecv:
			// The boring case, nothing to ratchet, just update nonce
			if !(info.recvNonce < nonce) {
				return
			}
			sharedKey = &info.recvShared
			onSuccess = func(_ boxPub) {
				info.recvNonce = nonce
			}
		case fromNext && toSend:
			// The remote side appears to have ratcheted forward
			if !(info.nextSendNonce < nonce) {
				return
			}
			sharedKey = &info.nextSendShared
			onSuccess = func(innerKey boxPub) {
				info.nextSendNonce = nonce
				if info.rotated.IsZero() || time.Since(info.rotated) > time.Minute {
					// Rotate their keys
					info.current = info.next
					info.next = innerKey
					info.remoteKeySeq++ // = remoteKeySeq
					// Rotate our own keys
					info.recvPub, info.recvPriv = info.sendPub, info.sendPriv
					info.sendPub, info.sendPriv = info.nextPub, info.nextPriv
					info.localKeySeq++
					// Generate new next keys
					info.nextPub, info.nextPriv = newBoxKeys()
					// Update nonces
					info._fixShared(nonce, 0)
					info.rotated = time.Now()
				}
			}
		case fromNext && toRecv:
			// The remote side appears to have ratcheted forward early
			// Technically there's no reason we can't handle this
			//panic("DEBUG") // TODO test this
			if !(info.nextRecvNonce < nonce) {
				return
			}
			sharedKey = &info.nextRecvShared
			onSuccess = func(innerKey boxPub) {
				info.nextRecvNonce = nonce
				if info.rotated.IsZero() || time.Since(info.rotated) > time.Minute {
					// Rotate their keys
					info.current = info.next
					info.next = innerKey
					info.remoteKeySeq++ // = remoteKeySeq
					// Rotate our own keys
					info.recvPub, info.recvPriv = info.sendPub, info.sendPriv
					info.sendPub, info.sendPriv = info.nextPub, info.nextPriv
					info.localKeySeq++
					// Generate new next keys
					info.nextPub, info.nextPriv = newBoxKeys()
					// Update nonces
					info._fixShared(nonce, 0)
					info.rotated = time.Now()
				}
			}
		default:
			// We can't make sense of their message
			// Send a sessionInit and hope they ack so we can fix things
			info._sendInit()
			return
		}
		// Decrypt and handle packet
		unboxed, ok := allocBytes(0), false
		defer func() { freeBytes(unboxed) }()
		if unboxed, ok = boxOpen(unboxed, msg, nonce, sharedKey); ok {
			var key boxPub
			copy(key[:], unboxed)
			msg := append(allocBytes(0), unboxed[len(key):]...)
			info.mgr.pc.network.recv(info, msg)
			// Misc remaining followup work
			onSuccess(key)
			info.rx += uint64(len(msg))
			info._resetTimer()
		} else {
			// Keys somehow became out-of-sync
			// This seems to happen in some edge cases if a node restarts
			// Fix by sending a new init
			info._sendInit()
		}
	})
}

func (info *sessionInfo) _sendInit() {
	init := newSessionInit(&info.sendPub, &info.nextPub, info.localKeySeq)
	info.mgr.sendInit(&info.ed, &init)
}

func (info *sessionInfo) _sendAck() {
	init := newSessionInit(&info.sendPub, &info.nextPub, info.localKeySeq)
	ack := sessionAck{init}
	info.mgr.sendAck(&info.ed, &ack)
}

/***************
 * sessionInit *
 ***************/

type sessionInit struct {
	current boxPub
	next    boxPub
	keySeq  uint64
	seq     uint64 // timestamp or similar
}

func newSessionInit(current, next *boxPub, keySeq uint64) sessionInit {
	var init sessionInit
	init.current = *current
	init.next = *next
	init.keySeq = keySeq
	init.seq = uint64(time.Now().Unix())
	return init
}

func (init *sessionInit) encrypt(from *edPriv, to *edPub) ([]byte, error) {
	fromPub, fromPriv := newBoxKeys()
	var toBox *boxPub
	var err error
	if toBox, err = to.toBox(); err != nil {
		return nil, err
	}
	// Get sig bytes
	var sigBytes []byte // TODO initialize to correct size
	sigBytes = append(sigBytes, fromPub[:]...)
	sigBytes = append(sigBytes, init.current[:]...)
	sigBytes = append(sigBytes, init.next[:]...)
	offset := len(sigBytes)
	sigBytes = sigBytes[:offset+8]
	binary.BigEndian.PutUint64(sigBytes[offset:offset+8], init.keySeq)
	offset = len(sigBytes)
	sigBytes = sigBytes[:offset+8]
	binary.BigEndian.PutUint64(sigBytes[offset:offset+8], init.seq)
	// Sign
	sig := edSign(sigBytes, from)
	// Prepare the payload (to be encrypted)
	var payload []byte // TODO initialize to correct size
	payload = append(payload, sig[:]...)
	payload = append(payload, sigBytes[boxPubSize:]...)
	// Encrypt
	var shared boxShared
	getShared(&shared, toBox, &fromPriv)
	bs := boxSeal(nil, payload, 0, &shared)
	// Assemble final message
	data := make([]byte, 1, sessionInitSize)
	data[0] = sessionTypeInit
	data = append(data, fromPub[:]...)
	data = append(data, bs...)
	if len(data) != sessionInitSize {
		panic("this should never happen")
	}
	return data, nil
}

func (init *sessionInit) decrypt(priv *boxPriv, from *edPub, data []byte) bool {
	if len(data) != sessionInitSize {
		return false
	}
	var fromBox boxPub
	offset := 1
	offset = bytesPop(fromBox[:], data, offset)
	bs := data[offset:]
	var shared boxShared
	getShared(&shared, &fromBox, priv)
	var payload []byte
	var ok bool
	if payload, ok = boxOpen(nil, bs, 0, &shared); !ok {
		return false
	}
	offset = 0
	var sig edSig
	offset = bytesPop(sig[:], payload, offset)
	tmp := payload[offset:] // Used in sigBytes
	offset = bytesPop(init.current[:], payload, offset)
	offset = bytesPop(init.next[:], payload, offset)
	init.keySeq = binary.BigEndian.Uint64(payload[offset : offset+8])
	offset += 8
	init.seq = binary.BigEndian.Uint64(payload[offset:])
	// Check signature
	var sigBytes []byte
	sigBytes = append(sigBytes, fromBox[:]...)
	sigBytes = append(sigBytes, tmp...)
	return edCheck(sigBytes, &sig, from)
}

/**************
 * sessionAck *
 **************/

type sessionAck struct {
	sessionInit
}

func (ack *sessionAck) encrypt(from *edPriv, to *edPub) ([]byte, error) {
	data, err := ack.sessionInit.encrypt(from, to)
	if err == nil {
		data[0] = sessionTypeAck
	}
	return data, err
}

/*****************
 * sessionBuffer *
 *****************/

type sessionBuffer struct {
	data        []byte
	init        sessionInit
	currentPriv boxPriv     // pairs with init.recv
	nextPriv    boxPriv     // pairs with init.send
	timer       *time.Timer // time.AfterFunc to clean up
}
