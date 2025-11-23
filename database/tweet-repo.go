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

package database

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	log "github.com/sirupsen/logrus"

	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/json"
	"github.com/oklog/ulid/v2"
)

var ErrTweetNotFound = local.DBError("tweet not found")

const (
	TweetsNamespace         = "/TWEETS"
	tweetsCountSubspace     = "TWEETSCOUNT"
	tweetsModeratedSubspace = "TWEETSMODERATED"
	reTweetsCountSubspace   = "RETWEETSCOUNT"
	reTweetersSubspace      = "RETWEETERS"

	DefaultWarpnetTweetNetwork = "warpnet"
)

type TweetsStorer interface {
	NewTxn() (local.WarpTransactioner, error)
	Set(key local.DatabaseKey, value []byte) error
	Get(key local.DatabaseKey) ([]byte, error)
	Delete(key local.DatabaseKey) error
	GetExpiration(key local.DatabaseKey) (uint64, error)
}

type tweetGetter interface {
	Get(key local.DatabaseKey) ([]byte, error)
	GetExpiration(key local.DatabaseKey) (uint64, error)
}

type TweetRepo struct {
	db TweetsStorer
}

func NewTweetRepo(db TweetsStorer) *TweetRepo {
	return &TweetRepo{db: db}
}

// Create adds a new tweet to the database
func (repo *TweetRepo) Create(userId string, tweet domain.Tweet) (domain.Tweet, error) {
	return repo.CreateWithTTL(userId, tweet, math.MaxInt64)
}

func (repo *TweetRepo) Blocklist(tweetId string) error {
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

func (repo *TweetRepo) IsBlocklisted(tweetId string) bool {
	if tweetId == "" {
		return false
	}

	fixedKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(tweetsModeratedSubspace).
		AddRootID(tweetId).
		AddRange(local.FixedRangeKey).
		Build()
	_, err := repo.db.Get(fixedKey)
	return !local.IsNotFoundError(err)
}

func (repo *TweetRepo) CreateWithTTL(userId string, tweet domain.Tweet, duration time.Duration) (domain.Tweet, error) {
	if tweet == (domain.Tweet{}) {
		return tweet, local.DBError("nil tweet")
	}
	if tweet.Id == "" {
		tweet.Id = ulid.Make().String()
	}
	if tweet.CreatedAt.IsZero() {
		tweet.CreatedAt = time.Now()
	}
	if tweet.Network == "" {
		tweet.Network = "warpnet"
	}
	tweet.RootId = tweet.Id

	txn, err := repo.db.NewTxn()
	if err != nil {
		return tweet, err
	}
	defer txn.Rollback()

	newTweet, err := storeTweet(txn, userId, tweet, duration, false)
	if err != nil {
		return tweet, err
	}

	return newTweet, txn.Commit()
}

func (repo *TweetRepo) Update(updateTweet domain.Tweet) error {
	if updateTweet.Id == "" {
		return local.DBError("no tweet id")
	}
	if updateTweet.UserId == "" {
		return local.DBError("no user id")
	}
	now := time.Now()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	existedTweet, expiresAt, err := get(txn, updateTweet.UserId, updateTweet.Id)
	if err != nil {
		return err
	}

	// TODO other fields
	if updateTweet.Moderation != nil {
		existedTweet.Moderation = updateTweet.Moderation
	}
	existedTweet.UpdatedAt = &now

	expiration := time.Unix(int64(expiresAt), 0)
	ttl := expiration.Sub(now)
	if ttl <= 0 { //nolint:modernize
		ttl = 0
	}
	_, err = storeTweet(txn, existedTweet.UserId, existedTweet, ttl, true)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func storeTweet(
	txn local.WarpTransactioner, userId string, tweet domain.Tweet, duration time.Duration, isUpdate bool,
) (domain.Tweet, error) {
	fixedKey := local.NewPrefixBuilder(TweetsNamespace).
		AddRootID(userId).
		AddRange(local.FixedRangeKey).
		AddParentId(tweet.Id).
		Build()

	sortableKey := local.NewPrefixBuilder(TweetsNamespace).
		AddRootID(userId).
		AddReversedTimestamp(tweet.CreatedAt).
		AddParentId(tweet.Id).
		Build()

	data, err := json.Marshal(tweet)
	if err != nil {
		return tweet, fmt.Errorf("tweet marshal: %w", err)
	}

	if err = txn.SetWithTTL(fixedKey, sortableKey.Bytes(), duration); err != nil {
		return tweet, err
	}
	if err = txn.SetWithTTL(sortableKey, data, duration); err != nil {
		return tweet, err
	}
	if isUpdate {
		return tweet, nil
	}

	countKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(tweetsCountSubspace).
		AddRootID(userId).
		Build()
	if _, err := txn.Increment(countKey); err != nil {
		return tweet, err
	}
	return tweet, nil
}

// Get retrieves a tweet by its ID
func (repo *TweetRepo) Get(userID, tweetID string) (tweet domain.Tweet, err error) {
	t, _, err := get(repo.db, userID, tweetID)
	return t, err
}

func get(txn tweetGetter, userID, tweetID string) (tweet domain.Tweet, exp uint64, err error) {
	fixedKey := local.NewPrefixBuilder(TweetsNamespace).
		AddRootID(userID).
		AddRange(local.FixedRangeKey).
		AddParentId(tweetID).
		Build()
	sortableKeyBytes, err := txn.Get(fixedKey)
	if local.IsNotFoundError(err) {
		return tweet, exp, ErrTweetNotFound
	}
	if err != nil {
		return tweet, exp, err
	}

	data, err := txn.Get(local.DatabaseKey(sortableKeyBytes))
	if local.IsNotFoundError(err) {
		return tweet, exp, ErrTweetNotFound
	}
	if err != nil {
		return tweet, exp, err
	}

	exp, _ = txn.GetExpiration(local.DatabaseKey(sortableKeyBytes))

	err = json.Unmarshal(data, &tweet)
	if err != nil {
		return tweet, exp, err
	}
	return tweet, exp, nil
}

func (repo *TweetRepo) TweetsCount(userId string) (uint64, error) {
	if userId == "" {
		return 0, local.DBError("tweet count: empty userID")
	}
	countKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(tweetsCountSubspace).
		AddRootID(userId).
		Build()
	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()
	bt, err := txn.Get(countKey)
	if local.IsNotFoundError(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := binary.BigEndian.Uint64(bt)
	return count, txn.Commit()
}

// Delete removes a tweet by its ID
func (repo *TweetRepo) Delete(userID, tweetID string) error {
	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err := deleteTweet(txn, userID, tweetID); err != nil {
		return err
	}

	return txn.Commit()
}

func deleteTweet(txn local.WarpTransactioner, userId, tweetId string) error {
	countKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(tweetsCountSubspace).
		AddRootID(userId).
		Build()

	fixedKey := local.NewPrefixBuilder(TweetsNamespace).
		AddRootID(userId).
		AddRange(local.FixedRangeKey).
		AddParentId(tweetId).
		Build()
	sortableKeyBytes, err := txn.Get(fixedKey)
	if err != nil {
		return err
	}

	if err := txn.Delete(fixedKey); err != nil {
		return err
	}
	if err := txn.Delete(local.DatabaseKey(sortableKeyBytes)); err != nil {
		return err
	}
	if _, err := txn.Decrement(countKey); err != nil {
		return err
	}
	return nil
}

func (repo *TweetRepo) List(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
	if userId == "" {
		return nil, "", local.DBError("ID cannot be blank")
	}

	prefix := local.NewPrefixBuilder(TweetsNamespace).
		AddRootID(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	if err := txn.Commit(); err != nil {
		return nil, "", err
	}

	tweets := make([]domain.Tweet, 0, len(items))
	for _, item := range items {
		var t domain.Tweet
		err = json.Unmarshal(item.Value, &t)
		if err != nil {
			return nil, "", err
		}
		tweets = append(tweets, t)
	}
	sort.SliceStable(tweets, func(i, j int) bool {
		return tweets[i].CreatedAt.After(tweets[j].CreatedAt)
	})
	return tweets, cur, nil
}

// ====================== RETWEET ====================

func (repo *TweetRepo) NewRetweet(tweet domain.Tweet) (_ domain.Tweet, err error) {
	if tweet.RetweetedBy == nil {
		return tweet, local.DBError("retweet: by unknown")
	}
	retweetCountKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(reTweetsCountSubspace).
		AddRootID(tweet.Id).
		Build()

	retweetersKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(reTweetersSubspace).
		AddRootID(tweet.Id).
		AddRange(local.NoneRangeKey).
		AddParentId(*tweet.RetweetedBy).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return tweet, err
	}
	defer txn.Rollback()

	if err := txn.Set(retweetersKey, []byte(*tweet.RetweetedBy)); err != nil {
		return tweet, err
	}

	if tweet.UserId == *tweet.RetweetedBy {
		tweet.Id = domain.RetweetPrefix + tweet.Id
	}

	newTweet, err := storeTweet(txn, *tweet.RetweetedBy, tweet, warpnet.PermanentTTL, false)
	if err != nil {
		return tweet, err
	}

	_, err = repo.db.Get(retweetCountKey)
	if !local.IsNotFoundError(err) {
		if _, err = txn.Increment(retweetCountKey); err != nil {
			log.Debugf("Failed to increment retweet count for %s - %s", tweet.Id, err)
			return newTweet, txn.Commit()
		}
	}
	_, err = txn.Increment(retweetCountKey)
	if err != nil {
		return newTweet, err
	}

	return newTweet, txn.Commit()
}

func (repo *TweetRepo) UnRetweet(retweetedByUserID, tweetId string) error {
	if tweetId == "" || retweetedByUserID == "" {
		return local.DBError("unretweet: empty tweet ID or user ID")
	}

	retweetCountKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(reTweetsCountSubspace).
		AddRootID(tweetId).
		Build()

	retweetersKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(reTweetersSubspace).
		AddRootID(tweetId).
		AddRange(local.NoneRangeKey).
		AddParentId(retweetedByUserID).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err := txn.Delete(retweetersKey); err != nil {
		return err
	}

	if err := deleteTweet(txn, retweetedByUserID, tweetId); err != nil {
		return err
	}

	_, err = repo.db.Get(retweetCountKey)
	if local.IsNotFoundError(err) {
		return txn.Commit()
	}
	if err != nil {
		return err
	}
	_, err = txn.Decrement(retweetCountKey)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func (repo *TweetRepo) RetweetsCount(tweetId string) (uint64, error) {
	if tweetId == "" {
		return 0, local.DBError("retweets count: empty tweet id")
	}
	retweetCountKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(reTweetsCountSubspace).
		AddRootID(tweetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	bt, err := txn.Get(retweetCountKey)
	if local.IsNotFoundError(err) {
		return 0, ErrTweetNotFound
	}
	if err != nil {
		return 0, err
	}
	count := binary.BigEndian.Uint64(bt)
	return count, txn.Commit()
}

type retweetersIDs = []string

func (repo *TweetRepo) Retweeters(tweetId string, limit *uint64, cursor *string) (_ retweetersIDs, cur string, err error) {
	if tweetId == "" {
		return nil, "", local.DBError("retweeters: empty tweet id")
	}

	retweetersPrefix := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(reTweetersSubspace).
		AddRootID(tweetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(retweetersPrefix, limit, cursor)
	if local.IsNotFoundError(err) {
		return nil, "", ErrTweetNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if err = txn.Commit(); err != nil {
		return nil, "", err
	}

	retweeters := make(retweetersIDs, 0, len(items))
	for _, item := range items {
		userId := string(item.Value)
		retweeters = append(retweeters, userId)
	}
	return retweeters, cur, nil
}
