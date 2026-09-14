# Node rating — design and implementation plan

Status: implemented. The storage layer (`core/crdt/ratingstore`,
`database/rating-repo.go`, `domain.RatingRecord`), the engine
(`core/rating`), the fan-outs that feed it, the enforcement points that act
on what it concludes, and the routes that show it are all in the tree.
§7 (the discovery amplification fixes) and the warpdroid side remain. Where
the implementation departed from the original plan, the section says so and
why.

A node's rating is an inherent property of every Warpnet node, computed and
stored **by its neighbours** — never by itself — replicated over CRDT, decaying
back to full trust over time, and fed back into libp2p, the DHT, gossipsub and
the per-peer rate limiters through callbacks.

This document is the complete specification: every type, every constant,
every edit point, every test, across all stages.

---

## 1. Decisions already taken

| # | Question | Decision |
|---|---|---|
| 1 | Trust model | **Hybrid.** Signed first-hand observations in CRDT. Enforcement uses each node's own *subjective* score (first-hand at full weight, remote observers weighted and capped). A separate unweighted *public aggregate* is display-only and never enforces. |
| 2 | What a low rating may do | **Priority (ConnManager + DHT filters) and gossipsub peer score, plus tightening of per-peer rate limits.** No refusal of routes. No automatic blocklisting. |
| 3 | Relays and moderators | **Observations go to the network, state stays in memory.** No persistent store is added; they do hold a CRDT replica on an in-memory datastore, because that replica is the only thing that gives their view back after a restart. |
| 4 | Order of work | **Staged: network → application → moderation**, preceded by a no-behaviour-change prep stage. Stage 1 also carries the discovery-amplification fixes. |

---

## 2. What already exists

The plan is deliberately built on machinery already in the tree.

| Existing | File | Reused for |
|---|---|---|
| PN-counter over `go-ds-crdt`, single-writer generation-tagged keys, bitswap/DAG wiring | `core/crdt/statsstore` | `ratingstore.Store` is built the same way, in a sibling package with its own datastore; it stores signed records, not counters. |
| Gossip broadcaster adapter, topic `/warpnet/stats/1.0.0` | `core/crdt/broadcast` | Takes the topic as an argument, so each store rides one of its own and the two CRDTs never see each other's heads. |
| `UpsertTag` connection priority with a flap LRU | `core/node/priority.go` | Gains a second, independent `rating` tag. |
| Per-`route\|peer` leaky buckets | `core/middleware/rate-limiter.go` | Bucket parameters become a function of the peer's tier. |
| Signature / freshness / private-route checks | `core/middleware/auth.go:54-85` | Main source of first-hand network observations. |
| Payload-size and frame errors | `core/node/node.go:241,246` | Malformed-frame and oversize observations. |
| Exponential blocklist | `database/node-repo.go:703` | Left alone. Rating does **not** drive it. |
| Moderator spot-check ledger, `Outcome`/`Standing` | `cmd/node/moderator/audit/ledger.go` | Becomes the moderation dimension's observation source. |
| Verdict signature verification | `core/handler/moderation.go:103-116` | Moderation observations about moderators. |
| `dht.AddPeerCallbacks` / `RemovePeerCallbacks` | `core/dht/options.go:43-53` | Joined by rating-aware query/routing-table filters. |
| `pubsub.WithPeerScore` + `AppSpecificScore` | `vendor/github.com/libp2p/go-libp2p-pubsub/gossipsub.go:367` | The libp2p callback the rating drives. Currently unused — `NewGossipSub` is called with no options at `core/pubsub/gossip.go:221`. |
| `dht.QueryFilter` / `dht.RoutingTableFilter` | `vendor/github.com/libp2p/go-libp2p-kad-dht/dht_options.go:271,280` | The DHT callbacks the rating drives. Currently unused. |

No rating, reputation or trust-score code exists today. `audit.Ledger` is the
closest thing, is explicitly local and in-memory, and is wired to nothing —
`cmd/node/moderator/audit/doc.go` says so and explains why.

---

## 3. The model

### 3.1 Dimensions and roles

```go
// core/rating/rating.go
package rating

type Dimension uint8

const (
    Network     Dimension = iota // every node type
    Application                  // member nodes
    Moderation                   // moderator nodes
)

func (d Dimension) String() string
func (d Dimension) HalfLife() time.Duration   // 12 h on Network, 7 d elsewhere
func (d Dimension) Retention() time.Duration  // eight half-lives
func ParseDimension(s string) (Dimension, bool)

// Dimensions maps warpnet.NodeInfo.Type to the axes that node tracks.
//   warpnet.RelayNode     -> {Network}
//   warpnet.MemberNode    -> {Network, Application}
//   warpnet.ModeratorNode -> {Network, Application, Moderation}
//   unknown               -> {Network}
func Dimensions(nodeType string) []Dimension
```

A node writes only the dimensions its own role can witness, and reads only the
dimensions of the subject's role. A relay observing a member still only ever
writes `Network`.

### 3.2 Score and tiers

```go
type Score int32

const (
    MaxScore Score = 1000 // a node never seen before: full trust
    MinScore Score = 0
)

type Tier uint8

const (
    TierTrusted  Tier = iota // 800..1000  no effect
    TierWatched              // 500..799   mild deprioritisation
    TierDegraded             // 200..499   halved rate limits, low priority
    TierFloor                // 0..199     minimum priority, gossipsub graylist range
)

func (s Score) Tier() Tier
func (t Tier) String() string
```

A node's overall score is the **minimum** across the dimensions its role tracks:
a moderator that is clean on the wire but hands out forged verdicts is not a
"mostly fine" node.

### 3.3 New nodes start at maximum

No probation period, by requirement. A subject with no observations scores
`MaxScore`. Deliberate Sybil trade-off: identity is free, so probation would only
punish honest newcomers while a patient attacker waits it out. The protection
lives in the enforcement ceiling (§6.4) instead.

### 3.4 Decay — self-healing

Observations land in hour buckets. Score is a pure read-time function:

```
penalty(subject, dim) = Σ  weight(kind) × count × 2^( -age_hours / halfLife(dim) )
                       obs

score(subject, dim)   = clamp(MaxScore - penalty, MinScore, MaxScore)
```

```go
// core/rating/rating.go
func (d Dimension) HalfLife() time.Duration
func (d Dimension) Retention() time.Duration // eight half-lives: older records are
                                             // ignored on read and deleted by their author
func (d Dimension) decay(age time.Duration) float64

// core/rating/entries.go — one peer's records and the arithmetic over them
type bucket int64                              // unix hour
func bucketAt(t time.Time) bucket
func (b bucket) start() time.Time

type entries []entry
func (es entries) penalty(dim Dimension, now time.Time) Score
func (es entries) median(dim Dimension, now time.Time) (Score, int)
func (es entries) tallies(dim Dimension) []domain.OffenceTally
```

| Dimension | Half-life | Retention | Rationale |
|---|---|---|---|
| `Network` | 12 h | 4 d | transport misbehaviour is often a bad build or a bad link; recover fast |
| `Application` | 7 d | 56 d | an upheld moderation verdict should outlive a news cycle |
| `Moderation` | 7 d | 56 d | a moderator's standing must not be washable overnight |

Read-time decay means no background sweeper, no rewrite traffic, no clock skew
changing anyone's *stored* data, and identical results on every node given the
same observation set.

---

## 4. Offence catalogue

```go
// core/rating/offence.go
type Kind uint16

const (
    // network
    KindBadSignature Kind = iota + 1
    KindMissingSignature
    KindMalformedFrame
    KindOversizePayload
    KindStaleOrReplayed
    KindPrivateRouteDenied
    KindRateLimitHit
    KindDiscoveryFlood
    KindConnectionFlap
    KindDialFailure
    KindForgedRecord
    // application
    KindModerationUpheld
    KindForeignAuthorship
    KindWriteFlood
    KindFalseReportBurst
    // moderation
    KindVerdictMalformed
    KindVerdictOutlier
    KindAuditWrong
    KindAuditInvalid
    KindAuditUnreachable
)

type offence struct {
    dim    Dimension
    weight int32
    // ceiling is the most this kind may ever contribute to one
    // (subject, observer, dimension) penalty, before decay. Zero means
    // no ceiling. Liveness-ish kinds carry one so a flaky link can
    // never on its own push a peer out of TierWatched.
    ceiling int32
}

var catalogue = map[Kind]offence{ /* table below */ }

func (k Kind) Dimension() Dimension
func (k Kind) Weight() int32
func (k Kind) Ceiling() int32
func (k Kind) String() string   // stable wire/UI name, e.g. "bad_signature"
func (k Kind) Valid() bool
func ParseKind(name string) (Kind, bool)
```

### 4.1 Network — every node type

| Kind | Weight | Ceiling | Detection site |
|---|---|---|---|
| `KindBadSignature` | 250 | — | `core/middleware/auth.go:69` |
| `KindMissingSignature` | 250 | — | `core/middleware/auth.go:59` |
| `KindMalformedFrame` | 120 | — | `core/middleware/auth.go:54`, `core/node/node.go:246` |
| `KindOversizePayload` | 150 | — | `core/node/node.go:241` (`stream.ErrPayloadTooLarge`) |
| `KindStaleOrReplayed` | 120 | — | `core/middleware/auth.go:76` |
| `KindPrivateRouteDenied` | 200 | — | `core/middleware/auth.go:82` |
| `KindRateLimitHit` | 15 | 300 | `core/middleware/rate-limiter.go:109` |
| `KindDiscoveryFlood` | 25 | 300 | new per-peer discovery bucket (§7d) |
| `KindConnectionFlap` | 10 | 200 | `core/node/priority.go` flap LRU |
| `KindDialFailure` | 2 | 100 | `core/discovery/discovery.go:268` |
| `KindForgedRecord` | 400 | — | a correctly signed rating record that is structurally illegal — self-rating, wrong-dimension kind, back-dated bucket (§5.4) |

`KindBadSignature`, `KindMissingSignature` and `KindPrivateRouteDenied` are the
only network kinds that are self-evidently deliberate. They carry the weights
that reach `TierFloor` quickly, and only from first-hand evidence.

### 4.2 Application — member nodes

| Kind | Weight | Ceiling | Source |
|---|---|---|---|
| `KindModerationUpheld` | 300 | — | the moderator that carried a decided round, in `Moderator.Decided`: the report it judged already names the offending node, so one verdict is one observation rather than one per node it reaches |
| `KindForeignAuthorship` | 350 | — | `warpnet.VerifyAuthorship` → `ErrForeignAuthor` |
| `KindWriteFlood` | 20 | 300 | sustained rate-limit hits on write routes |
| `KindFalseReportBurst` | 60 | 300 | reports from this node the quorum cleared, counted only above a per-window threshold |

### 4.3 Moderation — moderator nodes

| Kind | Weight | Ceiling | Source |
|---|---|---|---|
| `KindVerdictMalformed` | 200 | — | missing object/user id, or an unknown type, on a verdict whose signature already verified |
| `KindVerdictOutlier` | 60 | 400 | ballot disagreeing with the round's own outcome |
| `KindAuditWrong` | 60 | 400 | the audit ledger's standing worsening to Suspect |
| `KindAuditInvalid` | 500 | — | the audit ledger's standing worsening to Banned |
| `KindAuditUnreachable` | 5 | 100 | `audit.OutcomeUnreachable`, per probe |

**Three kinds from the plan were dropped rather than shipped inert.**

`VerdictBadSignature` and `VerdictNoModeratorID` are not chargeable. A
verdict that fails verification names a moderator that may have had
nothing to do with it, and verdicts travel by pubsub, so there is no
relaying peer to charge either. The only honest response is to drop it.
Everything that remains is chargeable precisely because the signature
verified first, which proves authorship.

`VerdictUnsolicited` has no detection site: a round has no eligibility
gate by design — the volunteer order in `cmd/node/moderator/round` is a
delay, not a permission — so voting early is allowed and there is
nothing to charge.

All weights are calibration targets.

---

## 5. Storage

### 5.1 Why CRDT — restart recovery for stateless nodes

Every node type joins the rating CRDT. That is the whole point of using one
here: relays and moderators hold no disk, so **the CRDT is what lets them
survive a restart**. A stateless node loses its entire local view when the
process dies; on the next start the DAG replays it back from peers, exactly the
"total local-data loss" recovery path `core/crdt/statsstore` already documents for
counters. Without CRDT a relay would be permanently memoryless and its
observations would die with it.

Rating has **its own** `go-ds-crdt` datastore, `ratingstore.Store`, built
exactly like `statsstore.Store`: its own backing datastore, its own blockstore
and bitswap exchange, its own gossip topic. A go-ds-crdt instance owns its
whole namespace and its set of heads, so two of them cannot share a backing
store or a topic without merging each other's deltas.

The two are separate packages that share nothing. Each declares the
interfaces it needs — `Broadcaster`, `Datastore`, `Router` — and owns the
topic its replicas converge on. Neither imports the other, and neither
imports an umbrella package above it: the broadcaster they both ride is a
leaf package, `core/crdt/broadcast`, that knows nothing of either tenant.
Only the node assembly depends on all three.

| Node type | Backing datastore | Survives restart via |
|---|---|---|
| member | `database.NewRatingRepo(db)`, Badger-backed, prefix `RATING`, beside the stats repo's `CRDT` | its own disk, plus the DAG for anything it missed while down |
| relay | a `datastore.NewMapDatastore()` of its own | the DAG only |
| moderator | a `datastore.NewMapDatastore()` of its own | the DAG only |

Topics:

```go
// core/crdt/broadcast — one broadcaster, a topic per store
func NewGossip(ctx context.Context, gossip GossipPubSuber, topic string) (*Gossip, error)

statsstore.GossipTopic  // "/warpnet/stats/1.0.0"
ratingstore.GossipTopic // "/warpnet/rating/1.0.0"
```

**Why rating is not the stats store itself.** `statsstore.Store` is a PN-counter:
one `uint64` per key, merged by summing. A rating record is signed — observer,
dimension, hour bucket, per-kind counts, signature — and has to be verified
before it is believed and re-signed whenever it changes. A counter cannot carry
a signature, and a node that could bump another node's counter directly is
exactly the forgery the signature exists to stop.

**Where the layers meet.** `core/crdt/ratingstore` is the database layer: it owns
the key schema, the JSON encoding, the replication wiring and the merge hooks,
and it enforces that a node writes and deletes **only its own records**
(`Put` refuses a foreign `ObserverID`, `DeleteExpired` skips foreign keys).
`core/rating` is the engine: signing, verification, validation, indexing and
scoring. The two share `domain.RatingRecord` and nothing else; `core/rating`
declares the `Storer` interface it needs and `*ratingstore.Store` satisfies
it, so neither package imports the other.

### 5.2 Key layout

```
/record/{peerID}/{observerID}/{dimension}/{bucketHour}/{generation}
```

Built and parsed only in `core/crdt/ratingstore`; the engine never sees a key.
The repository name appears once, in `database.NewRatingRepo`, which hands the
store a datastore already namespaced under `RATING`; repeating it in the key
would write it into every Badger key twice.
`List(peerID)` is one prefix query, and a value whose content disagrees with
the key it sits under is dropped on read.

`{generation}` is a fresh 128-bit nonce minted once per engine start — the same
device `core/crdt/statsstore` uses, and for the same reason, only more acutely
here. A stateless relay restarts with an empty datastore and starts observing
immediately. Without a generation segment its first write of bucket B (count 1)
would land on the same key as the count-50 record the DAG is still replaying, and
LWW would silently destroy the replayed history — the exact failure the CRDT is
supposed to prevent. With it, the new process owns a key no past process can
collide with, the replayed records survive verbatim, and the reader **sums
across generations** within a bucket. The generation is part of the signed
content, which is why the signer mints it rather than the store.

Consequences:

- No read-before-write anywhere. Each `(peer, observer, dim, bucket,
  generation)` tuple has exactly one writer for its whole lifetime, so the
  in-memory count is always authoritative and eventual-consistency lag cannot
  lose an observation.
- Key growth is bounded by `restarts × buckets actually written`, not by
  restarts alone: a generation only appears under buckets that process really
  observed something in.
- Author-side GC (§5.5) deletes whole expired buckets including all their
  generations, so restart churn does not accumulate past the retention window.

### 5.3 Record

```go
// domain/rating.go — the persisted unit, like domain.Tweet
type RatingRecord struct {
    PeerID     string         `json:"peer_id"`
    ObserverID string         `json:"observer_id"`
    Dimension  string         `json:"dimension"`   // "net" | "app" | "mod"
    Bucket     int64          `json:"bucket"`      // unix hour
    Generation string         `json:"generation"`  // hex(16 random bytes), one per engine start
    Offences   []OffenceCount `json:"offences"`    // ascending by kind name
    UpdatedAt  time.Time      `json:"updated_at"`
    Signature  string         `json:"signature"`   // observer's ed25519 over the signing bytes
}

type OffenceCount struct {
    Kind  string `json:"kind"`  // stable name, e.g. "bad_signature"
    Count uint32 `json:"count"`
}
```

Kinds and dimensions travel by name, not by number, so a renumbered enum can
never silently change what an old record means.

```go
// core/rating/record.go — the persisted shape with the engine's behaviour
// attached; unexported, the engine is the only user
type record domain.RatingRecord

// signing bytes, canonical and stable across architectures:
//   peer "|" observer "|" dimension "|" itoa(bucket) "|" generation "|"
//   for each offence in ascending kind-name order: kind "=" itoa(count) ","
//   "|" itoa(updatedAt.UnixMilli())
func (r record) signingBytes() []byte
func (r *record) sign(priv ed25519.PrivateKey) error

// verify derives the pubkey from the observer peer id and checks the
// signature — the same trick StreamModerationResultHandler uses.
func (r record) verify() error

// validate enforces the structural rules, independent of signature:
//   - PeerID and ObserverID parse as peer ids, PeerID != ObserverID
//   - Dimension is known; every kind is known and belongs to it
//   - Generation is 32 hex characters; Offences is not empty
//   - Bucket is not in the future beyond one bucket, not older than retention
func (r record) validate(now time.Time) error
```

Properties this buys:

- **One writer per key, for the key's whole lifetime.** Only `ObserverID` writes
  `.../{observerId}/...`, the store refuses anything else, and only one process
  ever owns a given `{generation}`. LWW inside the key is therefore trivially
  safe: no read-modify-write, no eventual-consistency window, and no way for a
  restarted process to clobber its own replayed history (§5.2).
- **`PeerID == ObserverID` is invalid** and dropped on read. A node cannot rate
  itself by construction, not by convention.
- **Restart-safe by the same argument as the stats store.** A stateless node
  that comes back with an empty datastore mints a fresh generation, starts a new
  sub-counter, and the DAG layers its old generations back underneath. Reader
  sums them.

### 5.4 Authenticity

Every record is verified before it enters the index, both when a peer is loaded
from the store and when the store's put hook delivers a merged record. The hook
only *updates* peers the index already holds: creating one from a single delta
would shadow the rest of its history in the store, so an unindexed peer is
instead loaded whole on its next read. Three distinct failures:

- **`verifyRecord` fails** — the signature does not match the pubkey derived
  from the claimed observer. Nobody is attributable: anyone can forge a claim
  naming any observer. Dropped silently. Over CRDT there is no relaying peer to
  charge, so nothing is charged.
- **verifies, but breaks a structural rule** — self-rated, a kind from another
  dimension, a malformed generation, empty offences. The signature proves the
  named observer really authored it, so the observer earns `KindForgedRecord`
  — **once, when the record arrives**. A forgery stays in the store forever
  (only its author could delete it), so reloading its victim must not charge
  the author again.
- **verifies, but is outside the time window** — a bucket past retention or
  more than one bucket in the future. That is a late replica or a bad clock,
  not forgery: dropped, nobody charged.

This is the one place the CRDT transport is weaker than a point-to-point one:
authenticity survives any relay path, but blame for unsigned garbage does not.

### 5.5 Size and GC

Records exist only where something actually happened — empty buckets are never
written — so the dataset is proportional to observed misbehaviour, not to N².
Per node: `subjects × observers × dimensions × buckets_retained × generations`.
A node misbehaving continuously for a week against 50 observers produces ~8k
records, single-digit MB. Idle peers cost zero bytes.

```go
// core/crdt/ratingstore — deletes only this node's own keys of one dimension
func (s *Store) DeleteExpired(dimension string, beforeBucket int64) error

// core/rating/engine.go — on the flush ticker, at most once per hour,
// once per dimension the node witnesses, with that dimension's retention
func (e *Engine) gc()
```

Only the author deletes its own records, so one node can never erase another's
evidence — and a CRDT delete is a tombstone that propagates, which is precisely
why a node must never prune foreign records to save memory. The in-memory index
is bounded instead (§6.1); the datastore is bounded only by retention.

---

## 6. Aggregation and enforcement

### 6.1 The in-memory index — why scoring is not a CRDT query

Scoring runs on the rate-limiter hot path, i.e. once per inbound request. A
prefix query per request is not acceptable. The engine therefore keeps an
in-memory index and answers scores from arithmetic only:

```go
// core/rating/indexer.go
type indexer struct {
    peers *lru.Cache[string, *indexedPeer] // peer -> its complete record set + memoised score
}

// core/rating/engine.go — scoring is the engine's, because it depends on
// who this node is, whom it trusts and whom it is connected to
func (e *Engine) score(es entries, dim Dimension, now time.Time) Score     // local, enforced
func (e *Engine) firstHand(es entries, dim Dimension, now time.Time) Score // own evidence only
func (e *Engine) weight(observer string) float64                           // firstHand(observer) / MaxScore
func (e *Engine) acquainted(observer string) bool                          // connected >= 1 h
```

- **Loaded lazily, one peer at a time.** The first score of a peer is one
  `Storer.List(peerID)`; there is no startup scan. A peer nobody has observed
  is indexed empty, so it is not re-queried on every request.
- **Kept current by the store's merge hooks.** A merged record updates the
  peer's slot if the peer is indexed; a deletion forgets the peer, which is
  then reloaded whole on its next read.
- **Memoised.** Each peer carries its last score, revalidated for 15 s while
  its record set is unchanged; any update or merge invalidates it.
- Records failing verification or validation never enter the index.
- **Bounded by peer count, not by dataset size.** Above `maxIndexedPeers`
  (16k) the least-recently-scored peer is evicted from the index. Eviction
  is index-only — it never deletes from the CRDT, because a CRDT delete is a
  tombstone that would propagate and destroy other nodes' evidence (§5.5). An
  evicted peer simply falls back to one prefix query on its next scoring and
  re-enters the index. This is what keeps a stateless relay's memory flat
  regardless of how large the replicated dataset grows.

### 6.2 Two numbers

**Local (subjective) score — the only one that enforces.**

```
penalty_local(subject, dim) =
      Σ decayed(own observations)                           // full weight, uncapped
  +   min( Σ_i min( decayed(obs_i) × w(observer_i), capPerObserver ),
           capRemoteTotal )

w(observer) = score_local(observer) / MaxScore
```

```go
// core/rating/engine.go
const (
    capPerObserver Score = 150
    capRemoteTotal Score = 400
    // minAcquaintance: a remote observer's records are ignored until we
    // have been connected to it for this long in this session. A
    // drive-by accuser has no voice.
    minAcquaintance = time.Hour
)
```

`w(observer)` is computed from that observer's **first-hand-only** local score to
keep the recursion one level deep and terminating.

**Public (aggregate) score — display only.** Unweighted median across observers
per dimension. Shown to the node's own user and in peer detail views. It never
touches a rate limiter, a priority tag or a peer score.

### 6.3 The invariant that makes slander survivable

`capRemoteTotal = 400` means **remote observations alone can never push a peer
below 600** — the bottom of `TierWatched`. Reaching `TierDegraded` or
`TierFloor` requires first-hand evidence gathered on our own wire.

Consequence: a coordinated slander campaign against an honest node costs it a
mild priority drop and nothing else, on every node that has not itself witnessed
a problem. A genuinely misbehaving node hits the floor on exactly the peers it is
misbehaving against, which is where enforcement matters. This invariant gets its
own test (§10, Stage 1).

### 6.4 Where the score is applied

```go
// core/rating/enforce.go — pure mappings on the tier, no dependencies
func (t Tier) ConnTag() int            // 60 / 30 / 10 / 1
func (t Tier) GossipScore() float64    // 0 / -10 / -60 / -200
func (t Tier) RateMultiplier() float64 // 1.0 / 0.5 / 0.25 / 0.1
func (t Tier) IsAllowedInDHT() bool    // false only for TierFloor
```

A module never reads a score, and none of them keeps rating state of its
own. The engine records each peer's tier in one `rating.PeersRatings`,
and every module asks it the single question it acts on:

```go
// core/rating — the engine writes here as ratings move
func NewPeersRatings() *PeersRatings
func (r *PeersRatings) Rate(peerID warpnet.WarpPeerID, tier Tier)
func (r *PeersRatings) Tier(peerID warpnet.WarpPeerID) Tier
func (r *PeersRatings) ConnTag(peerID warpnet.WarpPeerID) int
func (r *PeersRatings) GossipScore(peerID warpnet.WarpPeerID) float64
func (r *PeersRatings) RateMultiplier(peerID warpnet.WarpPeerID) float64
func (r *PeersRatings) IsAllowedInDHT(peerID warpnet.WarpPeerID) bool

type RatingsCollector interface{ Rate(peerID warpnet.WarpPeerID, tier Tier) }
func WithRatings(ratings RatingsCollector) Option   // the whole wiring
```

Each module declares a `PeersRatings` of its own, with the one method it
asks, and imports nothing of the rating to ask it:

| module | asks | for |
|---|---|---|
| `core/middleware` | `RateMultiplier` | how much of a route this peer may spend |
| `core/pubsub` | `GossipScore` | what gossipsub should weigh it by |
| `core/dht` | `IsAllowedInDHT` | whether the routing table may hold it |
| `core/node` | `ConnTag` | what it is worth to the connection manager |

A node with no ratings serves, scores, routes and keeps every peer in
full: each of those readers answers for it. The connection tag is the one
that is pushed rather than read, because libp2p holds it: the node sets
it whenever a peer connects.

The pass is also what notices a rating that recovered: evidence decays,
so a peer improves with no event to announce it. A peer nobody has rated
is never recorded, and every module treats it as trusted.

| Surface | Change | File |
|---|---|---|
| ConnManager | `SetRatingPriority(pid, tag)` writes a **separate** `rating` tag, kept distinct from the existing `reachability` tag so the two compose additively as libp2p intends rather than overwriting each other. It ignores the flap window: a standing already changes slowly. | `core/node/priority.go` |
| gossipsub | `pubsub.NewGossipSub` gains `pubsub.WithPeerScore(params, thresholds)`, with `AppSpecificScore` reading the score the last standing left in the gossip's own cache; `GraylistThreshold: -100`. Per §6.3 only first-hand evidence reaches the graylist range. | `core/pubsub/gossip.go` |
| DHT | `dht.QueryFilter` and `dht.RoutingTableFilter` refuse the peers a standing put at the floor, and let them back in when it recovers. A peer nobody has rated is admitted. | `core/dht/dht.go` |
| Rate limits | a peer's buckets are built at the allowance its standing carries, `burst` and `perMinute` floored at 1 so no peer is ever starved outright. A standing that changes drops the peer's buckets, so it is measured against the allowance it has now. | `core/middleware/rate-limiter.go`, `core/middleware/middleware.go` |
| Moderation ballots | **Nothing.** Weighting a vote round's ballots by rating was planned and rejected: `planTally` must be a pure function of the ballots so every participant reaches the same answer, and a locally-held rating is not. See Stage 3. | `cmd/node/moderator/round/` |
| Discovery | the new per-peer discovery bucket (§7d) is scaled by the same multiplier, so an offender's discovery entries are dropped first under pressure. | `core/discovery/rate-limiter.go` |

### 6.5 Deliberately not done

- **No automatic blocklisting.** `BlocklistExponential` stays a user/operator
  action. A slandered node must never be cut off by an automatic process.
- **No route refusal.** A `TierFloor` peer is served slowly and last, never told
  "no".
- **No rating field in `NodeInfo`.** A node self-reporting its rating is
  worthless. "Rating is an inherent property of a node" is realised by every
  node type owning a rating store in its core and every peer having a score in
  it — not by a field on the wire.

---

## 7. Discovery self-amplification — prerequisite for Stage 1

The current discovery path makes every node generate traffic its peers would
score as flooding. These must land with Stage 1, or rating will penalise honest
nodes.

| # | Problem | Site | Fix |
|---|---|---|---|
| a | Answering `PUBLIC_GET_INFO` enqueues the requester for discovery, which requests *its* info back. `DiscoveryHandlerStream` short-circuits only when the peerstore already holds addrs, which is false on first contact — so every first contact costs an info ping-pong. | `core/handler/info.go:56`, `core/discovery/discovery.go:202` | Do not enqueue from the info handler for an already-connected peer; the connection is the discovery. |
| b | `handleAsMember` issues `requestNodeInfo` on **every** discovery event, including for peers already connected and already known. | `core/discovery/discovery.go:290` | Per-peer "recently probed" LRU, 30 min TTL, in front of `requestNodeInfo`; skip entirely when connected and the user row is fresh. |
| c | `publishPeerInfo` republishes up to 11 AddrInfos every 5 min, every topic is `topic.Relay()`-ed, and receivers treat every entry as a fresh discovery — O(N²) info requests network-wide. | `core/pubsub/gossip.go:534`, `:274` | Publish own AddrInfo plus only recently *verified* peers; carry a monotonic epoch so receivers drop repeats; receivers skip entries already in the peerstore. |
| d | The discovery leaky bucket is **global** — `newRateLimiter(32, 2)`, ~12/min for the whole service. It cannot tell "12 new peers" from "one peer 12 times", and one chatty peer starves discovery for everyone. | `core/discovery/discovery.go:129,224` | Per-source buckets plus a per-peer dedup LRU in front; per-peer bucket scaled by tier (§6.4). This is where `KindDiscoveryFlood` is raised. |
| e | The DHT `PeerAdded` hook runs `d.dht.FindPeer(ctx, id)` — a full DHT walk per routing-table insert — purely to log addresses. | `core/dht/dht.go:146` | Drop the `FindPeer`; log the id. Move callbacks off the routing-table hook onto a buffered channel so a slow callback cannot stall the table. |
| f | Discovery dials with `SimpleConnect` (raw `host.Connect`), bypassing `WarpNode.Connect`'s backoff, so a dead peer republished by gossip is redialled forever. | `core/discovery/discovery.go:262`, `core/node/node.go:181` | Route discovery dials through the backoff-aware path. |

Each is an independent commit with its own regression test.

---

## 8. Wire surface

```go
// event/paths.go
PRIVATE_GET_RATING = "/private/get/rating/0.0.0"
PUBLIC_GET_RATING  = "/public/get/rating/0.0.0"
```

```go
// event/event.go
type GetRatingEvent struct {
    NodeId string `json:"node_id"` // empty on the private route = self
}
```

```go
// domain/rating.go
type NodeRating struct {
    NodeID     string            `json:"node_id"`
    Overall    int32             `json:"overall"`
    Tier       string            `json:"tier"`
    Dimensions []DimensionRating `json:"dimensions"`
    Observers  int               `json:"observers"`
    UpdatedAt  time.Time         `json:"updated_at"`
}

type DimensionRating struct {
    Name   string         `json:"name"`
    Score  int32          `json:"score"`
    Tier   string         `json:"tier"`
    Recent []OffenceTally `json:"recent"`
}

type OffenceTally struct {
    Kind   string    `json:"kind"`
    Count  uint32    `json:"count"`
    LastAt time.Time `json:"last_at"`
}
```

- `PRIVATE_GET_RATING` → the owner's **public aggregate** for their own node,
  read from `/record/{self}/*`, i.e. entirely from records written by others.
  The node's subjective view of itself is empty by construction.
  `Recent` is what makes the feature useful: "37 rate-limit hits and 4 malformed
  frames in the last 6 hours" tells the user what to fix.
- `PUBLIC_GET_RATING` → this node's signed view of a given subject. Needed for
  thin clients, which hold no CRDT replica at all, and later for quorum work.
  Full nodes do not need it — they read the CRDT (§5.1) — so it is a
  convenience route, never a dependency of the rating mechanism itself.
  Rate-limited under `limitRead`.
- Route limits: add both to `routeLimits` in `core/middleware/rate-limiter.go`
  (`limitRead`).

UI:
- `frontend/src/views/Settings/Rating.vue`, beside `Blocks.vue`/`Mutes.vue`;
  router entry in `frontend/src/router`; call added to
  `frontend/src/service/service.js`.
- Compact badge in `frontend/src/components/InfoOverlay.vue`.
- warpdroid is out of scope; `PUBLIC_GET_RATING` is shaped so it can be added
  later with no protocol change.

---

## 9. Engine API and per-node wiring

```go
// core/rating/engine.go

// Storer is what the engine needs from the database layer;
// *ratingstore.Store satisfies it.
type Storer interface {
    Put(rec domain.RatingRecord) error                     // own records only
    List(peerID string) ([]domain.RatingRecord, error)     // own and foreign
    DeleteExpired(dimension string, beforeBucket int64) error
    OnPut(hook func(domain.RatingRecord))                  // every merged record
    OnDelete(hook func(domain.RatingRecord))               // key fields only
}

// ConnectionsProvider gates remote observers by acquaintance;
// the node's libp2p network satisfies it.
type ConnectionsProvider interface {
    ConnsToPeer(id warpnet.WarpPeerID) []network.Conn
}

func NewEngine(
    ctx context.Context,
    store Storer,
    conns ConnectionsProvider,
    privKey ed25519.PrivateKey, // self and the record signature both derive from it
    nodeType string,            // warpnet.MemberNode | RelayNode | ModeratorNode -> dimensions
    opts ...Option,             // WithClock, WithFlushInterval: for tests
) (*Engine, error)

// write path — non-blocking, buffered, folded into hour buckets, signed and
// written every 30 s. Misuse (an unknown kind, a dimension this role cannot
// witness) is a bug at the call site, so it is logged, not returned.
func (e *Engine) Record(peerID warpnet.WarpPeerID, kind Kind)

// read path — in-memory arithmetic, memoised per peer. Fail-open: a peer
// whose records cannot be read scores MaxScore, because an enforcement
// point must not act on evidence it has not seen. Enforcement points act
// on Score(peer).Tier().
func (e *Engine) Score(peerID warpnet.WarpPeerID) Score

// display — the public aggregate (§6.2) and what the network says about us
func (e *Engine) View(peerID warpnet.WarpPeerID) (domain.NodeRating, error)
func (e *Engine) Own() (domain.NodeRating, error)

func (e *Engine) Close() error // final flush; the store is closed by whoever built it
```

A nil `*Engine` is safe on every method and penalises nobody: that is the
"rating not built" state.

Consumers depend on the engine the way the rest of the tree depends on
anything — through an interface they declare themselves with the one or two
methods they call (`Score` for an enforcement point, `Record` for a detection
site, `View`/`Own` for the handlers). There is no shared handle object and
nothing to inject before the engine exists: `core/node` already exposes
`WarpNode.Event()`, and the intended write path is a consumer goroutine that
turns those events into `Record` calls.

Wiring — identical shape on all three, differing only in the backing datastore
and the node type:

| Node | Dimensions | Backing datastore | Gossip source |
|---|---|---|---|
| **member** (`cmd/node/member/node/member-node.go`) | `Network`, `Application` | `database.NewRatingRepo(db)` | `m.pubsubService.Gossip()` |
| **relay** (`cmd/node/relay/node/relay-node.go`) | `Network` | a `datastore.NewMapDatastore()` of its own | needs a `Gossip()` accessor on `cmd/node/relay/pubsub` |
| **moderator** (`cmd/node/moderator/node/moderator-node.go`) | `Network`, `Application`, `Moderation` | a `datastore.NewMapDatastore()` of its own | `cmd/node/moderator/pubsub/publisher.go` wraps a `*pubsub.Gossip`; add a `Gossip()` accessor |

```go
broadcaster, err := broadcast.NewGossip(ctx, gossip, ratingstore.GossipTopic)
store, err := ratingstore.New(ctx, broadcaster, ratingRepo, node.Node(), dHashTable)
engine, err := rating.NewEngine(ctx, store, node.Node().Network(), privKey, warpnet.MemberNode)
// ... on Stop: engine.Close() first, then store.Close()
```

Ordering constraint on all three: the store must be constructed after gossip
is running, the same ordering the member node already uses for the stats store.

No config: rating has no modes and no switch. A node cannot opt out of
being rated by its neighbours, and a switch for whether it acts on what
it sees would only produce a blind free-rider — which contradicts rating
being an inherent property of a node. The consequences are soft by
design (§6.4), so there is nothing here that needs arming carefully.
Every knob in `enforce.go` is a weighting, not a refusal — `RateMultiplier`
never reaches zero, nothing blocklists — so a mis-set weight costs a peer
latency, and the caps in §6.3 bound how far a wrong number can carry.

---

## 10. Work breakdown

Five stages, each independently reviewable, mergeable and testable.

### Stage 0 — storage layer (landed)

| File | Contents |
|---|---|
| `domain/rating.go` | `RatingRecord`, `OffenceCount` — the persisted unit; `NodeRating`, `DimensionRating`, `OffenceTally` — the wire DTOs |
| `core/crdt/ratingstore` | `Store`: own go-ds-crdt datastore, key schema, encoding, merge hooks, own-records-only writes and deletes |
| `core/crdt/statsstore` | the PN-counter, moved out of `core/crdt` so the two tenants share no package |
| `core/crdt/broadcast` | the gossip broadcaster, moved out of the umbrella package and parametrised by topic |
| `database/rating-repo.go` | `NewRatingRepo(db)` — the member node's Badger-backed datastore for the rating CRDT, prefix `/RATING` |

**Acceptance:** `core/crdt/stats_test.go` passes **unmodified**. `go build ./...`
clean.

### Stage 1 — rating core, network dimension, enforcement, discovery fixes

The constants in §4 and §6 are calibration targets. They are safe to get
wrong: every consequence in `enforce.go` is a weighting rather than a
refusal, so a mis-set weight costs a peer priority and latency, never
service.

New files:

| File | Contents |
|---|---|
| `core/rating/rating.go` | `Dimension` with its half-life and retention, `Score` with its `Tier`, `Dimensions` per node type |
| `core/rating/offence.go` | `Kind`, catalogue, accessors, `ParseKind` |
| `core/rating/record.go` | `record`: signing bytes, sign, verify, validate; generation minting |
| `core/rating/entries.go` | `bucket`, `entries`: decay, generation summing, penalty, median, tallies |
| `core/rating/indexer.go` | lazily loaded per-peer index, memoised scores, LRU eviction |
| `core/rating/enforce.go` | `Tier` methods: the enforcement knobs |
| `core/rating/engine.go` | `Engine`, `Storer`, `ConnectionsProvider`, local scoring, buffered writer, flush, GC, merge hooks |
| `core/handler/rating.go` | `StreamGetOwnRatingHandler`, `StreamGetRatingHandler` over `Engine.View` and `Engine.Own` — not in the storage/engine change |
| `frontend/src/views/Settings/Rating.vue` | own rating, per-dimension bars, recent offences |

The engine and the storage layer of Stage 0 are landed; everything below is
the integration still to do.

Edited files:

| File | Change |
|---|---|
| `event/paths.go` | two new routes |
| `event/event.go` | `GetRatingEvent` |
| `core/middleware/middleware.go` | `NewWarpMiddleware` takes a consumer-declared interface over `*rating.Engine`; `record` filters self-streams and charges through it |
| `core/middleware/auth.go` | `Record` at the five sites in §4.1 |
| `core/middleware/rate-limiter.go` | `Record(KindRateLimitHit)`; `limitForRoute(route, tier)`; bucket carries its tier and is rebuilt on change; register the two new routes under `limitRead` |
| `core/node/node.go` | `Record` on oversize/read error in `unwrap`; offending libp2p events reach the engine through `WarpNode.Event()` |
| `core/node/priority.go` | `rating` tag; `Record(KindConnectionFlap)` |
| `core/pubsub/gossip.go` | `WithPeerScore` + `AppSpecificScore`; fix (c) |
| `core/dht/options.go`, `core/dht/dht.go` | `QueryFilter`/`RoutingTableFilter` options; fix (e) |
| `core/discovery/discovery.go`, `core/discovery/rate-limiter.go` | fixes (b), (d), (f); `KindDiscoveryFlood`, `KindDialFailure` |
| `core/handler/info.go` | fix (a) |
| `cmd/node/member/node/member-node.go`, `types.go` | build the one datastore, put both stores on it, register the private route |
| `cmd/node/relay/node/relay-node.go` | build a `Network`-only store on the `MapDatastore` it already has |
| `cmd/node/moderator/node/moderator-node.go`, `cmd/node/moderator/moderator/moderator.go` | build a store on the `MapDatastore` it already has; `Moderation` observations wired in Stage 3 |
| `frontend/src/router`, `frontend/src/service/service.js`, `frontend/src/views/Settings.vue` | route + link |

Tests:

| Test | Asserts |
|---|---|
| `record_test.go` | signing bytes are canonical and order-independent; `Verify` rejects a foreign signature; `Validate` rejects `Subject == Observer`, unknown kinds, a kind from the wrong dimension, a malformed generation, an out-of-window bucket |
| `record_test.go` | an unsigned/forged record is dropped and charges nobody; a correctly signed but structurally illegal one charges its observer `KindForgedRecord` (§5.4) |
| `aggregate_test.go` | decay is deterministic and monotonic; a record exactly one half-life old contributes half its weight; generations under one bucket are summed, not overwritten; kind ceilings hold; `capPerObserver` and `capRemoteTotal` hold |
| `aggregate_test.go` | **the §6.3 invariant**: any number of remote observers, any number of records, score never < 600 |
| `aggregate_test.go` | first-hand evidence alone reaches `TierFloor` |
| `engine_test.go` | `Record` is non-blocking under a stalled store; buckets fold correctly; flush writes exactly one record per (peer, dim, bucket, generation) |
| `engine_test.go` | **stateless restart recovery**: an engine whose store is wiped, fed its own prior-generation records the way the DAG would replay them, reports the same score as before the wipe, and its new writes do not overwrite the replayed ones |
| `engine_test.go` | eviction never issues a CRDT delete; an evicted peer scores identically after falling back to a prefix query; a forgery is charged once however often its victim is reloaded |
| `core/crdt/ratingstore/store_test.go` | own-records-only writes and deletes; key/value consistency; hooks fire for local and replicated records; every write is broadcast |
| `enforce_test.go` | tier → tag/score/multiplier/DHT mappings; the floor multiplier still serves a peer |
| `rating_test.go` | `Dimensions` per node type; overall = min over dimensions |
| `core/handler/rating_test.go` | own rating excludes self-authored records; `PUBLIC_GET_RATING` response shape |
| `core/discovery/discovery_test.go` | (b) a second discovery event for a known peer issues no `PUBLIC_GET_INFO`; (d) one peer cannot exhaust the global budget; (f) a backoffed peer is not redialled |
| `core/handler/info_test.go` | (a) answering info does not enqueue an already-connected peer |
| `core/pubsub/gossip_test.go` | (c) a repeated epoch is dropped |

End-to-end on testnet, via the `warpnet-testnet-verify` skill:

1. Three member nodes, one deliberately sending unsigned messages. Assert the
   two honest nodes converge on the same tier for the offender, that the
   offender's own `PRIVATE_GET_RATING` reports the drop, and that a fourth node
   with no first-hand contact stays above 600.
2. **Stateless restart** — the scenario the CRDT exists for. Run a relay
   alongside the members, let it accumulate observations, kill and restart it
   with its memory gone, and assert that after DAG replay it reports the same
   scores it held before the restart and that its own prior observations are
   still visible to the members.

### Stage 2 — application dimension (member nodes)

| File | Change |
|---|---|
| `cmd/node/moderator/moderator/moderator.go` | on a FAIL verdict, the chair reports the offending node named by the report it judged; observers never re-report a verdict they only received |
| `core/warpnet/warpnet.go` call sites of `VerifyAuthorship` | `Record(peer, KindForeignAuthorship)` on `ErrForeignAuthor` |
| `core/middleware/rate-limiter.go` | classify write routes; `KindWriteFlood` above a sustained-hit threshold |
| `core/handler/report.go` + moderator round result | `KindFalseReportBurst` above a per-window threshold, so an honest mistaken report costs nothing |
| `core/handler/rating.go` | register `PUBLIC_GET_RATING` on the member node |
| `frontend/src/components/InfoOverlay.vue` | peer rating badge |

**Tests:** a FAIL verdict moves only the named node's `Application` score and no
one else's; `overall == min(dimensions)`; a single cleared report costs zero; the
`KindWriteFlood` threshold does not trigger on ordinary posting rates.

### Stage 3 — moderation dimension (moderator nodes)

| File | Change |
|---|---|
| `cmd/node/moderator/audit/ledger.go` | `Ledger` gains a `rating.Reporter` and files an observation when a peer's standing **worsens** — once per crossing, so a long audit does not grind a peer down for a conclusion it already drew |
| `core/handler/moderation.go` | `KindVerdictMalformed` — observed by *member* nodes about *moderators*, the cross-role case the CRDT exists for |
| `cmd/node/moderator/round/round.go` | new optional `BallotObserver` capability, handed every ballot of a decided round |
| `cmd/node/moderator/moderator/audit.go` | implements `BallotObserver`: charges `KindVerdictOutlier` to moderators that voted against the outcome |
| `cmd/node/moderator/moderator/moderator.go` | ledger wired to the node's rating store |
| `docs/MODERATION.md` | user-facing sections on moderator standing and on the node's own rating |
| `cmd/node/moderator/audit/doc.go` | gap (1) marked partly addressed, with what is still missing |

**Three deviations from the plan, each forced by the code.**

*`Standing` stays.* The plan called for deleting it and feeding raw
outcomes to the rating. That does not work: audit quality is a **rate** —
agreement over many probes — while the rating counts discrete offences,
and a count cannot tell six wrong answers out of sixty from six out of
six. Feeding raw outcomes would punish an honest moderator on a
different model exactly as hard as a bot with no model at all. The
statistical tolerance therefore stays in the ledger, and only threshold
crossings reach the rating.

*Ballots are not weighted by rating.* `planTally` is a pure function
precisely so every participant reaches the same answer from the same
ballots, which is what lets the round pick a chair and a takeover order
without exchanging a message. Each node holds its own view of every
moderator, so weighted tallies would differ between participants and the
round would split. Dissent is observed and replicated instead — cheap
and capped, because model diversity produces it honestly.

*Two kinds dropped*, per §4.3.

**Tests:** the audit ledger's existing standing tests are kept intact — they
encode the tolerance that separates an honest exotic model from a bot — and
`ledger_rating_test.go` adds the reporting behaviour: an honest peer produces
no observations at all, a coin-flipper crosses straight to the ban line, a
mildly disagreeing peer is reported suspect and never banned, a conclusion is
reported once however long the audit runs, and unreachability is reported
every time but never bans.

### Stage 4 — retune the constants

Separate, deliberately small change: adjust the §4/§6 weights once testnet
data says what they should be. Nothing else in the diff.

---

## 11. What this does not protect against

Stated plainly, in the spirit of `cmd/node/moderator/audit/doc.go`.

1. **Identity is free.** A node at `TierFloor` restarts with a new key at
   `MaxScore`. Rating raises the cost of sustained abuse from one identity; it
   does not price identity. Only a stake, proof of work or a vouching web would,
   and none is in scope.
2. **Remote observations are advisory, therefore partly ignorable.** The caps in
   §6.3 that defeat slander also mean a real offender is fully sanctioned only by
   the peers it actually attacked. That is the intended trade.
3. **The observer weighting is circular.** `w(observer)` is computed from that
   observer's first-hand-only score to keep the recursion one level deep, but a
   large honest-looking clique can still shift the public aggregate. The
   aggregate is display-only for exactly this reason.
4. **Model diversity still muddies the moderation dimension.**
   `KindVerdictOutlier` carries a low weight and a ceiling because gap (2) of
   `audit/doc.go` — establishing model identity rather than assuming it — is
   unsolved. Until it is, a moderator's score must weight ballots, never
   disqualify them.
5. **Rating is not evidence.** A record proves an observer *claimed* something,
   not that it happened. Nothing here reaches the standard needed to ban a node,
   which is why §6.5 forbids automatic blocklisting.
6. **Restart recovery is only as good as the peers still holding the data.** The
   CRDT is what lets a stateless relay or moderator get its view back after a
   restart (§5.1), but the DAG can only replay what someone else still has. A
   node that restarts into an empty or partitioned network recovers nothing, and
   a record whose every holder has GC'd it past retention is gone for good. Fast
   restart loops also cost key growth: each start mints a generation, and those
   sub-counters live until the bucket expires.
7. **Blame for unsigned garbage is not attributable over CRDT.** A record whose
   signature does not verify names an observer that may have had nothing to do
   with it, so it can only be dropped, never charged (§5.4). A flood of such
   records is a bandwidth attack the rating system cannot price.

---

## 12. Open questions for review

1. **Retention vs. usefulness of `Recent`.** Author-side GC drops network records
   after ~4 days. Enough for a user to diagnose their own node, or should the
   *display* keep a longer, coarser summary (daily buckets, no per-kind detail)
   beyond the enforcement window?
2. **Should relays observe the application dimension?** They see enough traffic
   to notice write floods, but scoring content-adjacent behaviour from a node
   with no user context invites false positives. Plan says no.
3. **`minAcquaintance` on a mobile-heavy network.** One hour of connection before
   an observer counts may be too long for peers that are online in short bursts.
   Worth measuring on a testnet before fixing the constant.
4. **Retuning the constants.** Whether a weight change is an ordinary release
   or a network-epoch decision should be settled before Stage 1 merges.
