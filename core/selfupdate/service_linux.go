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
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/creativeprojects/go-selfupdate"
	log "github.com/sirupsen/logrus"
)

const (
	warpnetRepository = "Warp-net/warpnet"

	// peerVersionThreshold is the number of peers with a higher version that
	// must be observed before a self-update is triggered.
	peerVersionThreshold int64 = 2
)

// Service is a Linux-only self-update service for the bootstrap node.
// It responds to SIGUSR1 as well as internal Trigger() calls.
// When triggered, it checks GitHub for a newer release and replaces the
// running binary if one is found, then exits so the supervisor can restart
// the process with the new version.
type Service struct {
	ctx                context.Context
	currentVersion     string
	triggerCh          chan struct{}
	higherVersionCount int64
}

// NewService creates a new self-update service for the given version string.
func NewService(ctx context.Context, currentVersion string) *Service {
	return &Service{
		ctx:            ctx,
		currentVersion: currentVersion,
		triggerCh:      make(chan struct{}, 1),
	}
}

// Run starts the service in a background goroutine.
// It listens for SIGUSR1 as well as calls to Trigger().
func (s *Service) Run() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)

	go func() {
		defer signal.Stop(sigCh)
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-sigCh:
				if err := s.doUpdate(); err != nil {
					log.Errorf("selfupdate: %v", err)
				}
			case <-s.triggerCh:
				if err := s.doUpdate(); err != nil {
					log.Errorf("selfupdate: %v", err)
				}
			}
		}
	}()
}

// Trigger sends an internal signal to the service to perform an update check.
// It is non-blocking: if the service is already busy or the channel is full,
// the trigger is silently dropped (the update will still run).
func (s *Service) Trigger() {
	select {
	case s.triggerCh <- struct{}{}:
	default:
	}
}

// ObservedHigherVersion records that a peer with a higher version has been seen.
// Once peerVersionThreshold such observations have been made, a self-update is
// triggered exactly once.
func (s *Service) ObservedHigherVersion() {
	count := atomic.AddInt64(&s.higherVersionCount, 1)
	if count == peerVersionThreshold {
		s.Trigger()
	}
}

// doUpdate performs the actual self-update: detects the latest GitHub release,
// and if it is newer than currentVersion, downloads and replaces the binary.
func (s *Service) doUpdate() error {
	log.Infof("selfupdate: checking for updates (current version %s)", s.currentVersion)

	latest, found, err := selfupdate.DetectLatest(s.ctx, selfupdate.ParseSlug(warpnetRepository))
	if err != nil {
		return fmt.Errorf("failed to detect latest release: %w", err)
	}
	if !found {
		return fmt.Errorf("no release found for %s", warpnetRepository)
	}

	if latest.LessOrEqual(s.currentVersion) {
		log.Infof("selfupdate: current version %s is already up to date", s.currentVersion)
		return nil
	}

	log.Infof("selfupdate: newer version %s found, updating...", latest.Version())

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	if err := selfupdate.UpdateTo(s.ctx, latest.AssetURL, latest.AssetName, exe); err != nil {
		return fmt.Errorf("failed to update binary: %w", err)
	}

	log.Infof("selfupdate: successfully updated to version %s, restarting...", latest.Version())
	os.Exit(0)
	return nil
}
