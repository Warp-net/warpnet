// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package cc

import (
	"fmt"
	"time"

	"github.com/pion/rtcp"
)

// Acknowledgment holds information about a packet and if/when it has been
// sent/received.
type Acknowledgment struct {
	SequenceNumber uint16 // Either RTP SequenceNumber or TWCC
	SSRC           uint32
	Size           int
	Departure      time.Time
	Arrival        time.Time
	ECN            rtcp.ECN
}

func (a Acknowledgment) String() string {
	s := "ACK:\n"
	s += fmt.Sprintf("\tTWCC:\t%v\n", a.SequenceNumber)
	s += fmt.Sprintf("\tSIZE:\t%v\n", a.Size)
	s += fmt.Sprintf("\tDEPARTURE:\t%v\n", int64(float64(a.Departure.UnixNano())/1e+6))
	s += fmt.Sprintf("\tARRIVAL:\t%v\n", int64(float64(a.Arrival.UnixNano())/1e+6))

	return s
}
