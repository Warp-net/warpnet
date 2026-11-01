// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"io"
	"net"
	"time"

	"github.com/pion/transport/v5"
)

// TCPConn implements transport.TCPConn.
type TCPConn struct {
	locAddr *net.TCPAddr
	remAddr *net.TCPAddr
}

var _ transport.TCPConn = &TCPConn{}

// Read reads data from the connection.
func (c *TCPConn) Read(b []byte) (int, error) {
	return 0, transport.ErrNotSupported
}

// Write writes data to the connection.
func (c *TCPConn) Write(b []byte) (int, error) {
	return 0, transport.ErrNotSupported
}

// Close closes the connection.
func (c *TCPConn) Close() error {
	return transport.ErrNotSupported
}

// CloseRead closes the read side of the connection.
func (c *TCPConn) CloseRead() error {
	return transport.ErrNotSupported
}

// CloseWrite closes the write side of the connection.
func (c *TCPConn) CloseWrite() error {
	return transport.ErrNotSupported
}

// LocalAddr returns the local network address.
func (c *TCPConn) LocalAddr() net.Addr {
	return c.locAddr
}

// RemoteAddr returns the remote network address.
func (c *TCPConn) RemoteAddr() net.Addr {
	return c.remAddr
}

// SetDeadline sets the read and write deadlines associated with the connection.
func (c *TCPConn) SetDeadline(t time.Time) error {
	return transport.ErrNotSupported
}

// SetReadDeadline sets the deadline for future Read calls.
func (c *TCPConn) SetReadDeadline(t time.Time) error {
	return transport.ErrNotSupported
}

// SetWriteDeadline sets the deadline for future Write calls.
func (c *TCPConn) SetWriteDeadline(t time.Time) error {
	return transport.ErrNotSupported
}

// ReadFrom reads data from r and writes it to the connection.
func (c *TCPConn) ReadFrom(r io.Reader) (int64, error) {
	return 0, transport.ErrNotSupported
}

// SetLinger sets the behavior of Close method on a connection with pending data.
func (c *TCPConn) SetLinger(int) error {
	return transport.ErrNotSupported
}

// SetKeepAlive enables or disables the keep-alive functionality for this connection.
func (c *TCPConn) SetKeepAlive(bool) error {
	return transport.ErrNotSupported
}

// SetKeepAlivePeriod sets the period between keep-alive messages for this connection.
func (c *TCPConn) SetKeepAlivePeriod(time.Duration) error {
	return transport.ErrNotSupported
}

// SetNoDelay enables or disables the Nagle's algorithm for this connection.
func (c *TCPConn) SetNoDelay(bool) error {
	return transport.ErrNotSupported
}

// SetWriteBuffer sets the size of the operating system's transmit buffer associated
// with the connection.
func (c *TCPConn) SetWriteBuffer(int) error {
	return transport.ErrNotSupported
}

// SetReadBuffer sets the size of the operating system's receive buffer associated
// with the connection.
func (c *TCPConn) SetReadBuffer(int) error {
	return transport.ErrNotSupported
}
