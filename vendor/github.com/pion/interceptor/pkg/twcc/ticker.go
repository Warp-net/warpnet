// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package twcc

import "time"

// Ticker is an interface for *time.Ticker for use with the SendTicker option.
type Ticker interface {
	Ch() <-chan time.Time
	Stop()
}

// TickerFactory is a factory to create new tickers.
type TickerFactory func(d time.Duration) Ticker

type timeTicker struct {
	*time.Ticker
}

func (t *timeTicker) Ch() <-chan time.Time {
	return t.C
}
