// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build windows

package reuseport

import (
	"syscall"

	"golang.org/x/sys/windows"
)

func Control(network, address string, c syscall.RawConn) (err error) {
	return c.Control(func(fd uintptr) {
		_ = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_REUSEADDR, 1)
	})
}
