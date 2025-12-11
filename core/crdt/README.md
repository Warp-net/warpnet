# CRDT-Based Tweet Statistics

This document describes the Conflict-free Replicated Data Type (CRDT) implementation for distributed tweet statistics in Warpnet.

## Overview

Warpnet uses CRDT to maintain eventually consistent statistics (likes, retweets, replies, views) across distributed nodes without requiring centralized coordination. This enables:

- **True offline-first operation**: Nodes can update statistics while disconnected
- **Automatic conflict resolution**: No manual intervention needed for concurrent updates
- **Eventual consistency**: All nodes converge to the same state over time
- **No single point of failure**: Each node operates independently
- **Scalability**: Linear scaling with the number of nodes

## Architecture

### Core Components

1. **CRDTStatsStore** (`core/crdt/stats.go`)
   - Manages CRDT-based statistics storage
   - Uses `go-ds-crdt` for Merkle-CRDT implementation
   - Synchronizes state via libp2p PubSub
   - Supports four stat types: likes, retweets, replies, views

2. **Database Wrappers**
   - `CRDTLikeRepo`: Wraps like operations with CRDT counters
   - `CRDTTweetRepo`: Wraps retweet and view operations
   - `CRDTReplyRepo`: Wraps reply counter operations

### Data Model

Each statistic is stored as a G-Counter (Grow-only Counter) CRDT:
- Key format: `/{namespace}/{tweetID}/{statType}/{nodeID}`
- Value: 64-bit unsigned integer counter
- Aggregation: Sum of all node counters for a tweet

### Synchronization

1. **Local Updates**:
   ```
   User action (like/retweet/reply)
     ↓
   Update local database
     ↓
   Increment CRDT counter for current node
     ↓
   Broadcast update via PubSub
   ```

2. **Remote Updates**:
   ```
   Receive PubSub message
     ↓
   Merge CRDT state
     ↓
   Update aggregated count
   ```

3. **Query**:
   ```
   Query request
     ↓
   Query CRDT with prefix for tweet+statType
     ↓
   Sum all node counters
     ↓
   Return aggregated count
   ```

## Usage

### Initialization

```go
import (
    "context"
    "github.com/Warp-net/warpnet/core/crdt"
    ds "github.com/ipfs/go-datastore"
    pubsub "github.com/libp2p/go-libp2p-pubsub"
)

// Create CRDT stats store
crdtStore, err := crdt.NewCRDTStatsStore(
    ctx,
    baseDatastore,  // Base datastore for persistence
    pubsub,         // libp2p PubSub for sync
    nodeID,         // Unique node identifier
    "mainnet",      // Network namespace
)
```

### Creating CRDT-Enabled Repositories

```go
import "github.com/Warp-net/warpnet/database"

// Create CRDT-enabled like repository
likeRepo := database.NewCRDTLikeRepo(db, crdtStore)

// Create CRDT-enabled tweet repository
tweetRepo := database.NewCRDTTweetRepo(db, crdtStore)

// Create CRDT-enabled reply repository
replyRepo := database.NewCRDTReplyRepo(db, crdtStore)
```

### Operations

```go
// Like a tweet
count, err := likeRepo.Like(tweetID, userID)
// count is the aggregated likes across all nodes

// Unlike a tweet
count, err := likeRepo.Unlike(tweetID, userID)

// Get like count
count, err := likeRepo.LikesCount(tweetID)

// Retweet
retweeted, err := tweetRepo.NewRetweet(tweet)

// Get retweet count
count, err := tweetRepo.RetweetsCount(tweetID)

// Increment view count
count, err := tweetRepo.IncrementViewCount(tweetID)
```

### Getting All Statistics

```go
stats, err := crdtStore.GetTweetStats(tweetID)
// stats.LikesCount    - aggregated likes
// stats.RetweetsCount - aggregated retweets
// stats.RepliesCount  - aggregated replies
// stats.ViewsCount    - aggregated views
```

## Design Decisions

### Why G-Counter CRDT?

- **Monotonic growth**: Statistics naturally increase (likes, retweets, etc.)
- **Simple merge**: Sum operation for aggregation
- **No conflict resolution needed**: All increments are commutative and associative
- **Deterministic**: Same inputs always produce same output

### Handling Unlikes/Unretweets

While G-Counters are grow-only, we maintain:
1. **Local tracking**: Traditional database tracks user-specific actions
2. **Node-level counter**: Each node maintains its own counter
3. **Decrement**: When a user unlikes, we decrement the node's counter

This approach:
- Maintains correct counts per node
- Aggregates correctly across nodes
- Handles offline scenarios gracefully

### Fallback Strategy

All CRDT operations include fallback to local storage:
```go
if repo.crdtStore != nil {
    count, err := repo.crdtStore.GetAggregatedStat(tweetID, statType)
    if err != nil {
        // Fallback to local count
        return repo.BaseRepo.Count(tweetID)
    }
    return count, nil
}
return repo.BaseRepo.Count(tweetID)
```

This ensures:
- Graceful degradation if CRDT unavailable
- Backward compatibility with existing code
- Resilience to CRDT failures

## Performance Considerations

### Space Complexity
- **Per statistic**: `O(n)` where n = number of nodes that have updated it
- **Total**: Grows with active nodes, not with number of operations
- **Compaction**: Future improvement to merge old node entries

### Time Complexity
- **Update**: `O(1)` - single key update
- **Query**: `O(n)` - sum across n nodes
- **Sync**: `O(log n)` - Merkle-DAG sync via IPFS

### Network Usage
- **Updates**: Broadcast via PubSub (configurable gossip factor)
- **Bandwidth**: Minimal - only deltas are transmitted
- **Optimization**: Rebroadcast disabled, manual sync control

## Testing

### Unit Tests
```bash
go test -v ./core/crdt/...
go test -v ./database/crdt-*_test.go
```

### Integration Tests
```bash
# Test with multiple nodes
go test -v -run TestCRDTLikeRepo_MultipleNodes
```

### Load Testing
```bash
# Simulate concurrent updates
go test -v -bench=BenchmarkCRDT
```

## Future Improvements

1. **Compaction**: Periodic merging of old node entries
2. **Garbage Collection**: Remove entries for inactive nodes
3. **Selective Sync**: Only sync statistics for viewed tweets
4. **Caching**: LRU cache for frequently accessed stats
5. **Metrics**: Prometheus metrics for CRDT operations
6. **Advanced CRDTs**:
   - OR-Set for user lists (who liked/retweeted)
   - LWW-Register for last view timestamps
   - PN-Counter if we need decrements without local tracking

## References

- [IPFS go-ds-crdt](https://github.com/ipfs/go-ds-crdt)
- [Merkle-CRDTs Paper](https://arxiv.org/abs/2004.00107)
- [CRDT Overview](https://crdt.tech/)
- [libp2p PubSub](https://docs.libp2p.io/concepts/pubsub/overview/)

## Troubleshooting

### Stats not syncing between nodes

1. Check PubSub connectivity:
   ```go
   peers := pubsub.ListPeers(crdt.StatsTopicPrefix)
   log.Printf("Connected peers: %v", peers)
   ```

2. Verify CRDT store is initialized:
   ```go
   if crdtStore == nil {
       log.Error("CRDT store not initialized")
   }
   ```

3. Check for network partition

### Inconsistent counts

1. Allow time for convergence (eventual consistency)
2. Check for node ID collisions
3. Verify PubSub messages are being received

### High memory usage

1. Implement compaction
2. Limit number of tracked statistics
3. Use selective sync for popular tweets only

## Security Considerations

- **DoS Protection**: Rate limit stat updates per node
- **Sybil Resistance**: One node ID per actual node
- **Spam Prevention**: Validate user actions locally before CRDT update
- **Data Integrity**: CRDT updates signed with node identity

## License

AGPL-3.0-or-later
