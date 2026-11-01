// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package srtp

import (
	"errors"
	"net"
	"slices"
	"time"

	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/transport/v5/packetio"
)

const defaultSessionSRTCPReplayProtectionWindow = 64

// SessionSRTCP implements io.ReadWriteCloser and provides a bi-directional SRTCP session
// SRTCP itself does not have a design like this, but it is common in most applications
// for local/remote to each have their own keying material. This provides those patterns
// instead of making everyone re-implement.
type SessionSRTCP struct {
	session
	writeStream *WriteStreamSRTCP

	// rawPackets is a scratch slice used in decrypt to avoid allocations, and should only
	// be used by decrypt() and not aliased or otherwise returned to a caller.
	rawPackets []rtcp.RawPacket

	// destinationSSRCs is a scratch slice used internally in decrypt to avoid allocations.
	destinationSSRCs []uint32

	// compoundSSRCs is a scratch slice used internally in decrypt to avoid allocations. It
	// is lazily populated upon receiving an unknown RTCP type with the SSRCs of all known
	// types in a compound RTCP packet.
	compoundSSRCs []uint32
}

// NewSessionSRTCP creates a SRTCP session using conn as the underlying transport.
func NewSessionSRTCP(conn net.Conn, config *Config) (*SessionSRTCP, error) { //nolint:dupl
	if config == nil {
		return nil, errNoConfig
	} else if conn == nil {
		return nil, errNoConn
	}

	loggerFactory := config.LoggerFactory
	if loggerFactory == nil {
		loggerFactory = logging.NewDefaultLoggerFactory()
	}

	localOpts := append(
		[]ContextOption{},
		config.LocalOptions...,
	)
	remoteOpts := append(
		[]ContextOption{
			// Default options
			SRTCPReplayProtection(defaultSessionSRTCPReplayProtectionWindow),
		},
		config.RemoteOptions...,
	)

	srtcpSession := &SessionSRTCP{
		session: session{
			nextConn:            conn,
			localOptions:        localOpts,
			remoteOptions:       remoteOpts,
			readStreams:         map[uint32]readStream{},
			newStream:           make(chan readStream),
			acceptStreamTimeout: config.AcceptStreamTimeout,
			started:             make(chan any),
			closed:              make(chan any),
			bufferFactory:       config.BufferFactory,
			log:                 loggerFactory.NewLogger("srtp"),
		},
	}
	srtcpSession.writeStream = &WriteStreamSRTCP{srtcpSession}

	err := srtcpSession.session.start(
		config.Keys.LocalMasterKey, config.Keys.LocalMasterSalt,
		config.Keys.RemoteMasterKey, config.Keys.RemoteMasterSalt,
		config.Profile,
		srtcpSession,
	)
	if err != nil {
		return nil, err
	}

	return srtcpSession, nil
}

// OpenWriteStream returns the global write stream for the Session.
func (s *SessionSRTCP) OpenWriteStream() (*WriteStreamSRTCP, error) {
	return s.writeStream, nil
}

// OpenReadStream opens a read stream for the given SSRC, it can be used
// if you want a certain SSRC, but don't want to wait for AcceptStream.
func (s *SessionSRTCP) OpenReadStream(ssrc uint32) (*ReadStreamSRTCP, error) {
	r, _ := s.session.getOrCreateReadStream(ssrc, s, newReadStreamSRTCP)

	if readStream, ok := r.(*ReadStreamSRTCP); ok {
		return readStream, nil
	}

	return nil, errFailedTypeAssertion
}

// AcceptStream returns a stream to handle RTCP for a single SSRC.
func (s *SessionSRTCP) AcceptStream() (*ReadStreamSRTCP, uint32, error) {
	stream, ok := <-s.newStream
	if !ok {
		return nil, 0, errStreamAlreadyClosed
	}

	readStream, ok := stream.(*ReadStreamSRTCP)
	if !ok {
		return nil, 0, errFailedTypeAssertion
	}

	return readStream, stream.GetSSRC(), nil
}

// Close ends the session.
func (s *SessionSRTCP) Close() error {
	return s.session.close()
}

// Private

func (s *SessionSRTCP) write(buf []byte) (int, error) {
	if _, ok := <-s.session.started; ok {
		return 0, errStartedChannelUsedIncorrectly
	}

	pbuf, ok := bufferpool.Get().(*[]byte)
	if !ok {
		return 0, errStartedChannelUsedIncorrectly
	}
	defer bufferpool.Put(pbuf)

	s.session.localContextMutex.Lock()
	encrypted, err := s.localContext.EncryptRTCP(*pbuf, buf, nil)
	s.session.localContextMutex.Unlock()

	if err != nil {
		return 0, err
	}

	return s.session.nextConn.Write(encrypted)
}

func (s *SessionSRTCP) setWriteDeadline(t time.Time) error {
	return s.session.nextConn.SetWriteDeadline(t)
}

// dedupeSSRCs sorts and removes duplicate SSRCs in place, preserving capacity.
func dedupeSSRCs(ssrcs []uint32) []uint32 {
	// Note: sort + compact was chosen over the more obvious choice
	// of using a map to dedupe for two reasons:
	//  - It is more performant on small slices (length under ~100,
	//    which will be almost all inputs)
	//  - To avoid allocations, the map would need to be reused, and
	//    if it ever grew large then subsequent small dedupes would
	//    have degraded performance due to needing to clear a larger
	//    map.
	if len(ssrcs) < 2 {
		return ssrcs
	}
	slices.Sort(ssrcs)

	return slices.Compact(ssrcs)
}

// appendDestinationSSRCs appends destinations from packets whose SSRCs can be parsed.
func appendDestinationSSRCs(dst []uint32, pkts []rtcp.RawPacket) []uint32 {
	for _, pkt := range pkts {
		ssrcCount := len(dst)
		var err error
		dst, err = pkt.ParseDestinationSSRC(dst)
		if err != nil {
			// A failed parse may have appended partial destinations.
			dst = dst[:ssrcCount]
		}
	}

	dst = dedupeSSRCs(dst)

	return dst
}

// decrypt is called synchronously by the session's read loop and must not
// be called concurrently. It reuses scratch buffers that are not protected
// by a mutex.
//
//nolint:cyclop
func (s *SessionSRTCP) decrypt(buf []byte, attrs packetio.Attributes) error {
	s.session.remoteContextMutex.Lock()
	decrypted, err := s.remoteContext.DecryptRTCP(buf, buf, nil)
	s.session.remoteContextMutex.Unlock()
	if err != nil {
		return err
	}

	s.rawPackets, err = rtcp.AppendRawPackets(s.rawPackets[:0], decrypted)
	if err != nil {
		return err
	}
	pkts := s.rawPackets

	compoundSSRCsComputed := false
	var parseErrs error
	for _, pkt := range pkts {
		s.destinationSSRCs, err = pkt.ParseDestinationSSRC(s.destinationSSRCs[:0])
		if err != nil {
			// Skip packets whose destination SSRCs could not be parsed so
			// the remaining packets in the compound are still forwarded.
			parseErrs = errors.Join(parseErrs, err)

			continue
		}

		destinations := dedupeSSRCs(s.destinationSSRCs)
		if len(destinations) == 0 {
			// Packets without a destination of their own (e.g. unknown packet
			// types parsed as rtcp.RawPacket) are delivered to every stream the
			// compound packet is addressed to instead of being dropped.
			if !compoundSSRCsComputed {
				s.compoundSSRCs = appendDestinationSSRCs(s.compoundSSRCs[:0], pkts)
				compoundSSRCsComputed = true
			}
			destinations = s.compoundSSRCs
		}

		for _, ssrc := range destinations {
			r, isNew := s.session.getOrCreateReadStream(ssrc, s, newReadStreamSRTCP)
			if r == nil {
				return nil // Session has been closed
			} else if isNew {
				if !s.session.acceptStreamTimeout.IsZero() {
					_ = s.session.nextConn.SetReadDeadline(time.Time{})
				}
				s.session.newStream <- r // Notify AcceptStream
			}

			readStream, ok := r.(*ReadStreamSRTCP)
			if !ok {
				return errFailedTypeAssertion
			}

			_, err = readStream.write(pkt, attrs)
			if err != nil {
				return err
			}
		}
	}

	return parseErrs
}

// UpdateKey resets packet state with fresh keys.
func (s *SessionSRTCP) UpdateKey(keys SessionKeys, profile ProtectionProfile) error {
	return s.session.updateKey(keys, profile)
}
