//go:build darwin

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package security

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestSetCoreDumpLimit(t *testing.T) {
	if err := setCoreDumpLimit(0); err != nil {
		t.Fatalf("setCoreDumpLimit(0): %v", err)
	}

	var rlim unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &rlim); err != nil {
		t.Fatalf("Getrlimit(RLIMIT_CORE): %v", err)
	}
	if rlim.Cur != 0 {
		t.Errorf("core dump soft limit = %d, want 0", rlim.Cur)
	}
}
