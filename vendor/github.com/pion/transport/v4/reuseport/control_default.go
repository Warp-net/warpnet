// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !unix && !windows

package reuseport

import (
	"syscall"
)

func Control(network, address string, c syscall.RawConn) error {
	return nil
}
