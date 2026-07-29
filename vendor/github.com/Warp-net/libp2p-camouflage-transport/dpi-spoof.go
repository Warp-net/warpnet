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

WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package camouflage

import (
	"bytes"
	"crypto/rand"
	manet "github.com/multiformats/go-multiaddr/net"
	"io"
	"math/big"
	"sync"
	"time"
)

const defaultFragmentSize = 2

// SpoofConn wraps a manet.Conn and transparently splits Write calls into
// small TCP segments while the connection is in the handshake phase (the
// first handshakeLen bytes). After the handshake, writes pass through
// without modification.
type SpoofConn struct {
	manet.Conn

	mu           sync.Mutex
	bytesWritten int
	fragmentSize int
	handshakeLen int
	maxDelay     time.Duration
	sni          []byte
}

// NewSpoofConn wraps conn. When sni is non-empty, handshake writes carrying
// it are cut once inside the name; otherwise the whole handshake prefix is
// chunked into fragmentSize segments.
func NewSpoofConn(conn manet.Conn, fragmentSize, handshakeLen int, maxDelay time.Duration, sni string) *SpoofConn {
	return &SpoofConn{
		Conn:         conn,
		mu:           sync.Mutex{},
		bytesWritten: 0,
		fragmentSize: fragmentSize,
		handshakeLen: handshakeLen,
		maxDelay:     maxDelay,
		sni:          []byte(sni),
	}
}

// Write fragments b into small segments if the handshake phase is still
// active; otherwise it delegates directly to the underlying connection.
func (c *SpoofConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	pastHandshake := c.bytesWritten >= c.handshakeLen
	c.mu.Unlock()

	if pastHandshake {
		return c.Conn.Write(b)
	}
	if len(c.sni) > 0 {
		return c.sniSplitWrite(b)
	}

	return c.fragmentedWrite(b)
}

// sniSplitWrite cuts b once inside the SNI so the name never appears whole
// in a single segment. Buffers that do not carry it go out unmodified: a
// browser sends its ClientHello in one segment, and shredding every
// handshake byte is itself a signature no browser produces.
func (c *SpoofConn) sniSplitWrite(b []byte) (int, error) {
	i := bytes.Index(b, c.sni)
	if i < 0 {
		return c.trackedWrite(b)
	}

	cut := i + len(c.sni)/2
	if cut <= 0 || cut >= len(b) {
		return c.trackedWrite(b)
	}

	n, err := c.trackedWrite(b[:cut])
	if err != nil {
		return n, err
	}
	if delay := randDuration(c.maxDelay); delay > 0 {
		time.Sleep(delay)
	}
	m, err := c.trackedWrite(b[cut:])
	return n + m, err
}

// trackedWrite writes all of b, counting the bytes toward handshakeLen.
func (c *SpoofConn) trackedWrite(b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		n, err := c.Conn.Write(b)
		c.mu.Lock()
		c.bytesWritten += n
		c.mu.Unlock()
		total += n
		if n == 0 && err == nil {
			return total, io.ErrShortWrite
		}
		if err != nil {
			return total, err
		}
		b = b[n:]
	}
	return total, nil
}

func (c *SpoofConn) fragmentedWrite(b []byte) (int, error) {
	fragSize := c.fragmentSize
	if fragSize <= 0 {
		fragSize = defaultFragmentSize
	}

	total := 0
	for len(b) > 0 {
		c.mu.Lock()
		pastHandshake := c.bytesWritten >= c.handshakeLen
		c.mu.Unlock()

		if pastHandshake {
			// Past handshake: write-all loop for the remainder.
			for len(b) > 0 {
				n, err := c.Conn.Write(b)
				c.mu.Lock()
				c.bytesWritten += n
				c.mu.Unlock()
				total += n
				if n == 0 && err == nil {
					return total, io.ErrShortWrite
				}
				if err != nil {
					return total, err
				}
				b = b[n:]
			}
			return total, nil
		}

		size := min(fragSize, len(b))
		n, err := c.Conn.Write(b[:size])
		c.mu.Lock()
		c.bytesWritten += n
		c.mu.Unlock()
		total += n
		if n == 0 && err == nil {
			return total, io.ErrShortWrite
		}
		if err != nil {
			return total, err
		}
		b = b[n:]

		c.mu.Lock()
		stillHandshake := c.bytesWritten < c.handshakeLen
		c.mu.Unlock()
		if len(b) > 0 && stillHandshake {
			delay := randDuration(c.maxDelay)
			if delay > 0 {
				time.Sleep(delay)
			}
		}
	}
	return total, nil
}

// CloseRead forwards to the underlying connection if supported.
func (c *SpoofConn) CloseRead() error {
	if cr, ok := c.Conn.(interface{ CloseRead() error }); ok {
		return cr.CloseRead()
	}
	return nil
}

// CloseWrite forwards to the underlying connection if supported.
func (c *SpoofConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

func randDuration(maximum time.Duration) time.Duration {
	if maximum <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(maximum)))
	if err != nil {
		return maximum / 2
	}
	return time.Duration(n.Int64())
}
