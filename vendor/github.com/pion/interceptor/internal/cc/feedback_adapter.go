// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package cc

import (
	"container/list"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// TwccExtensionAttributesKey identifies the TWCC value in the attribute collection
// so we don't need to reparse.
const TwccExtensionAttributesKey = iota

// TargetBitrateKey identifies a func() int returning the current target bitrate in bits per second.
type TargetBitrateKey struct{}

var (
	errMissingTWCCExtension = errors.New("missing transport layer cc header extension")
	errInvalidFeedback      = errors.New("invalid feedback")
)

// FeedbackAdapter converts incoming RTCP Packets (TWCC and RFC8888) into Acknowledgments.
// Acknowledgments are the common format that Congestion Controllers in Pion understand.
type FeedbackAdapter struct {
	lock                     sync.Mutex
	history                  *feedbackHistory
	referenceTime            int64
	referenceTimeInitialized bool
}

// NewFeedbackAdapter returns a new FeedbackAdapter.
func NewFeedbackAdapter() *FeedbackAdapter {
	return &FeedbackAdapter{history: newFeedbackHistory(250)}
}

func (f *FeedbackAdapter) onSentRFC8888(ts time.Time, header *rtp.Header, size int) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.history.add(Acknowledgment{
		SequenceNumber: header.SequenceNumber,
		SSRC:           header.SSRC,
		Size:           size,
		Departure:      ts,
		Arrival:        time.Time{},
		ECN:            0,
	})

	return nil
}

func (f *FeedbackAdapter) onSentTWCC(ts time.Time, extID uint8, header *rtp.Header, size int) error {
	sequenceNumber := header.GetExtension(extID)
	var tccExt rtp.TransportCCExtension
	err := tccExt.Unmarshal(sequenceNumber)
	if err != nil {
		return errMissingTWCCExtension
	}

	f.lock.Lock()
	defer f.lock.Unlock()
	f.history.add(Acknowledgment{
		SequenceNumber: tccExt.TransportSequence,
		SSRC:           0,
		Size:           header.MarshalSize() + size,
		Departure:      ts,
		Arrival:        time.Time{},
		ECN:            0,
	})

	return nil
}

// OnSent records that and when an outgoing packet was sent for later mapping to
// acknowledgments.
func (f *FeedbackAdapter) OnSent(ts time.Time, header *rtp.Header, size int, attributes interceptor.Attributes) error {
	hdrExtensionID := attributes.Get(TwccExtensionAttributesKey)
	id, ok := hdrExtensionID.(uint8)
	if ok && hdrExtensionID != 0 {
		return f.onSentTWCC(ts, id, header, size)
	}

	return f.onSentRFC8888(ts, header, size)
}

// unpackRunLengthChunk reads at most maxStatuses packet statuses from the
// chunk: the feedback's PacketStatusCount bounds every chunk.
func (f *FeedbackAdapter) unpackRunLengthChunk(
	start uint16, refTime time.Time, chunk *rtcp.RunLengthChunk, deltas []*rtcp.RecvDelta, maxStatuses uint16,
) (consumedDeltas int, nextRef time.Time, statusCount uint16, acks []Acknowledgment, err error) {
	count := min(chunk.RunLength, maxStatuses)
	deltaIndex := 0
	acks = []Acknowledgment{}
	if (chunk.PacketStatusSymbol == rtcp.TypeTCCPacketNotReceived ||
		chunk.PacketStatusSymbol == rtcp.TypeTCCPacketReceivedWithoutDelta) && int(count) > len(f.history.items) {
		return 0, refTime, count, f.history.twccInRange(start, count), nil
	}

	end := start + count
	for sequenceNumber := start; sequenceNumber != end; sequenceNumber++ {
		var arrival time.Time
		if chunk.PacketStatusSymbol == rtcp.TypeTCCPacketReceivedSmallDelta ||
			chunk.PacketStatusSymbol == rtcp.TypeTCCPacketReceivedLargeDelta {
			if len(deltas)-1 < deltaIndex {
				return deltaIndex, refTime, count, acks, errInvalidFeedback
			}
			refTime = refTime.Add(time.Duration(deltas[deltaIndex].Delta) * time.Microsecond)
			arrival = refTime
			deltaIndex++
		}

		key := feedbackHistoryKey{
			ssrc:           0,
			sequenceNumber: sequenceNumber,
		}
		if ack, ok := f.history.get(key); ok {
			ack.Arrival = arrival
			acks = append(acks, ack)
		}
	}

	return deltaIndex, refTime, count, acks, nil
}

// unpackStatusVectorChunk reads at most maxStatuses symbols from the chunk.
func (f *FeedbackAdapter) unpackStatusVectorChunk(
	start uint16, refTime time.Time, chunk *rtcp.StatusVectorChunk, deltas []*rtcp.RecvDelta, maxStatuses uint16,
) (consumedDeltas int, nextRef time.Time, statusCount uint16, acks []Acknowledgment, err error) {
	symbols := chunk.SymbolList[:min(len(chunk.SymbolList), int(maxStatuses))]
	deltaIndex := 0
	acks = []Acknowledgment{}
	for i, symbol := range symbols {
		var arrival time.Time
		if symbol == rtcp.TypeTCCPacketReceivedSmallDelta || symbol == rtcp.TypeTCCPacketReceivedLargeDelta {
			if len(deltas)-1 < deltaIndex {
				return deltaIndex, refTime, uint16(len(symbols)), acks, errInvalidFeedback //nolint:gosec // G115
			}
			refTime = refTime.Add(time.Duration(deltas[deltaIndex].Delta) * time.Microsecond)
			arrival = refTime
			deltaIndex++
		}

		key := feedbackHistoryKey{
			ssrc:           0,
			sequenceNumber: start + uint16(i), //nolint:gosec // G115
		}
		if ack, ok := f.history.get(key); ok {
			ack.Arrival = arrival
			acks = append(acks, ack)
		}
	}

	return deltaIndex, refTime, uint16(len(symbols)), acks, nil //nolint:gosec // G115
}

// OnTransportCCFeedback converts incoming TWCC RTCP packet feedback to
// Acknowledgments.
func (f *FeedbackAdapter) OnTransportCCFeedback(
	_ time.Time, feedback *rtcp.TransportLayerCC,
) ([]Acknowledgment, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	result := []Acknowledgment{}
	index := feedback.BaseSequenceNumber
	referenceTime := int64(feedback.ReferenceTime & 0xffffff)
	if f.referenceTimeInitialized {
		// Choose the nearest epoch.
		delta := (referenceTime-f.referenceTime+(1<<23))&0xffffff - (1 << 23)
		referenceTime = f.referenceTime + delta
	}
	refTime := time.Time{}.Add(time.Duration(referenceTime) * 64 * time.Millisecond)
	recvDeltas := feedback.RecvDeltas
	// PacketStatusCount is the number of packets the feedback reports on;
	// symbols beyond it are padding.
	remaining := feedback.PacketStatusCount

	for _, chunk := range feedback.PacketChunks {
		if remaining == 0 {
			break
		}
		switch chunk := chunk.(type) {
		case *rtcp.RunLengthChunk:
			n, nextRefTime, statusCount, acks, err := f.unpackRunLengthChunk(index, refTime, chunk, recvDeltas, remaining)
			if err != nil {
				return nil, err
			}
			refTime = nextRefTime
			result = append(result, acks...)
			recvDeltas = recvDeltas[n:]
			index += statusCount
			remaining -= statusCount
		case *rtcp.StatusVectorChunk:
			n, nextRefTime, statusCount, acks, err := f.unpackStatusVectorChunk(index, refTime, chunk, recvDeltas, remaining)
			if err != nil {
				return nil, err
			}
			refTime = nextRefTime
			result = append(result, acks...)
			recvDeltas = recvDeltas[n:]
			index += statusCount
			remaining -= statusCount
		default:
			return nil, errInvalidFeedback
		}
	}

	if feedback.PacketStatusCount > 0 {
		f.referenceTime = referenceTime
		f.referenceTimeInitialized = true
	}

	return result, nil
}

// OnRFC8888Feedback converts incoming Congestion Control Feedback RTCP packet
// to Acknowledgments.
func (f *FeedbackAdapter) OnRFC8888Feedback(_ time.Time, feedback *rtcp.CCFeedbackReport) []Acknowledgment {
	f.lock.Lock()
	defer f.lock.Unlock()

	result := []Acknowledgment{}
	referenceTime := ntp.ToTime(uint64(feedback.ReportTimestamp) << 16)
	for _, rb := range feedback.ReportBlocks {
		for i, mb := range rb.MetricBlocks {
			sequenceNumber := rb.BeginSequence + uint16(i) //nolint:gosec // G115
			key := feedbackHistoryKey{
				ssrc:           rb.MediaSSRC,
				sequenceNumber: sequenceNumber,
			}
			if ack, ok := f.history.get(key); ok {
				if mb.Received {
					delta := time.Duration((float64(mb.ArrivalTimeOffset) / 1024.0) * float64(time.Second))
					ack.Arrival = referenceTime.Add(-delta)
					ack.ECN = mb.ECN
				}
				result = append(result, ack)
			}
		}
	}

	return result
}

type feedbackHistoryKey struct {
	ssrc           uint32
	sequenceNumber uint16
}

type feedbackHistory struct {
	size      int
	evictList *list.List
	items     map[feedbackHistoryKey]*list.Element
}

func newFeedbackHistory(size int) *feedbackHistory {
	return &feedbackHistory{
		size:      size,
		evictList: list.New(),
		items:     make(map[feedbackHistoryKey]*list.Element),
	}
}

func (f *feedbackHistory) get(key feedbackHistoryKey) (Acknowledgment, bool) {
	ent, ok := f.items[key]
	if ok {
		if ack, ok := ent.Value.(Acknowledgment); ok {
			return ack, true
		}
	}

	return Acknowledgment{}, false
}

func (f *feedbackHistory) add(ack Acknowledgment) {
	key := feedbackHistoryKey{
		ssrc:           ack.SSRC,
		sequenceNumber: ack.SequenceNumber,
	}
	// Check for existing
	if ent, ok := f.items[key]; ok {
		f.evictList.MoveToFront(ent)
		ent.Value = ack

		return
	}
	// Add new
	ent := f.evictList.PushFront(ack)
	f.items[key] = ent
	// Evict if necessary
	if f.evictList.Len() > f.size {
		f.removeOldest()
	}
}

func (f *feedbackHistory) removeOldest() {
	if ent := f.evictList.Back(); ent != nil {
		f.evictList.Remove(ent)
		if ack, ok := ent.Value.(Acknowledgment); ok {
			key := feedbackHistoryKey{
				ssrc:           ack.SSRC,
				sequenceNumber: ack.SequenceNumber,
			}
			delete(f.items, key)
		}
	}
}

func (f *feedbackHistory) twccInRange(start, count uint16) []Acknowledgment {
	acks := []Acknowledgment{}
	for key := range f.items {
		if key.ssrc == 0 && key.sequenceNumber-start < count {
			if ack, ok := f.get(key); ok {
				ack.Arrival = time.Time{}
				acks = append(acks, ack)
			}
		}
	}
	slices.SortFunc(acks, func(a, b Acknowledgment) int {
		return int(a.SequenceNumber-start) - int(b.SequenceNumber-start)
	})

	return acks
}
