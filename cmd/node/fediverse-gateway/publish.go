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

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
)

// publishNote fans a Warpnet tweet out to every Fediverse follower of localUser
// as a signed Create(Note). Delivery is best-effort per follower and bounded by
// the gateway's delivery semaphore; it blocks until all deliveries settle.
func (g *gateway) publishNote(ctx context.Context, localUser string, t tweet) {
	actorURLs, err := g.followers.List(localUser)
	if err != nil {
		log.Errorf("publish: list followers of %s: %v", localUser, err)
		return
	}
	if len(actorURLs) == 0 {
		return
	}
	create := g.buildCreateNote(localUser, t)

	var wg sync.WaitGroup
	for _, actorURL := range actorURLs {
		select {
		case g.sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Add(1)
		go func(actor string) {
			defer wg.Done()
			defer func() { <-g.sem }()
			inbox, ierr := g.remoteInbox(ctx, actor)
			if ierr != nil {
				log.Errorf("publish: resolve inbox for %s: %v", actor, ierr)
				return
			}
			if perr := g.postSigned(ctx, localUser, inbox, create); perr != nil {
				log.Errorf("publish: deliver tweet %s to %s: %v", t.Id, inbox, perr)
			}
		}(actorURL)
	}
	wg.Wait()
}
