// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"net"
	"time"

	"github.com/pion/transport/v5"
)

// TCPListener implements transport.TCPListener.
type TCPListener struct {
	locAddr *net.TCPAddr
}

var _ transport.TCPListener = &TCPListener{}

// Accept waits for and returns the next connection to the listener.
func (l *TCPListener) Accept() (net.Conn, error) {
	return nil, transport.ErrNotSupported
}

// AcceptTCP waits for and returns the next TCP connection to the listener.
func (l *TCPListener) AcceptTCP() (transport.TCPConn, error) {
	return nil, transport.ErrNotSupported
}

// Close closes the listener. Any blocked Accept operations will be unblocked and return errors.
func (l *TCPListener) Close() error {
	return transport.ErrNotSupported
}

// Addr returns the listener's network address.
func (l *TCPListener) Addr() net.Addr {
	return l.locAddr
}

// SetDeadline sets the deadline for future Accept calls.
func (l *TCPListener) SetDeadline(t time.Time) error {
	return transport.ErrNotSupported
}
