//go:build linux

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

package selfupdate

import (
	"context"
	"fmt"
	"testing"
	"time"

	goselfupdate "github.com/creativeprojects/go-selfupdate"
)

// TestDetectLatestRelease verifies that the Warp-net/warpnet GitHub repository
// is reachable and has at least one published release.
func TestDetectLatestRelease(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	release, found, err := goselfupdate.DetectLatest(ctx, goselfupdate.ParseSlug(warpnetRepository))
	if err != nil {
		t.Skipf("GitHub API unavailable: %v", err)
	}
	if !found {
		t.Fatalf("no release found for %s", warpnetRepository)
	}
	if release.Version() == "" {
		t.Fatalf("expected non-empty version, got empty string")
	}
	t.Logf("latest release: %s", release.Version())
}

// TestDetectLatestRelease_VersionComparison verifies that the library correctly
// reports when the current version is already up to date.
func TestDetectLatestRelease_VersionComparison(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	release, found, err := goselfupdate.DetectLatest(ctx, goselfupdate.ParseSlug(warpnetRepository))
	if err != nil {
		t.Skipf("GitHub API unavailable: %v", err)
	}
	if !found {
		t.Skipf("no release found for %s", warpnetRepository)
	}

	latestVer := release.Version()

	// A very high fake version must be reported as already up to date.
	if !release.LessOrEqual("9999.0.0") {
		t.Errorf("expected latest release %s to be <= 9999.0.0", latestVer)
	}

	// The release itself must not be less than version "0.0.1".
	if release.LessThan("0.0.1") {
		t.Errorf("expected latest release %s to be >= 0.0.1", latestVer)
	}
}

// TestObservedHigherVersion_BelowThreshold verifies that no trigger is sent
// when fewer than peerVersionThreshold distinct peers have been recorded.
func TestObservedHigherVersion_BelowThreshold(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := NewService(ctx, "1.0.0")

	for i := int64(0); i < peerVersionThreshold-1; i++ {
		svc.ObservedHigherVersion(fmt.Sprintf("peer-%d", i))
	}

	select {
	case <-svc.triggerCh:
		t.Fatal("trigger fired before threshold was reached")
	default:
		// expected: no trigger yet
	}
}

// TestObservedHigherVersion_AtThreshold verifies that exactly peerVersionThreshold
// distinct peers cause one trigger to be sent.
func TestObservedHigherVersion_AtThreshold(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := NewService(ctx, "1.0.0")

	for i := int64(0); i < peerVersionThreshold; i++ {
		svc.ObservedHigherVersion(fmt.Sprintf("peer-%d", i))
	}

	select {
	case <-svc.triggerCh:
		// expected: trigger fired at threshold
	default:
		t.Fatal("trigger was not sent after reaching the threshold")
	}
}

// TestObservedHigherVersion_AboveThreshold verifies that observations beyond
// the threshold do not cause more than one trigger to be queued (channel capacity
// is 1; extra triggers are dropped gracefully).
func TestObservedHigherVersion_AboveThreshold(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := NewService(ctx, "1.0.0")

	for i := int64(0); i < peerVersionThreshold+5; i++ {
		svc.ObservedHigherVersion(fmt.Sprintf("peer-%d", i))
	}

	// Drain the channel; exactly one item should be present.
	count := 0
	for {
		select {
		case <-svc.triggerCh:
			count++
		default:
			if count != 1 {
				t.Fatalf("expected exactly 1 trigger, got %d", count)
			}
			return
		}
	}
}

// TestObservedHigherVersion_DuplicatePeer verifies that repeated observations
// from the same peer are counted only once.
func TestObservedHigherVersion_DuplicatePeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := NewService(ctx, "1.0.0")

	// Report the same peer many times — should never reach threshold.
	for i := int64(0); i < peerVersionThreshold+10; i++ {
		svc.ObservedHigherVersion("peer-same")
	}

	select {
	case <-svc.triggerCh:
		t.Fatal("trigger fired based on duplicate peer observations")
	default:
		// expected: duplicate peer counted only once, threshold not reached
	}
}
