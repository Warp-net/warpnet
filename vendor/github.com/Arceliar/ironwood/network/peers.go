package network

import (
	"bufio"
	"encoding/binary"
	"io"

	//"math"
	"net"
	"time"

	"github.com/Arceliar/phony"

	"github.com/Arceliar/ironwood/types"
)

// TODO? copy relevant config info to structs here, to avoid needing to dereference pointers all the way back to the core

type peerPort uint64

type peers struct {
	phony.Inbox // Used to create/remove peers
	core        *core
	ports       map[peerPort]struct{}
	peers       map[publicKey]map[*peer]struct{}
	order       uint64 // global counter for (*peer).order
}

func (ps *peers) init(c *core) {
	ps.core = c
	ps.ports = make(map[peerPort]struct{})
	ps.peers = make(map[publicKey]map[*peer]struct{})
}

func (ps *peers) addPeer(key publicKey, conn net.Conn, prio uint8) (*peer, error) {
	var p *peer
	var err error
	ps.core.pconn.closeMutex.Lock()
	defer ps.core.pconn.closeMutex.Unlock()
	select {
	case <-ps.core.pconn.closed:
		return nil, types.ErrClosed
	default:
	}
	phony.Block(ps, func() {
		var port peerPort
		if keyPeers, isIn := ps.peers[key]; isIn {
			for p := range keyPeers {
				port = p.port
				break
			}
		} else {
			// Allocate port
			for idx := 1; ; idx++ { // skip 0
				if _, isIn := ps.ports[peerPort(idx)]; isIn {
					continue
				}
				port = peerPort(idx)
				break
			}
			ps.ports[port] = struct{}{}
			ps.peers[key] = make(map[*peer]struct{})
		}
		p = new(peer)
		p.peers = ps
		p.conn = conn
		p.done = make(chan struct{})
		p.key = key
		p.port = port
		p.prio = prio
		p.monitor.peer = p
		p.monitor.pDelay = ps.core.config.peerTimeout // It doesn't make sense to start the ping delay any shorter than this
		p.writer.peer = p
		p.writer.wbuf = bufio.NewWriter(p.conn)
		p.order = ps.order
		ps.order++
		ps.peers[p.key][p] = struct{}{}
	})
	return p, err
}

func (ps *peers) removePeer(p *peer) error {
	var err error
	phony.Block(ps, func() {
		kps := ps.peers[p.key]
		if _, isIn := kps[p]; !isIn {
			err = types.ErrPeerNotFound
		} else {
			delete(kps, p)
			if len(kps) == 0 {
				delete(ps.peers, p.key)
				delete(ps.ports, p.port)
			}
		}
	})
	return err
}

type peer struct {
	phony.Inbox // Only used to process or send some protocol traffic
	peers       *peers
	conn        net.Conn
	done        chan struct{}
	key         publicKey
	port        peerPort
	prio        uint8
	queue       packetQueue
	order       uint64 // order in which peers were connected (relative uptime)
	monitor     peerMonitor
	writer      peerWriter
	ready       bool      // is the writer ready for traffic?
	srst        time.Time // sigReq send time
	srrt        time.Time // sigRes receive time
}

type peerMonitor struct {
	phony.Inbox
	peer           *peer
	keepAliveTimer *time.Timer
	pDelay         time.Duration
	deadlined      bool
}

func (m *peerMonitor) keepAlive() {
	m.Act(nil, func() {
		select {
		case <-m.peer.done:
			return
		default:
		}
		m.peer.writer.Act(m, func() {
			m.peer.writer._write([]byte{0x01, byte(wireKeepAlive)}, wireKeepAlive)
		})
	})
}

func (m *peerMonitor) sent(pType wirePacketType) {
	m.Act(&m.peer.writer, func() {
		if m.keepAliveTimer != nil {
			// We're sending a packet, so we definitely don't need to send a keepalive after this
			m.keepAliveTimer.Stop()
			m.keepAliveTimer = nil
		}
		switch {
		case m.deadlined:
			return
		case pType == wireDummy:
		case pType == wireKeepAlive:
		default:
			// We're sending non-keepalive traffic
			// This means we expect some kind of acknowledgement (at least a keepalive)
			// Set a read deadline for that (and make a note that we did so)
			m.peer.conn.SetReadDeadline(time.Now().Add(m.peer.peers.core.config.peerTimeout))
			m.deadlined = true
		}
	})
}

func (m *peerMonitor) recv(pType wirePacketType) {
	m.Act(nil, func() {
		m.peer.conn.SetReadDeadline(time.Time{})
		m.deadlined = false
		switch {
		case m.keepAliveTimer != nil:
		case pType == wireDummy:
		case pType == wireKeepAlive:
		default:
			// We just received non-keepalive traffic
			// The other side is expecting some kind of response, at least a keepalive
			// We set a timer to trigger a response later, if we don't send any traffic in the mean time
			select {
			case <-m.peer.done:
			default:
				m.keepAliveTimer = time.AfterFunc(m.peer.peers.core.config.peerKeepAliveDelay, m.keepAlive)
			}
		}
	})
}

type peerWriter struct {
	phony.Inbox
	peer *peer
	wbuf *bufio.Writer
	seq  uint64
}

func (w *peerWriter) _write(bs []byte, pType wirePacketType) {
	w.peer.monitor.sent(pType)
	// _, _ = w.peer.conn.Write(bs)
	_, _ = w.wbuf.Write(bs)
	w.seq++
	seq := w.seq
	w.Act(nil, func() {
		if seq == w.seq {
			w.peer.pop() // Ask for more traffic to send
		}
	})
}

func (w *peerWriter) sendPacket(pType wirePacketType, data wireEncodeable, done func()) {
	w.Act(nil, func() {
		bufSize := uint64(data.size() + 1)
		if bufSize > w.peer.peers.core.config.peerMaxMessageSize {
			return
		}
		writeBuf := allocBytes(0)
		defer freeBytes(writeBuf)
		// The +1 is from 1 byte for the pType
		writeBuf = binary.AppendUvarint(writeBuf[:], bufSize)
		var err error
		writeBuf, err = wireEncode(writeBuf, byte(pType), data)
		if err != nil {
			panic(err)
		}
		w._write(writeBuf, pType)
		switch tr := data.(type) {
		case *traffic:
			freeTraffic(tr)
		default:
			// Not a special case, don't free anything
		}
		if done != nil {
			w.peer.Act(w, done)
		}
	})
}

func (p *peer) handler() error {
	defer func() {
		p.peers.core.router.removePeer(nil, p)
	}()
	defer p.monitor.Act(nil, func() {
		if p.monitor.keepAliveTimer != nil {
			p.monitor.keepAliveTimer.Stop()
			p.monitor.keepAliveTimer = nil
		}
	})
	defer close(p.done)
	p.conn.SetDeadline(time.Time{})
	// Add peer to the router, to kick off protocol exchanges
	p.peers.core.router.addPeer(p, p)
	// Now allocate buffers and start reading / handling packets...
	rbuf := bufio.NewReader(p.conn)
	for {
		var usize uint64
		var err error
		if usize, err = binary.ReadUvarint(rbuf); err != nil {
			return err
		}
		if usize > p.peers.core.config.peerMaxMessageSize {
			return types.ErrOversizedMessage
		}
		size := int(usize)
		bs := allocBytes(size)
		if _, err = io.ReadFull(rbuf, bs); err != nil {
			freeBytes(bs)
			return err
		}
		phony.Block(p, func() {
			err = p._handlePacket(bs)
		})
		freeBytes(bs)
		if err != nil {
			return err
		}
	}
}

func (p *peer) _handlePacket(bs []byte) error {
	// Note: this function should be non-blocking.
	// Individual handlers should send actor messages as needed.
	if len(bs) == 0 {
		return types.ErrEmptyMessage
	}
	pType := wirePacketType(bs[0])
	p.monitor.recv(pType)
	switch pType {
	case wireDummy:
		return nil
	case wireKeepAlive:
		return nil
	case wireProtoSigReq:
		return p._handleSigReq(bs[1:])
	case wireProtoSigRes:
		return p._handleSigRes(bs[1:])
	case wireProtoAnnounce:
		return p._handleAnnounce(bs[1:])
	case wireProtoBloomFilter:
		return p._handleBloom(bs[1:])
	case wireProtoPathLookup:
		return p._handlePathLookup(bs[1:])
	case wireProtoPathNotify:
		return p._handlePathNotify(bs[1:])
	case wireProtoPathBroken:
		return p._handlePathBroken(bs[1:])
	case wireTraffic:
		return p._handleTraffic(bs[1:])
	default:
		return types.ErrUnrecognizedMessage
	}
}

func (p *peer) sendDirect(from phony.Actor, pType wirePacketType, data wireEncodeable, done func()) {
	p.Act(from, func() {
		p.writer.sendPacket(pType, data, done)
	})
}

func (p *peer) _handleSigReq(bs []byte) error {
	req := new(routerSigReq)
	if err := req.decode(bs); err != nil {
		return err
	}
	p.peers.core.router.handleRequest(p, p, req)
	return nil
}

func (p *peer) sendSigReq(from phony.Actor, req *routerSigReq) {
	p.sendDirect(from, wireProtoSigReq, req, func() {
		p.srst = time.Now()
	})
}

func (p *peer) _handleSigRes(bs []byte) error {
	res := new(routerSigRes)
	if err := res.decode(bs); err != nil {
		return err
	}
	if !res.check(p.peers.core.crypto.publicKey, p.key) {
		return types.ErrBadMessage
	}
	p.srrt = time.Now()
	rtt := p.srrt.Sub(p.srst)
	p.peers.core.router.handleResponse(p, p, res, rtt)
	return nil
}

func (p *peer) sendSigRes(from phony.Actor, res *routerSigRes) {
	p.sendDirect(from, wireProtoSigRes, res, nil)
}

func (p *peer) _handleAnnounce(bs []byte) error {
	ann := new(routerAnnounce)
	if err := ann.decode(bs); err != nil {
		return err
	}
	if !ann.check() {
		return types.ErrBadMessage
	}
	p.peers.core.router.handleAnnounce(p, p, ann)
	return nil
}

func (p *peer) sendAnnounce(from phony.Actor, ann *routerAnnounce) {
	p.sendDirect(from, wireProtoAnnounce, ann, nil)
}

func (p *peer) _handleBloom(bs []byte) error {
	b := newBloom()
	if err := b.decode(bs); err != nil {
		return err
	}
	p.peers.core.router.blooms.handleBloom(p, b)
	return nil
}

func (p *peer) sendBloom(from phony.Actor, b *bloom) {
	p.sendDirect(from, wireProtoBloomFilter, b, nil)
}

func (p *peer) _handlePathLookup(bs []byte) error {
	lookup := new(pathLookup)
	if err := lookup.decode(bs); err != nil {
		return err
	}
	p.peers.core.router.pathfinder.handleLookup(p, lookup)
	return nil
}

func (p *peer) _handlePathNotify(bs []byte) error {
	notify := new(pathNotify)
	if err := notify.decode(bs); err != nil {
		return err
	}
	p.peers.core.router.pathfinder.handleNotify(p, notify)
	return nil
}

func (p *peer) sendPathNotify(from phony.Actor, notify *pathNotify) {
	//p.sendDirect(from, wireProtoPathNotify, notify)
	p.sendQueued(from, notify)
}

func (p *peer) _handlePathBroken(bs []byte) error {
	broken := new(pathBroken)
	if err := broken.decode(bs); err != nil {
		return err
	}
	p.peers.core.router.pathfinder.handleBroken(p, broken)
	return nil
}

func (p *peer) sendPathBroken(from phony.Actor, broken *pathBroken) {
	//p.sendDirect(from, wireProtoPathBroken, broken)
	p.sendQueued(from, broken)
}

func (p *peer) _handleTraffic(bs []byte) error {
	tr := allocTraffic()
	if err := tr.decode(bs); err != nil {
		return err // This is just to check that it unmarshals correctly
	}
	p.peers.core.router.handleTraffic(p, tr)
	return nil
}

func (p *peer) sendTraffic(from phony.Actor, tr *traffic) {
	p.sendQueued(from, tr)
}

func (p *peer) sendQueued(from phony.Actor, packet pqPacket) {
	p.Act(from, func() {
		p._push(packet)
	})
}

func (p *peer) _push(packet pqPacket) {
	if p.ready {
		p.writer.sendPacket(packet.wireType(), packet, nil)
		p.ready = false
		return
	}
	// We're waiting, so queue the packet up for later
	if info, ok := p.queue.peek(); ok && time.Since(info.time) > 25*time.Millisecond {
		// The queue already has a significant delay
		// Drop the oldest packet from the larget queue to make room
		p.queue.drop()
	}
	// Add the packet to the queue
	p.queue.push(packet)
}

func (p *peer) pop() {
	p.Act(nil, func() {
		if info, ok := p.queue.pop(); ok {
			p.writer.sendPacket(info.packet.wireType(), info.packet, nil)
		} else {
			p.ready = true
			p.writer.Act(nil, func() {
				p.writer.wbuf.Flush()
			})
		}
	})
}
