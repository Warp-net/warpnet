// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package stream

import (
	"testing"
	"time"
)

// A nil map has to be usable: node types that declare no policy at all
// (relay, moderator) must still get sane budgets rather than zeros.
func TestRoutePolicies_NilYieldsDefaults(t *testing.T) {
	var policies RoutePolicies

	got := policies.For("/public/get/user/0.0.0")
	if got.MaxInboundSize != int64(DefaultMaxInboundSize) {
		t.Errorf("max inbound = %d, want %d", got.MaxInboundSize, DefaultMaxInboundSize)
	}
	if got.IODeadline != DefaultIODeadline {
		t.Errorf("deadline = %s, want %s", got.IODeadline, DefaultIODeadline)
	}
}

func TestRoutePolicies_UnlistedRouteGetsDefaults(t *testing.T) {
	policies := RoutePolicies{"/private/post/video/0.0.0": {MaxInboundSize: 999}}

	got := policies.For("/public/get/user/0.0.0")
	if got.MaxInboundSize != int64(DefaultMaxInboundSize) {
		t.Errorf("max inbound = %d, want default %d", got.MaxInboundSize, DefaultMaxInboundSize)
	}
}

func TestRoutePolicies_Override(t *testing.T) {
	policies := RoutePolicies{
		"/private/post/video/0.0.0": {MaxInboundSize: 72 << 20, IODeadline: 5 * time.Minute},
	}

	got := policies.For("/private/post/video/0.0.0")
	if got.MaxInboundSize != 72<<20 {
		t.Errorf("max inbound = %d, want %d", got.MaxInboundSize, 72<<20)
	}
	if got.IODeadline != 5*time.Minute {
		t.Errorf("deadline = %s, want 5m", got.IODeadline)
	}
}

// A caller states only what differs; the unset half must not become zero.
func TestRoutePolicies_PartialOverrideKeepsOtherDefault(t *testing.T) {
	policies := RoutePolicies{"/public/get/video/0.0.0": {IODeadline: 5 * time.Minute}}

	got := policies.For("/public/get/video/0.0.0")
	if got.IODeadline != 5*time.Minute {
		t.Errorf("deadline = %s, want 5m", got.IODeadline)
	}
	if got.MaxInboundSize != int64(DefaultMaxInboundSize) {
		t.Errorf("max inbound = %d, want default %d", got.MaxInboundSize, DefaultMaxInboundSize)
	}
}
