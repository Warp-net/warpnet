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
	"sync"
	"time"

	_ "github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/domain"
	log "github.com/sirupsen/logrus"

	local "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/json"
	"github.com/oklog/ulid/v2"
)

var (
	ErrTweetNotFound = local.DBError("tweet not found")
	ErrViewsNotFound = local.DBError("views not found")
	ErrReplyNotFound = local.DBError("reply not found")
)

const (
	TweetsNamespace         = "/TWEETS"
	tweetsCountSubspace     = "TWEETSCOUNT"
	tweetsModeratedSubspace = "TWEETSMODERATED"
	reTweetsCountSubspace   = "RETWEETSCOUNT"
	reTweetersSubspace      = "RETWEETERS"
	viewsSubspace           = "VIEWS"
	viewersSubspace         = "VIEWERS"

	// RepliesNamespace is the thread index: replies are stored keyed by their
	// root tweet so a whole thread subtree can be fetched with one prefix
	// scan, independent of which user authored each reply. Replies are tweets
	// (domain.Tweet with ParentId set) but live here instead of the author's
	// timeline keyspace.
	RepliesNamespace     = "/REPLY"
	repliesCountSubspace = "REPLIESCOUNT"
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
type TweetStatsStorer interface {
	GetAggregatedStat(key ds.Key) (uint64, error)
	Increment(key ds.Key) error
	Decrement(key ds.Key) error
}

// viewLockShards is the number of stripes in the per-tweet RecordView
// lock pool. Sized so timeline-scrolling workloads see negligible
// contention without holding one mutex per tweet ever observed.
const viewLockShards = 64

type TweetRepo struct {
	db      TweetsStorer
	statsDb TweetStatsStorer
	// viewLocks is a sharded mutex pool keyed by hash(tweetId). It
	// serializes concurrent RecordView calls on the same counter so
	// they don't collide on Badger's optimistic transactions and lose
	// updates, while still allowing different tweets to proceed in
	// parallel.
	viewLocks [viewLockShards]sync.Mutex
}

func NewTweetRepo(db TweetsStorer, statsDb TweetStatsStorer) *TweetRepo {
	return &TweetRepo{db: db, statsDb: statsDb}
}

func (repo *TweetRepo) viewLock(tweetId string) *sync.Mutex {
	var h uint32
	for i := 0; i < len(tweetId); i++ {
		h = h*31 + uint32(tweetId[i])
	}
	return &repo.viewLocks[h%viewLockShards]
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
	if tweet.UserId == "" && tweet.Text == "" {
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
		return newTweet, err
	}

	countKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(tweetsCountSubspace).
		AddRootID(userId).
		Build()

	_, err = txn.Increment(countKey)
	if err != nil {
		return newTweet, err
	}

	if err := txn.Commit(); err != nil {
		return newTweet, err
	}

	return newTweet, nil
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
	if updateTweet.Text != "" && updateTweet.Text != existedTweet.Text {
		existedTweet.Text = updateTweet.Text
	}
	existedTweet.UpdatedAt = &now

	expiration := time.Unix(int64(expiresAt), 0) //#nosec
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

// Pin / Unpin flip the Pinned flag on the tweet record. Pin must be a no-op
// after the first call to keep the storage write idempotent; the caller is
// responsible for ensuring userId is the tweet author (handler-side check).
func (repo *TweetRepo) Pin(userId, tweetId string) (domain.Tweet, error) {
	return repo.setPinned(userId, tweetId, true)
}

func (repo *TweetRepo) Unpin(userId, tweetId string) (domain.Tweet, error) {
	return repo.setPinned(userId, tweetId, false)
}

// AppendEdit records an immutable edit revision for a tweet. Revisions
// are append-only — never updated, never deleted (except via the tweet's
// own delete handler, which removes the tweet from List* but leaves the
// revisions in place for audit).
func (repo *TweetRepo) AppendEdit(edit domain.TweetEdit) (domain.TweetEdit, error) {
	if edit.OriginalTweetId == "" {
		return domain.TweetEdit{}, local.DBError("empty tweet id")
	}
	if edit.UserId == "" {
		return domain.TweetEdit{}, local.DBError("empty user id")
	}
	if edit.Text == "" {
		return domain.TweetEdit{}, local.DBError("empty text")
	}
	if edit.Id == "" {
		edit.Id = ulid.Make().String()
	}
	if edit.EditedAt.IsZero() {
		edit.EditedAt = time.Now()
	}

	key := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix("EDITS").
		AddRootID(edit.OriginalTweetId).
		AddReversedTimestamp(edit.EditedAt).
		AddParentId(edit.Id).
		Build()

	bt, err := json.Marshal(edit)
	if err != nil {
		return domain.TweetEdit{}, err
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.TweetEdit{}, err
	}
	defer txn.Rollback()

	if err := txn.Set(key, bt); err != nil {
		return domain.TweetEdit{}, err
	}
	if err := txn.Commit(); err != nil {
		return domain.TweetEdit{}, err
	}
	return edit, nil
}

func (repo *TweetRepo) setPinned(userId, tweetId string, pinned bool) (domain.Tweet, error) {
	if userId == "" {
		return domain.Tweet{}, local.DBError("no user id")
	}
	if tweetId == "" {
		return domain.Tweet{}, local.DBError("no tweet id")
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.Tweet{}, err
	}
	defer txn.Rollback()

	existing, expiresAt, err := get(txn, userId, tweetId)
	if err != nil {
		return domain.Tweet{}, err
	}
	if existing.Pinned == pinned {
		return existing, txn.Commit()
	}
	existing.Pinned = pinned
	now := time.Now()
	existing.UpdatedAt = &now

	expiration := time.Unix(int64(expiresAt), 0) //#nosec
	ttl := max(expiration.Sub(now), 0)
	if _, err := storeTweet(txn, existing.UserId, existing, ttl, true); err != nil {
		return domain.Tweet{}, err
	}
	if err := txn.Commit(); err != nil {
		return domain.Tweet{}, err
	}
	return existing, nil
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
	countKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(tweetsCountSubspace).
		AddRootID(userID).
		Build()

	_, err = txn.Decrement(countKey)
	if err != nil {
		return err
	}
	if err := txn.Commit(); err != nil {
		return err
	}
	return nil
}

func deleteTweet(txn local.WarpTransactioner, userId, tweetId string) error {
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

// ====================== REPLIES (thread) ====================

// AddReply stores a reply (a domain.Tweet with ParentId set) in the thread
// index keyed by its root tweet and bumps the parent's reply counter.
func (repo *TweetRepo) AddReply(reply domain.Tweet) (domain.Tweet, error) {
	if reply.UserId == "" && reply.Text == "" {
		return reply, local.DBError("empty reply")
	}
	if reply.RootId == "" {
		return reply, local.DBError("empty root")
	}
	if reply.ParentId == nil {
		return reply, local.DBError("empty parent")
	}
	if reply.Id == "" {
		reply.Id = ulid.Make().String()
	}
	if reply.Id == reply.RootId {
		return reply, local.DBError("this is tweet not reply")
	}
	if reply.CreatedAt.IsZero() {
		reply.CreatedAt = time.Now()
	}

	data, err := json.Marshal(reply)
	if err != nil {
		return reply, fmt.Errorf("marshalling reply meta: %w", err)
	}

	treeKey := local.NewPrefixBuilder(RepliesNamespace).
		AddRootID(reply.RootId).
		AddRange(local.FixedRangeKey).
		AddParentId(reply.Id).
		Build()

	parentSortableKey := local.NewPrefixBuilder(RepliesNamespace).
		AddRootID(reply.RootId).
		AddParentId(*reply.ParentId).
		AddId(reply.Id).
		AddReversedTimestamp(reply.CreatedAt).
		Build()

	replyCountKey := local.NewPrefixBuilder(RepliesNamespace).
		AddSubPrefix(repliesCountSubspace).
		AddRootID(*reply.ParentId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return reply, fmt.Errorf("creating transaction: %w", err)
	}
	defer txn.Rollback()

	if err := txn.Set(treeKey, parentSortableKey.Bytes()); err != nil {
		return reply, fmt.Errorf("adding reply sortable key: %w", err)
	}
	if err := txn.Set(parentSortableKey, data); err != nil {
		return reply, fmt.Errorf("adding reply data: %w", err)
	}
	_, err = txn.Increment(replyCountKey)
	if err != nil {
		return reply, err
	}
	if err := txn.Commit(); err != nil {
		return reply, err
	}
	if repo.statsDb == nil {
		return reply, nil
	}
	if err := repo.statsDb.Increment(replyCountKey.DatastoreKey()); err != nil {
		log.Warnf("reply: stats db increment: %v", err)
	}
	return reply, nil
}

func (repo *TweetRepo) GetReply(rootID string, replyId string) (tweet domain.Tweet, err error) {
	if rootID == "" || replyId == "" {
		return tweet, local.DBError("rootID and replyId cannot be empty")
	}

	treeKey := local.NewPrefixBuilder(RepliesNamespace).
		AddRootID(rootID).
		AddRange(local.FixedRangeKey).
		AddParentId(replyId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return tweet, fmt.Errorf("creating transaction: %w", err)
	}
	defer txn.Rollback()

	sortableKey, err := txn.Get(treeKey)
	if err != nil {
		return tweet, err
	}

	data, err := txn.Get(local.DatabaseKey(sortableKey))
	if err != nil {
		return tweet, err
	}

	if err = json.Unmarshal(data, &tweet); err != nil {
		return tweet, fmt.Errorf("unmarshalling reply: %w", err)
	}
	return tweet, txn.Commit()
}

func (repo *TweetRepo) RepliesCount(tweetId string) (repliesNum uint64, err error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}
	replyCountKey := local.NewPrefixBuilder(RepliesNamespace).
		AddSubPrefix(repliesCountSubspace).
		AddRootID(tweetId).
		Build()

	if repo.statsDb != nil {
		total, err := repo.statsDb.GetAggregatedStat(replyCountKey.DatastoreKey())
		if err == nil {
			return total, nil
		}
		log.Warnf("get replies count stat: %v", err)
	}

	bt, err := repo.db.Get(replyCountKey)

	if local.IsNotFoundError(err) {
		return 0, ErrReplyNotFound
	}
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(bt), nil
}

func (repo *TweetRepo) DeleteReply(rootID, parentID, replyID string) error {
	if rootID == "" || parentID == "" || replyID == "" {
		return local.DBError("rootID, parent ID or replyID cannot be empty")
	}

	treeKey := local.NewPrefixBuilder(RepliesNamespace).
		AddRootID(rootID).
		AddRange(local.FixedRangeKey).
		AddParentId(replyID).
		Build()

	replyCountKey := local.NewPrefixBuilder(RepliesNamespace).
		AddSubPrefix(repliesCountSubspace).
		AddRootID(parentID).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return fmt.Errorf("creating transaction: %w", err)
	}
	defer txn.Rollback()

	sortableKey, err := txn.Get(treeKey)
	if err != nil {
		return fmt.Errorf("getting sortable key: %w", err)
	}
	if err := txn.Delete(treeKey); err != nil {
		return fmt.Errorf("deleting tree key: %w", err)
	}
	if err := txn.Delete(local.DatabaseKey(sortableKey)); err != nil {
		return fmt.Errorf("deleting sortable key: %w", err)
	}
	_, err = txn.Decrement(replyCountKey)
	if err != nil {
		return err
	}
	if err := txn.Commit(); err != nil {
		return err
	}
	if repo.statsDb == nil {
		return nil
	}
	if err := repo.statsDb.Decrement(replyCountKey.DatastoreKey()); err != nil {
		log.Warnf("reply: stats db decrement: %v", err)
	}
	return nil
}

func (repo *TweetRepo) GetRepliesTree(rootId, parentId string, limit *uint64, cursor *string) ([]domain.ReplyNode, string, error) {
	if rootId == "" {
		return nil, "", local.DBError("root ID cannot be blank")
	}

	prefix := local.NewPrefixBuilder(RepliesNamespace).
		AddRootID(rootId).
		AddParentId(parentId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", fmt.Errorf("creating transaction: %w", err)
	}
	defer txn.Rollback()

	items, cur, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return nil, "", fmt.Errorf("listing replies: %w", err)
	}

	if err := txn.Commit(); err != nil {
		return nil, "", fmt.Errorf("committing transaction: %w", err)
	}

	replies := make([]domain.Tweet, 0, len(items))
	for _, item := range items {
		var t domain.Tweet
		if err = json.Unmarshal(item.Value, &t); err != nil {
			return nil, "", fmt.Errorf("unmarshalling reply: %w", err)
		}
		replies = append(replies, t)
	}

	return buildRepliesTree(replies), cur, nil
}

func buildRepliesTree(replies []domain.Tweet) []domain.ReplyNode {
	if len(replies) == 0 {
		return []domain.ReplyNode{}
	}

	type node struct {
		reply    domain.Tweet
		children []*node
	}

	nodeMap := make(map[string]*node, len(replies))
	var rootNodes []*node

	for _, reply := range replies {
		if reply.Id == "" {
			continue
		}
		nodeMap[reply.Id] = &node{reply: reply}
	}

	for _, reply := range replies {
		if reply.Id == "" {
			continue
		}

		n := nodeMap[reply.Id]
		if n == nil {
			continue
		}

		if reply.ParentId == nil {
			rootNodes = append(rootNodes, n)
			continue
		}

		parentNode, ok := nodeMap[*reply.ParentId]
		if !ok {
			rootNodes = append(rootNodes, n)
			continue
		}

		parentNode.children = append(parentNode.children, n)
	}

	var toReplyNode func(n *node) domain.ReplyNode
	toReplyNode = func(n *node) domain.ReplyNode {
		children := make([]domain.ReplyNode, 0, len(n.children))
		for _, child := range n.children {
			children = append(children, toReplyNode(child))
		}
		return domain.ReplyNode{
			Reply:    n.reply,
			Children: children,
		}
	}

	roots := make([]domain.ReplyNode, 0, len(rootNodes))
	for _, rn := range rootNodes {
		roots = append(roots, toReplyNode(rn))
	}

	sort.SliceStable(roots, func(i, j int) bool {
		return roots[i].Reply.CreatedAt.After(roots[j].Reply.CreatedAt)
	})

	return roots
}

// ====================== RETWEET ====================

func (repo *TweetRepo) NewRetweet(tweet domain.Tweet) (_ domain.Tweet, err error) {
	if tweet.RetweetedBy == nil {
		return tweet, local.DBError("retweet: by unknown")
	}

	// A retweet is keyed by the *source* tweet id (which the wire
	// puts in tweet.Id). Whether or not the retweeter attached a
	// comment is a frontend concern — see below for how a comment
	// turns the retweet record into a quote.
	sourceTweetId := tweet.Id
	retweetCountKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(reTweetsCountSubspace).
		AddRootID(sourceTweetId).
		Build()

	retweetersKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(reTweetersSubspace).
		AddRootID(sourceTweetId).
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

	// A quote — a regular tweet that *references* another tweet —
	// is signalled by a non-empty QuotedTweetId on the wire. Store
	// it as its own tweet under the retweeter's namespace with a
	// fresh ULID so it lives alongside the retweeter's other
	// posts (and a retweeter can quote the same source more than
	// once with different commentary). The retweet count above
	// still increments for the source so quotes show up in the
	// source's "X people retweeted me" count. The reader decides
	// from QuotedTweetId whether to render an embedded preview.
	isQuote := tweet.QuotedTweetId != nil && *tweet.QuotedTweetId != ""
	switch {
	case isQuote:
		tweet.Id = ulid.Make().String()
	case tweet.UserId == *tweet.RetweetedBy:
		tweet.Id = domain.RetweetPrefix + tweet.Id
	}

	newTweet, err := storeTweet(txn, *tweet.RetweetedBy, tweet, local.PermanentTTL, false)
	if err != nil {
		return tweet, err
	}

	_, err = txn.Increment(retweetCountKey)
	if err != nil {
		return newTweet, err
	}

	if err := txn.Commit(); err != nil {
		return newTweet, err
	}
	if repo.statsDb == nil {
		return newTweet, nil
	}
	if err := repo.statsDb.Increment(retweetCountKey.DatastoreKey()); err != nil {
		log.Warnf("retweet: stats db increment: %v", err)
	}
	return newTweet, nil
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
	if err := txn.Commit(); err != nil {
		return err
	}
	if repo.statsDb == nil {
		return nil
	}
	if err := repo.statsDb.Decrement(retweetCountKey.DatastoreKey()); err != nil {
		log.Warnf("retweet: stats db decrement: %v", err)
	}
	return nil
}

func (repo *TweetRepo) RetweetsCount(tweetId string) (uint64, error) {
	if tweetId == "" {
		return 0, local.DBError("retweets count: empty tweet id")
	}
	retweetCountKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(reTweetsCountSubspace).
		AddRootID(tweetId).
		Build()

	if repo.statsDb != nil {
		total, err := repo.statsDb.GetAggregatedStat(retweetCountKey.DatastoreKey())
		if err == nil {
			return total, nil
		}
		log.Warnf("crdt retweets count not found for %s - %s", tweetId, err)
	}

	bt, err := repo.db.Get(retweetCountKey)
	if local.IsNotFoundError(err) {
		return 0, ErrTweetNotFound
	}
	if err != nil {
		return 0, err
	}
	count := binary.BigEndian.Uint64(bt)
	return count, nil
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

// RecordView increments the view counter for tweetId on behalf of viewerId.
// The (tweetId, viewerId) pair is recorded permanently, so subsequent
// views from the same viewer — across sessions, restarts, days — are
// no-ops. The first call wins; the counter is incremented exactly once
// per unique viewer. The increment is atomic via the underlying
// transaction and replicated through the CRDT stats store, so it is
// safe under concurrent calls across nodes.
func (repo *TweetRepo) RecordView(tweetId, viewerId string) (uint64, error) {
	if tweetId == "" {
		return 0, local.DBError("view: empty tweet id")
	}
	if viewerId == "" {
		return 0, local.DBError("view: empty viewer id")
	}

	viewsKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(viewsSubspace).
		AddRootID(tweetId).
		Build()

	viewerKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(viewersSubspace).
		AddRootID(tweetId).
		AddRange(local.NoneRangeKey).
		AddParentId(viewerId).
		Build()

	mu := repo.viewLock(tweetId)
	mu.Lock()
	defer mu.Unlock()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	_, err = txn.Get(viewerKey)
	switch {
	case err == nil:
		// This viewer has already been counted for this tweet — drop
		// the read-only txn and report the current canonical count.
		if err := txn.Commit(); err != nil {
			return 0, err
		}
		return repo.GetViewsCount(tweetId)
	case local.IsNotFoundError(err):
		// fall through and record the new view
	default:
		return 0, err
	}

	if err := txn.Set(viewerKey, []byte(viewerId)); err != nil {
		return 0, err
	}
	localCount, err := txn.Increment(viewsKey)
	if err != nil {
		return 0, err
	}
	if err := txn.Commit(); err != nil {
		return 0, err
	}
	if repo.statsDb != nil {
		if err := repo.statsDb.Increment(viewsKey.DatastoreKey()); err != nil {
			log.Warnf("view: stats db increment: %v", err)
		}
	}
	// Prefer the CRDT-aggregated total when it reflects this write,
	// but fall back to the just-incremented local counter if the
	// CRDT replication failed or hasn't caught up yet — otherwise the
	// caller could see a count smaller than the value we just wrote.
	if crdtCount, err := repo.GetViewsCount(tweetId); err == nil && crdtCount > localCount {
		return crdtCount, nil
	}
	return localCount, nil
}

func (repo *TweetRepo) GetViewsCount(tweetId string) (uint64, error) {
	if tweetId == "" {
		return 0, local.DBError("views: empty tweet id")
	}

	viewsKey := local.NewPrefixBuilder(TweetsNamespace).
		AddSubPrefix(viewsSubspace).
		AddRootID(tweetId).
		Build()

	if repo.statsDb != nil {
		total, err := repo.statsDb.GetAggregatedStat(viewsKey.DatastoreKey())
		if err == nil {
			return total, nil
		}
		log.Warnf("crdt views count not found for %s - %s", tweetId, err)
	}

	bt, err := repo.db.Get(viewsKey)
	if local.IsNotFoundError(err) {
		return 0, ErrViewsNotFound
	}
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(bt), nil
}
