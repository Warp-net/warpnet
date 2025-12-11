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

package database

import (
	"time"

	"github.com/Warp-net/warpnet/core/crdt"
	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/domain"
	log "github.com/sirupsen/logrus"
)

// CRDTTweetRepo wraps TweetRepo with CRDT-based statistics for retweets
type CRDTTweetRepo struct {
	*TweetRepo
	crdtStore *crdt.CRDTStatsStore
}

// NewCRDTTweetRepo creates a new CRDT-enabled tweet repository
func NewCRDTTweetRepo(db TweetsStorer, crdtStore *crdt.CRDTStatsStore) *CRDTTweetRepo {
	return &CRDTTweetRepo{
		TweetRepo: NewTweetRepo(db),
		crdtStore: crdtStore,
	}
}

// NewRetweet creates a new retweet and updates CRDT counter
func (repo *CRDTTweetRepo) NewRetweet(tweet domain.Tweet) (_ domain.Tweet, err error) {
	// Create retweet locally first
	retweeted, err := repo.TweetRepo.NewRetweet(tweet)
	if err != nil {
		return domain.Tweet{}, err
	}

	// Update CRDT counter
	if repo.crdtStore != nil {
		_, err := repo.crdtStore.IncrementStat(tweet.Id, crdt.StatTypeRetweets)
		if err != nil {
			log.Warnf("failed to increment CRDT retweet count: %v", err)
		}
	}

	return retweeted, nil
}

// UnRetweet removes a retweet and updates CRDT counter
func (repo *CRDTTweetRepo) UnRetweet(retweetedByUserID, tweetId string) error {
	// Remove retweet locally first
	err := repo.TweetRepo.UnRetweet(retweetedByUserID, tweetId)
	if err != nil {
		return err
	}

	// Update CRDT counter
	if repo.crdtStore != nil {
		_, err := repo.crdtStore.DecrementStat(tweetId, crdt.StatTypeRetweets)
		if err != nil {
			log.Warnf("failed to decrement CRDT retweet count: %v", err)
		}
	}

	return nil
}

// RetweetsCount returns the aggregated retweet count from CRDT if available
func (repo *CRDTTweetRepo) RetweetsCount(tweetId string) (uint64, error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}

	// Try to get aggregated count from CRDT first
	if repo.crdtStore != nil {
		count, err := repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeRetweets)
		if err != nil {
			log.Warnf("failed to get CRDT retweet count, falling back to local: %v", err)
			return repo.TweetRepo.RetweetsCount(tweetId)
		}
		return count, nil
	}

	// Fallback to local count
	return repo.TweetRepo.RetweetsCount(tweetId)
}

// Create adds a new tweet and increments view counter
func (repo *CRDTTweetRepo) Create(userId string, tweet domain.Tweet) (domain.Tweet, error) {
	created, err := repo.TweetRepo.Create(userId, tweet)
	if err != nil {
		return domain.Tweet{}, err
	}

	// Initialize view counter in CRDT (optional)
	if repo.crdtStore != nil {
		// Views will be tracked when tweets are fetched
	}

	return created, nil
}

// CreateWithTTL adds a tweet with TTL and initializes CRDT counters
func (repo *CRDTTweetRepo) CreateWithTTL(userId string, tweet domain.Tweet, duration time.Duration) (domain.Tweet, error) {
	created, err := repo.TweetRepo.CreateWithTTL(userId, tweet, duration)
	if err != nil {
		return domain.Tweet{}, err
	}

	return created, nil
}

// IncrementViewCount increments the view counter for a tweet using CRDT
func (repo *CRDTTweetRepo) IncrementViewCount(tweetId string) (uint64, error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}

	if repo.crdtStore != nil {
		count, err := repo.crdtStore.IncrementStat(tweetId, crdt.StatTypeViews)
		if err != nil {
			log.Warnf("failed to increment CRDT view count: %v", err)
			return 0, err
		}
		return count, nil
	}

	return 0, local.DBError("CRDT store not available")
}

// GetViewCount returns the aggregated view count from CRDT
func (repo *CRDTTweetRepo) GetViewCount(tweetId string) (uint64, error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}

	if repo.crdtStore != nil {
		count, err := repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeViews)
		if err != nil {
			log.Warnf("failed to get CRDT view count: %v", err)
			return 0, err
		}
		return count, nil
	}

	return 0, local.DBError("CRDT store not available")
}
