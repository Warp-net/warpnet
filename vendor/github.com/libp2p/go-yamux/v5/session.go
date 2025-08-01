package yamux

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pool "github.com/libp2p/go-buffer-pool"
)

// The MemoryManager allows management of memory allocations.
// Memory is allocated:
// 1. When opening / accepting a new stream. This uses the highest priority.
// 2. When trying to increase the stream receive window. This uses a lower priority.
// This is a subset of the libp2p's resource manager ResourceScopeSpan interface.
type MemoryManager interface {
	ReserveMemory(size int, prio uint8) error

	// ReleaseMemory explicitly releases memory previously reserved with ReserveMemory
	ReleaseMemory(size int)

	// Done ends the span and releases associated resources.
	Done()
}

type nullMemoryManagerImpl struct{}

func (n nullMemoryManagerImpl) ReserveMemory(size int, prio uint8) error { return nil }
func (n nullMemoryManagerImpl) ReleaseMemory(size int)                   {}
func (n nullMemoryManagerImpl) Done()                                    {}

var nullMemoryManager = &nullMemoryManagerImpl{}

// Session is used to wrap a reliable ordered connection and to
// multiplex it into multiple streams.
type Session struct {
	rtt int64 // to be accessed atomically, in nanoseconds

	// localGoAway indicates that we should stop
	// accepting futher connections. Must be first for alignment.
	localGoAway int32

	// nextStreamID is the next stream we should
	// send. This depends if we are a client/server.
	nextStreamID uint32

	// config holds our configuration
	config *Config

	// logger is used for our logs
	logger *log.Logger

	// conn is the underlying connection
	conn net.Conn

	// reader is a buffered reader
	reader io.Reader

	newMemoryManager func() (MemoryManager, error)

	// pings is used to track inflight pings
	pingLock   sync.Mutex
	pingID     uint32
	activePing *ping

	// streams maps a stream id to a stream, and inflight has an entry
	// for any outgoing stream that has not yet been established. Both are
	// protected by streamLock.
	numIncomingStreams uint32
	streams            map[uint32]*Stream
	inflight           map[uint32]struct{}
	streamLock         sync.Mutex

	// synCh acts like a semaphore. It is sized to the AcceptBacklog which
	// is assumed to be symmetric between the client and server. This allows
	// the client to avoid exceeding the backlog and instead blocks the open.
	synCh chan struct{}

	// acceptCh is used to pass ready streams to the client
	acceptCh chan *Stream

	// sendCh is used to send messages
	sendCh chan []byte

	// pingCh and pingCh are used to send pings and pongs
	pongCh, pingCh chan uint32

	// recvDoneCh is closed when recv() exits to avoid a race
	// between stream registration and stream shutdown
	recvDoneCh chan struct{}
	// recvErr is the error the receive loop ended with
	recvErr error

	// sendDoneCh is closed when send() exits to avoid a race
	// between returning from a Stream.Write and exiting from the send loop
	// (which may be reading a buffer on-load-from Stream.Write).
	sendDoneCh chan struct{}

	// client is true if we're the client and our stream IDs should be odd.
	client bool

	// shutdown is used to safely close a session
	shutdown     bool
	shutdownErr  error
	shutdownCh   chan struct{}
	shutdownLock sync.Mutex

	// keepaliveTimer is a periodic timer for keepalive messages. It's nil
	// when keepalives are disabled.
	keepaliveLock   sync.Mutex
	keepaliveTimer  *time.Timer
	keepaliveActive bool
}

// newSession is used to construct a new session
func newSession(config *Config, conn net.Conn, client bool, readBuf int, newMemoryManager func() (MemoryManager, error)) *Session {
	var reader io.Reader = conn
	if readBuf > 0 {
		reader = bufio.NewReaderSize(reader, readBuf)
	}
	if newMemoryManager == nil {
		newMemoryManager = func() (MemoryManager, error) { return nullMemoryManager, nil }
	}
	s := &Session{
		config:           config,
		client:           client,
		logger:           log.New(config.LogOutput, "", log.LstdFlags),
		conn:             conn,
		reader:           reader,
		streams:          make(map[uint32]*Stream),
		inflight:         make(map[uint32]struct{}),
		synCh:            make(chan struct{}, config.AcceptBacklog),
		acceptCh:         make(chan *Stream, config.AcceptBacklog),
		sendCh:           make(chan []byte, 64),
		pongCh:           make(chan uint32, config.PingBacklog),
		pingCh:           make(chan uint32),
		recvDoneCh:       make(chan struct{}),
		sendDoneCh:       make(chan struct{}),
		shutdownCh:       make(chan struct{}),
		newMemoryManager: newMemoryManager,
	}
	if client {
		s.nextStreamID = 1
	} else {
		s.nextStreamID = 2
	}
	if config.EnableKeepAlive {
		s.startKeepalive()
	}
	go s.recv()
	go s.send()
	go s.startMeasureRTT()
	return s
}

// IsClosed does a safe check to see if we have shutdown
func (s *Session) IsClosed() bool {
	select {
	case <-s.shutdownCh:
		return true
	default:
		return false
	}
}

// CloseChan returns a read-only channel which is closed as
// soon as the session is closed.
func (s *Session) CloseChan() <-chan struct{} {
	return s.shutdownCh
}

// NumStreams returns the number of currently open streams
func (s *Session) NumStreams() int {
	s.streamLock.Lock()
	num := len(s.streams)
	s.streamLock.Unlock()
	return num
}

// Open is used to create a new stream as a net.Conn
func (s *Session) Open(ctx context.Context) (net.Conn, error) {
	conn, err := s.OpenStream(ctx)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// OpenStream is used to create a new stream
func (s *Session) OpenStream(ctx context.Context) (*Stream, error) {
	if s.IsClosed() {
		return nil, s.shutdownErr
	}

	// Block if we have too many inflight SYNs
	select {
	case s.synCh <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.shutdownCh:
		return nil, s.shutdownErr
	}

	span, err := s.newMemoryManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create resource scope span: %w", err)
	}
	if err := span.ReserveMemory(initialStreamWindow, 255); err != nil {
		return nil, err
	}

GET_ID:
	// Get an ID, and check for stream exhaustion
	id := atomic.LoadUint32(&s.nextStreamID)
	if id >= math.MaxUint32-1 {
		span.Done()
		return nil, ErrStreamsExhausted
	}
	if !atomic.CompareAndSwapUint32(&s.nextStreamID, id, id+2) {
		goto GET_ID
	}

	// Register the stream
	stream := newStream(s, id, streamInit, initialStreamWindow, span)
	s.streamLock.Lock()
	s.streams[id] = stream
	s.inflight[id] = struct{}{}
	s.streamLock.Unlock()

	// Send the window update to create
	if err := stream.sendWindowUpdate(ctx.Done()); err != nil {
		defer span.Done()
		select {
		case <-s.synCh:
		default:
			s.logger.Printf("[ERR] yamux: aborted stream open without inflight syn semaphore")
		}
		return nil, err
	}
	return stream, nil
}

// Accept is used to block until the next available stream
// is ready to be accepted.
func (s *Session) Accept() (net.Conn, error) {
	conn, err := s.AcceptStream()
	if err != nil {
		return nil, err
	}
	return conn, err
}

// AcceptStream is used to block until the next available stream
// is ready to be accepted.
func (s *Session) AcceptStream() (*Stream, error) {
	for {
		select {
		case stream := <-s.acceptCh:
			if err := stream.sendWindowUpdate(nil); err != nil {
				// don't return accept errors.
				s.logger.Printf("[WARN] error sending window update before accepting: %s", err)
				continue
			}
			return stream, nil
		case <-s.shutdownCh:
			return nil, s.shutdownErr
		}
	}
}

// Close is used to close the session and all streams. It doesn't send a GoAway before
// closing the connection.
func (s *Session) Close() error {
	return s.close(ErrSessionShutdown, false, goAwayNormal)
}

// CloseWithError is used to close the session and all streams after sending a GoAway message with errCode.
// Blocks for ConnectionWriteTimeout to write the GoAway message.
//
// The GoAway may not actually be sent depending on the semantics of the underlying net.Conn.
// For TCP connections, it may be dropped depending on LINGER value or if there's unread data in the kernel
// receive buffer.
func (s *Session) CloseWithError(errCode uint32) error {
	return s.close(&GoAwayError{Remote: false, ErrorCode: errCode}, true, errCode)
}

func (s *Session) close(shutdownErr error, sendGoAway bool, errCode uint32) error {
	s.shutdownLock.Lock()
	defer s.shutdownLock.Unlock()

	if s.shutdown {
		return nil
	}
	s.shutdown = true
	if s.shutdownErr == nil {
		s.shutdownErr = shutdownErr
	}
	close(s.shutdownCh)
	s.stopKeepalive()

	// Only send GoAway if we have an error code.
	if sendGoAway && errCode != goAwayNormal {
		// wait for write loop to exit
		// We need to write the current frame completely before sending a goaway.
		// This will wait for at most s.config.ConnectionWriteTimeout
		<-s.sendDoneCh
		ga := s.goAway(errCode)
		if err := s.conn.SetWriteDeadline(time.Now().Add(goAwayWaitTime)); err == nil {
			_, _ = s.conn.Write(ga[:]) // there's nothing we can do on error here
		}
		s.conn.SetWriteDeadline(time.Time{})
	}

	s.conn.Close()
	<-s.sendDoneCh
	<-s.recvDoneCh

	resetErr := shutdownErr
	if _, ok := resetErr.(*GoAwayError); !ok {
		resetErr = fmt.Errorf("%w: connection closed: %w", ErrStreamReset, shutdownErr)
	}
	s.streamLock.Lock()
	defer s.streamLock.Unlock()
	for id, stream := range s.streams {
		stream.forceClose(resetErr)
		delete(s.streams, id)
		stream.memorySpan.Done()
	}
	return nil
}

// GoAway can be used to prevent accepting further
// connections. It does not close the underlying conn.
func (s *Session) GoAway() error {
	return s.sendMsg(s.goAway(goAwayNormal), nil, nil, true)
}

// goAway is used to send a goAway message
func (s *Session) goAway(reason uint32) header {
	atomic.SwapInt32(&s.localGoAway, 1)
	hdr := encode(typeGoAway, 0, 0, reason)
	return hdr
}

func (s *Session) measureRTT() {
	rtt, err := s.Ping()
	if err != nil {
		return
	}
	if !atomic.CompareAndSwapInt64(&s.rtt, 0, rtt.Nanoseconds()) {
		prev := atomic.LoadInt64(&s.rtt)
		smoothedRTT := prev/2 + rtt.Nanoseconds()/2
		atomic.StoreInt64(&s.rtt, smoothedRTT)
	}
}

func (s *Session) startMeasureRTT() {
	s.measureRTT()
	t := time.NewTicker(s.config.MeasureRTTInterval)
	defer t.Stop()
	for {
		select {
		case <-s.CloseChan():
			return
		case <-t.C:
			s.measureRTT()
		}
	}
}

// 0 if we don't yet have a measurement
func (s *Session) getRTT() time.Duration {
	return time.Duration(atomic.LoadInt64(&s.rtt))
}

// Ping is used to measure the RTT response time
func (s *Session) Ping() (dur time.Duration, err error) {
	// Prepare a ping.
	s.pingLock.Lock()
	// If there's an active ping, jump on the bandwagon.
	if activePing := s.activePing; activePing != nil {
		s.pingLock.Unlock()
		return activePing.wait()
	}

	// Ok, our job to send the ping.
	activePing := newPing(s.pingID)
	s.pingID++
	s.activePing = activePing
	s.pingLock.Unlock()

	defer func() {
		// complete ping promise
		activePing.finish(dur, err)

		// Unset it.
		s.pingLock.Lock()
		s.activePing = nil
		s.pingLock.Unlock()
	}()

	// Send the ping request, waiting at most one connection write timeout
	// to flush it.
	timer := time.NewTimer(s.config.ConnectionWriteTimeout)
	defer timer.Stop()
	select {
	case s.pingCh <- activePing.id:
	case <-timer.C:
		return 0, ErrTimeout
	case <-s.shutdownCh:
		return 0, s.shutdownErr
	}

	// The "time" starts once we've actually sent the ping. Otherwise, we'll
	// measure the time it takes to flush the queue as well.
	start := time.Now()

	// Wait for a response, again waiting at most one write timeout.
	if !timer.Stop() {
		<-timer.C
	}
	timer.Reset(s.config.ConnectionWriteTimeout)
	select {
	case <-activePing.pingResponse:
	case <-timer.C:
		return 0, ErrTimeout
	case <-s.shutdownCh:
		return 0, s.shutdownErr
	}

	// Compute the RTT
	return time.Since(start), nil
}

// startKeepalive starts the keepalive process.
func (s *Session) startKeepalive() {
	s.keepaliveLock.Lock()
	defer s.keepaliveLock.Unlock()
	s.keepaliveTimer = time.AfterFunc(s.config.KeepAliveInterval, func() {
		s.keepaliveLock.Lock()
		if s.keepaliveTimer == nil || s.keepaliveActive {
			// keepalives have been stopped or a keepalive is active.
			s.keepaliveLock.Unlock()
			return
		}
		s.keepaliveActive = true
		s.keepaliveLock.Unlock()

		_, err := s.Ping()

		s.keepaliveLock.Lock()
		s.keepaliveActive = false
		if s.keepaliveTimer != nil {
			s.keepaliveTimer.Reset(s.config.KeepAliveInterval)
		}
		s.keepaliveLock.Unlock()

		if err != nil {
			s.logger.Printf("[ERR] yamux: keepalive failed: %v", err)
			s.close(ErrKeepAliveTimeout, false, 0)
		}
	})
}

// stopKeepalive stops the keepalive process.
func (s *Session) stopKeepalive() {
	s.keepaliveLock.Lock()
	defer s.keepaliveLock.Unlock()
	if s.keepaliveTimer != nil {
		s.keepaliveTimer.Stop()
		s.keepaliveTimer = nil
	}
}

func (s *Session) extendKeepalive() {
	s.keepaliveLock.Lock()
	if s.keepaliveTimer != nil && !s.keepaliveActive {
		// Don't stop the timer and drain the channel. This is an
		// AfterFunc, not a normal timer, and any attempts to drain the
		// channel will block forever.
		//
		// Go will stop the timer for us internally anyways. The docs
		// say one must stop the timer before calling reset but that's
		// to ensure that the timer doesn't end up firing immediately
		// after calling Reset.
		s.keepaliveTimer.Reset(s.config.KeepAliveInterval)
	}
	s.keepaliveLock.Unlock()
}

// send sends the header and body.
// If waitForShutDown is true, it will wait for shutdown to complete even if the send loop has exited. This
// ensures accurate error reporting. waitForShutDown should be true for callers other than the recvLoop.
// The recvLoop should set waitForShutdown to false to avoid a deadlock.
// For details see: https://github.com/libp2p/go-yamux/issues/129
// and the test `TestSessionCloseDeadlock`
func (s *Session) sendMsg(hdr header, body []byte, deadline <-chan struct{}, waitForShutDown bool) error {
	select {
	case <-s.shutdownCh:
		return s.shutdownErr
	default:
	}

	select {
	case <-deadline:
		return ErrTimeout
	default:
	}

	// duplicate as we're sending this async.
	buf := pool.Get(headerSize + len(body))
	copy(buf[:headerSize], hdr[:])
	copy(buf[headerSize:], body)

	select {
	case <-s.shutdownCh:
		pool.Put(buf)
		return s.shutdownErr
	case <-s.sendDoneCh:
		pool.Put(buf)
		if waitForShutDown {
			<-s.shutdownCh
			return s.shutdownErr
		}
		return errSendLoopDone
	case s.sendCh <- buf:
		return nil
	case <-deadline:
		pool.Put(buf)
		return ErrTimeout
	}
}

// send is a long running goroutine that sends data
func (s *Session) send() {
	if err := s.sendLoop(); err != nil {
		// If we are shutting down because remote closed the connection, prefer the recvLoop error
		// over the sendLoop error. The receive loop might have error code received in a GoAway frame,
		// which was received just before the TCP RST that closed the sendLoop.
		//
		// If we are closing because of an write error, we use the error from the sendLoop and not the recvLoop.
		// We hold the shutdownLock, close the connection, and wait for the receive loop to finish and
		// use the sendLoop error. Holding the shutdownLock ensures that the recvLoop doesn't trigger connection close
		// but the sendLoop does.
		s.shutdownLock.Lock()
		if s.shutdownErr == nil {
			s.conn.Close()
			<-s.recvDoneCh
			if _, ok := s.recvErr.(*GoAwayError); ok {
				err = s.recvErr
			}
			s.shutdownErr = err
		}
		s.shutdownLock.Unlock()
		s.close(err, false, 0)
	}
}

func (s *Session) sendLoop() (err error) {
	defer func() {
		if rerr := recover(); rerr != nil {
			fmt.Fprintf(os.Stderr, "caught panic: %s\n%s\n", rerr, debug.Stack())
			err = fmt.Errorf("panic in yamux send loop: %s", rerr)
		}
	}()

	defer close(s.sendDoneCh)

	// Extend the write deadline if we've passed the halfway point. This can
	// be expensive so this ensures we only have to do this once every
	// ConnectionWriteTimeout/2 (usually 5s).
	var lastWriteDeadline time.Time
	extendWriteDeadline := func() error {
		now := time.Now()
		// If over half of the deadline has elapsed, extend it.
		if now.Add(s.config.ConnectionWriteTimeout / 2).After(lastWriteDeadline) {
			lastWriteDeadline = now.Add(s.config.ConnectionWriteTimeout)
			return s.conn.SetWriteDeadline(lastWriteDeadline)
		}
		return nil
	}

	writer := s.conn

	// FIXME: https://github.com/libp2p/go-libp2p/issues/644
	// Write coalescing is disabled for now.

	// writer := pool.Writer{W: s.conn}

	// var writeTimeout *time.Timer
	// var writeTimeoutCh <-chan time.Time
	// if s.config.WriteCoalesceDelay > 0 {
	//	writeTimeout = time.NewTimer(s.config.WriteCoalesceDelay)
	//	defer writeTimeout.Stop()

	//	writeTimeoutCh = writeTimeout.C
	// } else {
	//	ch := make(chan time.Time)
	//	close(ch)
	//	writeTimeoutCh = ch
	// }

	for {
		// yield after processing the last message, if we've shutdown.
		// s.sendCh is a buffered channel and Go doesn't guarantee select order.
		select {
		case <-s.shutdownCh:
			return nil
		default:
		}

		var buf []byte
		// Make sure to send any pings & pongs first so they don't get stuck behind writes.
		select {
		case pingID := <-s.pingCh:
			buf = pool.Get(headerSize)
			hdr := encode(typePing, flagSYN, 0, pingID)
			copy(buf, hdr[:])
		case pingID := <-s.pongCh:
			buf = pool.Get(headerSize)
			hdr := encode(typePing, flagACK, 0, pingID)
			copy(buf, hdr[:])
		default:
			// Then send normal data.
			select {
			case buf = <-s.sendCh:
			case pingID := <-s.pingCh:
				buf = pool.Get(headerSize)
				hdr := encode(typePing, flagSYN, 0, pingID)
				copy(buf, hdr[:])
			case pingID := <-s.pongCh:
				buf = pool.Get(headerSize)
				hdr := encode(typePing, flagACK, 0, pingID)
				copy(buf, hdr[:])
			case <-s.shutdownCh:
				return nil
				// default:
				//	select {
				//	case buf = <-s.sendCh:
				//	case <-s.shutdownCh:
				//		return nil
				//	case <-writeTimeoutCh:
				//		if err := writer.Flush(); err != nil {
				//			if os.IsTimeout(err) {
				//				err = ErrConnectionWriteTimeout
				//			}
				//			return err
				//		}

				//		select {
				//		case buf = <-s.sendCh:
				//		case <-s.shutdownCh:
				//			return nil
				//		}

				//		if writeTimeout != nil {
				//			writeTimeout.Reset(s.config.WriteCoalesceDelay)
				//		}
				//	}
			}
		}

		if err := extendWriteDeadline(); err != nil {
			pool.Put(buf)
			return err
		}

		_, err := writer.Write(buf)
		pool.Put(buf)

		if err != nil {
			if os.IsTimeout(err) {
				err = ErrConnectionWriteTimeout
			}
			return err
		}
	}
}

// recv is a long running goroutine that accepts new data
func (s *Session) recv() {
	if err := s.recvLoop(); err != nil {
		s.close(err, false, 0)
	}
}

// Ensure that the index of the handler (typeData/typeWindowUpdate/etc) matches the message type
var (
	handlers = []func(*Session, header) error{
		typeData:         (*Session).handleStreamMessage,
		typeWindowUpdate: (*Session).handleStreamMessage,
		typePing:         (*Session).handlePing,
		typeGoAway:       (*Session).handleGoAway,
	}
)

// recvLoop continues to receive data until a fatal error is encountered
func (s *Session) recvLoop() (err error) {
	defer func() {
		if rerr := recover(); rerr != nil {
			fmt.Fprintf(os.Stderr, "caught panic: %s\n%s\n", rerr, debug.Stack())
			err = fmt.Errorf("panic in yamux receive loop: %s", rerr)
		}
	}()
	defer func() {
		s.recvErr = err
		close(s.recvDoneCh)
	}()
	var hdr header
	for {
		// fmt.Printf("ReadFull from %#v\n", s.reader)
		// Read the header
		if _, err := io.ReadFull(s.reader, hdr[:]); err != nil {
			if err != io.EOF && !strings.Contains(err.Error(), "closed") && !strings.Contains(err.Error(), "reset by peer") {
				s.logger.Printf("[ERR] yamux: Failed to read header: %v", err)
			}
			return err
		}

		// Reset the keepalive timer every time we receive data.
		// There's no reason to keepalive if we're active. Worse, if the
		// peer is busy sending us stuff, the pong might get stuck
		// behind a bunch of data.
		s.extendKeepalive()

		// Verify the version
		if hdr.Version() != protoVersion {
			s.logger.Printf("[ERR] yamux: Invalid protocol version: %d", hdr.Version())
			return ErrInvalidVersion
		}

		mt := hdr.MsgType()
		if mt < typeData || mt > typeGoAway {
			return ErrInvalidMsgType
		}

		if err := handlers[mt](s, hdr); err != nil {
			return err
		}
	}
}

// handleStreamMessage handles either a data or window update frame
func (s *Session) handleStreamMessage(hdr header) error {
	// Check for a new stream creation
	id := hdr.StreamID()
	flags := hdr.Flags()
	if flags&flagSYN == flagSYN {
		if err := s.incomingStream(id); err != nil {
			return err
		}
	}

	// Get the stream
	s.streamLock.Lock()
	stream := s.streams[id]
	s.streamLock.Unlock()

	// If we do not have a stream, likely we sent a RST and/or closed the stream for reading.
	if stream == nil {
		// Drain any data on the wire
		if hdr.MsgType() == typeData && hdr.Length() > 0 {
			if _, err := io.CopyN(io.Discard, s.reader, int64(hdr.Length())); err != nil {
				return nil
			}
		}
		return nil
	}

	// Check if this is a window update
	if hdr.MsgType() == typeWindowUpdate {
		stream.incrSendWindow(hdr, flags)
		return nil
	}

	// Read the new data
	if err := stream.readData(hdr, flags, s.reader); err != nil {
		if sendErr := s.sendMsg(s.goAway(goAwayProtoErr), nil, nil, false); sendErr != nil && sendErr != errSendLoopDone {
			s.logger.Printf("[WARN] yamux: failed to send go away: %v", sendErr)
		}
		return err
	}
	return nil
}

// handlePing is invoked for a typePing frame
func (s *Session) handlePing(hdr header) error {
	flags := hdr.Flags()
	pingID := hdr.Length()

	// Check if this is a query, respond back in a separate context so we
	// don't interfere with the receiving thread blocking for the write.
	if flags&flagSYN == flagSYN {
		select {
		case s.pongCh <- pingID:
		default:
			s.logger.Printf("[WARN] yamux: dropped ping reply")
		}
		return nil
	}

	// Handle a response
	s.pingLock.Lock()
	// If we have an active ping, and this is a response to that active
	// ping, complete the ping.
	if s.activePing != nil && s.activePing.id == pingID {
		// Don't assume that the peer won't send multiple responses for
		// the same ping.
		select {
		case s.activePing.pingResponse <- struct{}{}:
		default:
		}
	}
	s.pingLock.Unlock()
	return nil
}

// handleGoAway is invokde for a typeGoAway frame
func (s *Session) handleGoAway(hdr header) error {
	code := hdr.Length()
	switch code {
	case goAwayNormal:
		return ErrRemoteGoAway
	case goAwayProtoErr:
		s.logger.Printf("[ERR] yamux: received protocol error go away")
	case goAwayInternalErr:
		s.logger.Printf("[ERR] yamux: received internal error go away")
	default:
		s.logger.Printf("[ERR] yamux: received go away with error code: %d", code)
	}
	return &GoAwayError{Remote: true, ErrorCode: code}
}

// incomingStream is used to create a new incoming stream
func (s *Session) incomingStream(id uint32) error {
	if s.client != (id%2 == 0) {
		s.logger.Printf("[ERR] yamux: both endpoints are clients")
		return fmt.Errorf("both yamux endpoints are clients")
	}
	// Reject immediately if we are doing a go away
	if atomic.LoadInt32(&s.localGoAway) == 1 {
		hdr := encode(typeWindowUpdate, flagRST, id, 0)
		return s.sendMsg(hdr, nil, nil, false)
	}

	// Allocate a new stream
	span, err := s.newMemoryManager()
	if err != nil {
		return fmt.Errorf("failed to create resource span: %w", err)
	}
	if err := span.ReserveMemory(initialStreamWindow, 255); err != nil {
		return err
	}
	stream := newStream(s, id, streamSYNReceived, initialStreamWindow, span)

	s.streamLock.Lock()
	defer s.streamLock.Unlock()

	// Check if stream already exists
	if _, ok := s.streams[id]; ok {
		s.logger.Printf("[ERR] yamux: duplicate stream declared")
		if sendErr := s.sendMsg(s.goAway(goAwayProtoErr), nil, nil, false); sendErr != nil && sendErr != errSendLoopDone {
			s.logger.Printf("[WARN] yamux: failed to send go away: %v", sendErr)
		}
		span.Done()
		return ErrDuplicateStream
	}

	if s.numIncomingStreams >= s.config.MaxIncomingStreams {
		// too many active streams at the same time
		s.logger.Printf("[WARN] yamux: MaxIncomingStreams exceeded, forcing stream reset")
		defer span.Done()
		hdr := encode(typeWindowUpdate, flagRST, id, 0)
		return s.sendMsg(hdr, nil, nil, false)
	}

	s.numIncomingStreams++
	// Register the stream
	s.streams[id] = stream

	// Check if we've exceeded the backlog
	select {
	case s.acceptCh <- stream:
		return nil
	default:
		// Backlog exceeded! RST the stream
		defer span.Done()
		s.logger.Printf("[WARN] yamux: backlog exceeded, forcing stream reset")
		s.deleteStream(id)
		hdr := encode(typeWindowUpdate, flagRST, id, 0)
		return s.sendMsg(hdr, nil, nil, false)
	}
}

// closeStream is used to close a stream once both sides have
// issued a close. If there was an in-flight SYN and the stream
// was not yet established, then this will give the credit back.
func (s *Session) closeStream(id uint32) {
	s.streamLock.Lock()
	defer s.streamLock.Unlock()
	if _, ok := s.inflight[id]; ok {
		select {
		case <-s.synCh:
		default:
			s.logger.Printf("[ERR] yamux: SYN tracking out of sync")
		}
		delete(s.inflight, id)
	}
	s.deleteStream(id)
}

func (s *Session) deleteStream(id uint32) {
	str, ok := s.streams[id]
	if !ok {
		return
	}
	if s.client == (id%2 == 0) {
		if s.numIncomingStreams == 0 {
			s.logger.Printf("[ERR] yamux: numIncomingStreams underflow")
			// prevent the creation of any new streams
			s.numIncomingStreams = math.MaxUint32
		} else {
			s.numIncomingStreams--
		}
	}
	delete(s.streams, id)
	str.memorySpan.Done()
}

// establishStream is used to mark a stream that was in the
// SYN Sent state as established.
func (s *Session) establishStream(id uint32) {
	s.streamLock.Lock()
	if _, ok := s.inflight[id]; ok {
		delete(s.inflight, id)
	} else {
		s.logger.Printf("[ERR] yamux: established stream without inflight SYN (no tracking entry)")
	}
	select {
	case <-s.synCh:
	default:
		s.logger.Printf("[ERR] yamux: established stream without inflight SYN (didn't have semaphore)")
	}
	s.streamLock.Unlock()
}
