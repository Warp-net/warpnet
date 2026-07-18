// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"testing"
	"time"
)

func TestIsFresh(t *testing.T) {
	p := &WarpMiddleware{freshnessWindow: 5 * time.Minute}
	now := time.Now()

	cases := []struct {
		name string
		ts   time.Time
		want bool
	}{
		{"now", now, true},
		{"within past", now.Add(-2 * time.Minute), true},
		{"within future", now.Add(2 * time.Minute), true},
		{"stale", now.Add(-10 * time.Minute), false},
		{"future skew", now.Add(10 * time.Minute), false},
		{"zero", time.Time{}, false},
	}
	for _, c := range cases {
		if got := p.isFresh(c.ts); got != c.want {
			t.Errorf("%s: isFresh=%v want %v", c.name, got, c.want)
		}
	}
}

// TestIsFresh_DefaultsWindow ensures a zero-valued freshnessWindow falls back
// to the package default rather than rejecting everything.
func TestIsFresh_DefaultsWindow(t *testing.T) {
	p := &WarpMiddleware{}
	if !p.isFresh(time.Now()) {
		t.Fatal("expected now to be fresh with default window")
	}
	if p.isFresh(time.Now().Add(-messageFreshnessWindow - time.Minute)) {
		t.Fatal("expected stale message to fail with default window")
	}
}
