// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package nack

import (
	"math"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/cc"
	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/interceptor/internal/rtpbuffer"
	"github.com/pion/interceptor/internal/validation"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// ResponderInterceptorFactory is a interceptor.Factory for a ResponderInterceptor.
type ResponderInterceptorFactory struct {
	opts []ResponderOption
}

// NewInterceptor constructs a new ResponderInterceptor.
func (r *ResponderInterceptorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	responderInterceptor := &ResponderInterceptor{
		streamsFilter: streamSupportNack,
		size:          1024,
		streams:       map[uint32]*localStream{},
		now:           time.Now,
	}

	for _, opt := range r.opts {
		if err := opt(responderInterceptor); err != nil {
			return nil, err
		}
	}

	if responderInterceptor.loggerFactory == nil {
		responderInterceptor.loggerFactory = logging.NewDefaultLoggerFactory()
	}
	if responderInterceptor.log == nil {
		responderInterceptor.log = responderInterceptor.loggerFactory.NewLogger("nack_responder")
	}
	if responderInterceptor.packetFactory == nil {
		responderInterceptor.packetFactory = rtpbuffer.NewPacketFactoryCopy()
	}

	if err := validation.IsPowerOfTwo(responderInterceptor.size); err != nil {
		return nil, err
	}

	return responderInterceptor, nil
}

// ResponderInterceptor responds to nack feedback messages.
type ResponderInterceptor struct {
	interceptor.NoOp
	streamsFilter func(info *interceptor.StreamInfo) bool
	size          uint16
	log           logging.LeveledLogger
	loggerFactory logging.LoggerFactory
	packetFactory rtpbuffer.PacketFactory
	now           func() time.Time
	rate          retransmissionRate

	streams   map[uint32]*localStream
	streamsMu sync.Mutex
}

type localStream struct {
	rtpBuffer      *rtpbuffer.RTPBuffer
	rtpBufferMutex sync.RWMutex
	rtpWriter      interceptor.RTPWriter
	rtt            time.Duration
	rttSamples     int64
}

// NewResponderInterceptor returns a new ResponderInterceptorFactor.
func NewResponderInterceptor(opts ...ResponderOption) (*ResponderInterceptorFactory, error) {
	return &ResponderInterceptorFactory{opts}, nil
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once per sender/receiver, however this might
// change in the future. The returned method will be called once per packet batch.
func (n *ResponderInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}

		if attr == nil {
			attr = make(interceptor.Attributes)
		}
		pkts, err := attr.GetRTCPPackets(b[:i])
		if err != nil {
			return 0, nil, err
		}
		for _, rtcpPacket := range pkts {
			switch report := rtcpPacket.(type) {
			case *rtcp.ReceiverReport:
				n.updateRTT(report.Reports)
			case *rtcp.SenderReport:
				n.updateRTT(report.Reports)
			}
			nack, ok := rtcpPacket.(*rtcp.TransportLayerNack)
			if !ok {
				continue
			}

			targetBitrate, _ := attr.Get(cc.TargetBitrateKey{}).(func() int)
			go n.resendPackets(nack, targetBitrate)
		}

		return i, attr, err
	})
}

// BindLocalStream lets you modify any outgoing RTP packets. It is called once for per LocalStream.
// The returned method will be called once per rtp packet.
func (n *ResponderInterceptor) BindLocalStream(
	info *interceptor.StreamInfo, writer interceptor.RTPWriter,
) interceptor.RTPWriter {
	if !n.streamsFilter(info) {
		return writer
	}

	// error is already checked in NewGeneratorInterceptor
	rtpBuffer, _ := rtpbuffer.NewRTPBuffer(n.size)
	stream := &localStream{
		rtpBuffer: rtpBuffer,
		rtpWriter: writer,
	}
	n.streamsMu.Lock()
	n.streams[info.SSRC] = stream
	n.streamsMu.Unlock()

	return interceptor.RTPWriterFunc(
		func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
			// If this packet doesn't belong to the main SSRC, do not add it to rtpBuffer
			if header.SSRC != info.SSRC {
				return writer.Write(header, payload, attributes)
			}

			pkt, err := n.packetFactory.NewPacket(header, payload, info.SSRCRetransmission, info.PayloadTypeRetransmission)
			if err != nil {
				return 0, err
			}

			stream.rtpBufferMutex.Lock()
			stream.rtpBuffer.Add(pkt)
			stream.rtpBufferMutex.Unlock()

			return writer.Write(header, payload, attributes)
		},
	)
}

// UnbindLocalStream is called when the Stream is removed. It can be used to clean up any data related to that track.
func (n *ResponderInterceptor) UnbindLocalStream(info *interceptor.StreamInfo) {
	n.streamsMu.Lock()
	stream, ok := n.streams[info.SSRC]
	delete(n.streams, info.SSRC)
	n.streamsMu.Unlock()

	if ok {
		stream.rtpBufferMutex.Lock()
		stream.rtpBuffer.Clear()
		stream.rtpBufferMutex.Unlock()
	}
}

// Close releases all resources held by the ResponderInterceptor.
func (n *ResponderInterceptor) Close() error {
	n.streamsMu.Lock()
	streams := n.streams
	n.streams = map[uint32]*localStream{}
	n.streamsMu.Unlock()

	for _, stream := range streams {
		stream.rtpBufferMutex.Lock()
		stream.rtpBuffer.Clear()
		stream.rtpBufferMutex.Unlock()
	}

	return nil
}

func (n *ResponderInterceptor) resendPackets(nack *rtcp.TransportLayerNack, targetBitrate func() int) {
	n.streamsMu.Lock()
	stream, ok := n.streams[nack.MediaSSRC]
	n.streamsMu.Unlock()
	if !ok {
		return
	}

	for i := range nack.Nacks {
		limited := false
		nack.Nacks[i].Range(func(seq uint16) bool {
			// save the packet under the buffer lock
			stream.rtpBufferMutex.Lock()
			packet := stream.rtpBuffer.Get(seq)
			interval := stream.rtt + 5*time.Millisecond
			stream.rtpBufferMutex.Unlock()

			if packet != nil {
				if !packet.BeginResend(n.now(), interval) {
					packet.Release()

					return true
				}
				if targetBitrate != nil && !n.rate.allow(n.now(), packet.Header().MarshalSize()+len(packet.Payload()), targetBitrate()) {
					packet.EndResend(n.now(), false)
					packet.Release()
					limited = true

					return false
				}
				n.resendPacket(stream, packet)
			}

			return true
		})
		if limited {
			return
		}
	}
}

func (n *ResponderInterceptor) resendPacket(stream *localStream, packet *rtpbuffer.RetainablePacket) {
	resend := rtpbuffer.NewResend(func(sent bool) {
		packet.EndResend(n.now(), sent)
		packet.Release()
	})
	attributes := interceptor.Attributes{rtpbuffer.ResendKey{}: resend}
	_, err := stream.rtpWriter.Write(packet.Header(), packet.Payload(), attributes)
	if err != nil {
		n.log.Warnf("failed resending nacked packet: %+v", err)
	}
	resend.Release(err == nil)
}

func (n *ResponderInterceptor) updateRTT(reports []rtcp.ReceptionReport) {
	n.streamsMu.Lock()
	defer n.streamsMu.Unlock()
	for _, report := range reports {
		stream := n.streams[report.SSRC]
		rtt := ntp.ToNTP32(n.now()) - report.LastSenderReport - report.Delay
		if stream != nil && report.LastSenderReport != 0 && rtt < 1<<31 {
			stream.rtpBufferMutex.Lock()
			stream.rttSamples++
			stream.rtt += (time.Duration(rtt)*time.Second/(1<<16) - stream.rtt) / time.Duration(stream.rttSamples)
			stream.rtpBufferMutex.Unlock()
		}
	}
}

// Match libwebrtc's transport retransmission window (kRetransmitWindowSizeMs).
const retransmissionWindow = 500

type retransmissionRate struct {
	sync.Mutex
	start   time.Time
	first   int64
	last    int64
	bytes   int64
	samples int64
	buckets [retransmissionWindow]struct{ bytes, samples int64 }
}

func (r *retransmissionRate) allow(now time.Time, size, targetBitrate int) bool {
	r.Lock()
	defer r.Unlock()
	if r.start.IsZero() {
		r.start = now
	}
	elapsed := max(now.Sub(r.start).Milliseconds(), r.last)
	for tick := r.last + 1; tick <= min(elapsed, r.last+retransmissionWindow); tick++ {
		bucket := &r.buckets[tick%retransmissionWindow]
		r.bytes -= bucket.bytes
		r.samples -= bucket.samples
		bucket.bytes, bucket.samples = 0, 0
	}
	r.last = elapsed
	// Bound startup bursts while allowing one packet at low rates.
	budget := float64(max(targetBitrate, 0)) * retransmissionWindow / 8000
	window := min(elapsed-r.first+1, retransmissionWindow)
	switch {
	case r.samples == 0:
		r.first = elapsed
	case float64(r.bytes)+float64(size) > budget:
		return false
	case window > 1 && (r.samples > 1 || window == retransmissionWindow):
		rate := math.Round(float64(r.bytes) * 8000 / float64(window))
		addition := int64(size) * 8000 / retransmissionWindow
		if rate+float64(addition) > float64(max(targetBitrate, 0)) {
			return false
		}
	}
	bucket := &r.buckets[elapsed%retransmissionWindow]
	bucket.bytes += int64(size)
	bucket.samples++
	r.bytes += int64(size)
	r.samples++

	return true
}
