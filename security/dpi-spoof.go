package security

import (
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
}

func NewSpoofConn(conn manet.Conn, fragmentSize, handshakeLen int, maxDelay time.Duration) *SpoofConn {
	return &SpoofConn{
		Conn:         conn,
		mu:           sync.Mutex{},
		bytesWritten: 0,
		fragmentSize: fragmentSize,
		handshakeLen: handshakeLen,
		maxDelay:     maxDelay,
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

	return c.fragmentedWrite(b)
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
