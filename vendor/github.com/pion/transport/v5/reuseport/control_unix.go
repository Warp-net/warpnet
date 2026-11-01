// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build unix

package reuseport

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func Control(network, address string, conn syscall.RawConn) error {
	return conn.Control(func(fd uintptr) {
		err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1) //nolint:gosec // GG115
		if err != nil {
			return
		}
		_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1) //nolint:gosec // GG115
	})
}
