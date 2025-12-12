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
)

// CRDTTweetRepo manages tweets with CRDT-based statistics for retweets and views
type CRDTTweetRepo struct {
	db        TweetsStorer
	crdtStore *crdt.CRDTStatsStore
}

// NewCRDTTweetRepo creates a new CRDT-enabled tweet repository
func NewCRDTTweetRepo(db TweetsStorer, crdtStore *crdt.CRDTStatsStore) *CRDTTweetRepo {
	return &CRDTTweetRepo{
		db:        db,
		crdtStore: crdtStore,
	}
}

// Delegate non-stats methods to base repo
func (repo *CRDTTweetRepo) IsBlocklisted(tweetId string) bool {
	txn, err := repo.db.NewTxn()
	if err != nil {
		return false
	}
	defer txn.Rollback()
	
	fixedKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(tweetsModeratedSubspace).
		AddRootID(tweetId).
		AddRange(local.FixedRangeKey).
		Build()
	
	_, err = txn.Get(fixedKey)
	return !local.IsNotFoundError(err)
}

func (repo *CRDTTweetRepo) Blocklist(tweetId string) error {
	if tweetId == "" {
		return nil
	}
	fixedKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(tweetsModeratedSubspace).
		AddRootID(tweetId).
		AddRange(local.FixedRangeKey).
		Build()
	return repo.db.Set(fixedKey, []byte(""))
}

func (repo *CRDTTweetRepo) Get(userID, tweetID string) (domain.Tweet, error) {
	return getTweet(repo.db, userID, tweetID)
}

func (repo *CRDTTweetRepo) List(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
	return listTweets(repo.db, userId, limit, cursor)
}

func (repo *CRDTTweetRepo) Create(userId string, tweet domain.Tweet) (domain.Tweet, error) {
	return createTweet(repo.db, userId, tweet)
}

func (repo *CRDTTweetRepo) CreateWithTTL(userId string, tweet domain.Tweet, duration time.Duration) (domain.Tweet, error) {
	return createTweetWithTTL(repo.db, userId, tweet, duration)
}

func (repo *CRDTTweetRepo) Delete(userID, tweetID string) error {
	return baseTweetRepoDelete(repo.db, userID, tweetID)
}

// NewRetweet creates a new retweet and updates CRDT counter
func (repo *CRDTTweetRepo) NewRetweet(tweet domain.Tweet) (_ domain.Tweet, err error) {
	retweeted, err := baseTweetRepoNewRetweet(repo.db, tweet)
	if err != nil {
		return domain.Tweet{}, err
	}

	// Update CRDT counter
	if repo.crdtStore != nil {
		current, _ := repo.crdtStore.Get(tweet.Id, crdt.StatTypeRetweets)
		newValue := current + 1
		if err := repo.crdtStore.Put(tweet.Id, crdt.StatTypeRetweets, newValue); err != nil {
			return retweeted, err
		}
	}

	return retweeted, nil
}

// UnRetweet removes a retweet and updates CRDT counter
func (repo *CRDTTweetRepo) UnRetweet(retweetedByUserID, tweetId string) error {
	err := baseTweetRepoUnRetweet(repo.db, retweetedByUserID, tweetId)
	if err != nil {
		return err
	}

	// Update CRDT counter
	if repo.crdtStore != nil {
		current, _ := repo.crdtStore.Get(tweetId, crdt.StatTypeRetweets)
		if current > 0 {
			newValue := current - 1
			if err := repo.crdtStore.Put(tweetId, crdt.StatTypeRetweets, newValue); err != nil {
				return err
			}
		}
	}

	return nil
}

// RetweetsCount returns the aggregated retweet count from CRDT
func (repo *CRDTTweetRepo) RetweetsCount(tweetId string) (uint64, error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}

	if repo.crdtStore != nil {
		return repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeRetweets)
	}

	return 0, nil
}

// Retweeters returns the list of users who retweeted a tweet
func (repo *CRDTTweetRepo) Retweeters(tweetId string, limit *uint64, cursor *string) (_ []string, cur string, err error) {
	return baseTweetRepoGetRetweeters(repo.db, tweetId, limit, cursor)
}

// IncrementViewCount increments the view counter for a tweet using CRDT
func (repo *CRDTTweetRepo) IncrementViewCount(tweetId string) (uint64, error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}

	if repo.crdtStore != nil {
		current, _ := repo.crdtStore.Get(tweetId, crdt.StatTypeViews)
		newValue := current + 1
		if err := repo.crdtStore.Put(tweetId, crdt.StatTypeViews, newValue); err != nil {
			return 0, err
		}
		return repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeViews)
	}

	return 0, local.DBError("CRDT store not available")
}

// GetViewCount returns the aggregated view count from CRDT
func (repo *CRDTTweetRepo) GetViewCount(tweetId string) (uint64, error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}

	if repo.crdtStore != nil {
		return repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeViews)
	}

	return 0, nil
}

// Helper functions that call original TweetRepo methods
func getTweet(db TweetsStorer, userID, tweetID string) (domain.Tweet, error) {
	repo := &TweetRepo{db: db}
	return repo.Get(userID, tweetID)
}

func listTweets(db TweetsStorer, userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
	repo := &TweetRepo{db: db}
	return repo.List(userId, limit, cursor)
}

func createTweet(db TweetsStorer, userId string, tweet domain.Tweet) (domain.Tweet, error) {
	repo := &TweetRepo{db: db}
	return repo.Create(userId, tweet)
}

func createTweetWithTTL(db TweetsStorer, userId string, tweet domain.Tweet, duration time.Duration) (domain.Tweet, error) {
	repo := &TweetRepo{db: db}
	return repo.CreateWithTTL(userId, tweet, duration)
}

func baseTweetRepoDelete(db TweetsStorer, userID, tweetID string) error {
	repo := &TweetRepo{db: db}
	return repo.Delete(userID, tweetID)
}

func baseTweetRepoNewRetweet(db TweetsStorer, tweet domain.Tweet) (domain.Tweet, error) {
	repo := &TweetRepo{db: db}
	return repo.NewRetweet(tweet)
}

func baseTweetRepoUnRetweet(db TweetsStorer, retweetedByUserID, tweetId string) error {
	repo := &TweetRepo{db: db}
	return repo.UnRetweet(retweetedByUserID, tweetId)
}

func baseTweetRepoGetRetweeters(db TweetsStorer, tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	repo := &TweetRepo{db: db}
	return repo.Retweeters(tweetId, limit, cursor)
}
