/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU Affero General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU Affero General Public License for more details.

 You should have received a copy of the GNU Affero General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package retrier

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

type retrierError string

func (e retrierError) Error() string {
	return string(e)
}

const (
	ErrDeadlineReached retrierError = "retrier: deadline reached"
	ErrStopTrying      retrierError = "retrier: stop trying"
)

type (
	backoff byte

	RetrierFunc = func() error

	Retrier interface {
		Try(ctx context.Context, f RetrierFunc) error
	}

	retrier struct {
		minInterval time.Duration
		maxAttempts uint32
		backoff     backoff
	}
)

const (
	NoBackoff backoff = iota
	FixedBackoff
	ExponentialBackoff
)

// New creates a retrier with the given minimum interval between attempts and maximum retries.
func New(minInterval time.Duration, maxAttempts uint32, b backoff) Retrier {
	return &retrier{
		minInterval: minInterval,
		maxAttempts: maxAttempts,
		backoff:     b,
	}
}

func (r *retrier) Try(ctx context.Context, f RetrierFunc) (err error) {
	var (
		attempt  uint32
		interval = r.minInterval
	)

	for attempt = 0; attempt < r.maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err = f()
		if errors.Is(err, ErrStopTrying) {
			return err
		}
		if err == nil {
			return nil
		}

		switch r.backoff {
		case NoBackoff:

		case FixedBackoff:
			interval = interval + interval
		case ExponentialBackoff:
			interval = interval * 2
		}
		sleepDuration := interval + jitter(r.minInterval)
		time.Sleep(sleepDuration)
	}

	return fmt.Errorf("%v %w", err, ErrDeadlineReached)
}

func jitter(minInterval time.Duration) time.Duration {
	return time.Duration(rand.Int64N(int64(minInterval / 2))) // Add jitter
}
