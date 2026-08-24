# Node rating — design and implementation plan

Status: implemented. This document is both the design and the record of
what was built; where the implementation departed from the original plan,
the section says so and why.

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
| PN-counter over `go-ds-crdt`, single-writer generation-tagged keys, bitswap/DAG wiring | `core/crdt/stats.go` | The datastore wiring is extracted and reused; the rating store is a sibling, not a counter. |
| Gossip broadcaster adapter, topic `/warpnet/stats/1.0.0` | `core/crdt/gossip-adapter.go` | Parametrised by topic. |
| `UpsertTag` connection priority with a flap LRU | `core/node/priority.go` | Gains a second, independent `rating` tag. |
| Per-`route\|peer` leaky buckets | `core/middleware/rate-limiter.go` | Bucket parameters become a function of the peer's band. |
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
func ParseDimension(s string) (Dimension, bool)

// DimensionsFor maps warpnet.NodeInfo.Type to the axes that node tracks.
//   warpnet.RelayNode     -> {Network}
//   warpnet.MemberNode    -> {Network, Application}
//   warpnet.ModeratorNode -> {Network, Moderation}
//   unknown               -> {Network}
func DimensionsFor(nodeType string) []Dimension
```

A node writes only the dimensions its own role can witness, and reads only the
dimensions of the subject's role. A relay observing a member still only ever
writes `Network`.

### 3.2 Score and bands

```go
type Score int32

const (
    MaxScore Score = 1000 // a node never seen before: full trust
    MinScore Score = 0
)

type Band uint8

const (
    BandTrusted  Band = iota // 800..1000  no effect
    BandWatched              // 500..799   mild deprioritisation
    BandDegraded             // 200..499   halved rate limits, low priority
    BandFloor                // 0..199     minimum priority, gossipsub graylist range
)

func BandOf(s Score) Band
func (b Band) String() string
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
// core/rating/aggregate.go
const BucketDuration = time.Hour

var halfLife = map[Dimension]time.Duration{
    Network:     12 * time.Hour,
    Application: 7 * 24 * time.Hour,
    Moderation:  7 * 24 * time.Hour,
}

// retention is how far back records still contribute; older ones are
// ignored on read and GC'd by their author.
func retention(d Dimension) time.Duration { return 8 * halfLife[d] }

func decayFactor(age, half time.Duration) float64
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
    // never on its own push a peer out of BandWatched.
    ceiling int32
}

var catalogue = map[Kind]offence{ /* table below */ }

func (k Kind) Dimension() Dimension
func (k Kind) Weight() int32
func (k Kind) Ceiling() int32
func (k Kind) String() string   // stable wire/UI name, e.g. "bad_signature"
func (k Kind) Valid() bool
func KindByName(s string) (Kind, bool)
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
that reach `BandFloor` quickly, and only from first-hand evidence.

### 4.2 Application — member nodes

| Kind | Weight | Ceiling | Source |
|---|---|---|---|
| `KindModerationUpheld` | 300 | — | FAIL verdict from the moderator quorum naming this node's owner, already delivered to every observer at `core/handler/moderation.go` |
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
"total local-data loss" recovery path `core/crdt/stats.go` already documents for
counters. Without CRDT a relay would be permanently memoryless and its
observations would die with it.

A node has **one** CRDT replica (`crdt.Store`), and rating is a tenant of
it — not a second one. One blockstore, one bitswap exchange, one DAG and one gossip
topic replicate stat counters and peer ratings alike; a second datastore
would only buy a second copy of that machinery and a second set of blocks
to keep in sync.

| Node type | The node's one backing store | Survives restart via |
|---|---|---|
| member | `database.NewStatsRepo(db)`, Badger-backed — already there for stats | its own disk, plus the DAG for anything it missed while down |
| relay | the `datastore.NewMapDatastore()` it already constructs for the DHT | the DAG only |
| moderator | the `datastore.NewMapDatastore()` it already constructs for the DHT | the DAG only |

Tenants are separated by key prefix — `/STATS/...` and `/RATING/obs/...` —
and share nothing else. On the relay and the moderator the map store is
`MutexWrap`ped once at construction, because the DHT and the CRDT both
read and write it.

This is what decision 3 means in practice: state in memory, observations in the
network, and the network gives the state back.

Topics:

```go
// core/crdt/gossip-adapter.go
const crdtTopic = "/warpnet/stats/1.0.0"
```

One datastore, one broadcaster, one topic. The name of the string is
historical — it predates anything but stats living in the CRDT — and renaming
it would cut replication between versions for no gain.

**Why rating is not the stats store itself.** `CRDTStatsStore` is a PN-counter:
one `uint64` per key, merged by summing. A rating entry is a signed record —
observer, dimension, hour bucket, per-kind counts, signature — that has to be
verified before it is believed and re-signed whenever it changes. A counter
cannot carry a signature, and a node that could bump another node's counter
directly is exactly the forgery the signature exists to stop. So the two are
sibling tenants of the one datastore: same package (`core/crdt`), same
replication, same generation-nonce trick, different value type, different key
prefix.

**How two tenants share one set of hooks.** go-ds-crdt takes exactly one
`PutHook` and one `DeleteHook`, fixed at construction. `crdt.Store` installs a
dispatcher there and exposes `OnPut`/`OnDelete`, so each tenant subscribes and
every merged delta reaches all of them. The rating store filters by its own key
prefix; the stats store subscribes to nothing.

### 5.2 Key layout

```
/RATING/obs/{subjectID}/{observerID}/{dimension}/{bucketHour}/{generation}
```

`{generation}` is a fresh 128-bit nonce minted once per process start — the same
device `core/crdt/stats.go` uses, and for the same reason, only more acutely
here. A stateless relay restarts with an empty datastore and starts observing
immediately. Without a generation segment its first write of bucket B (count 1)
would land on the same key as the count-50 record the DAG is still replaying, and
LWW would silently destroy the replayed history — the exact failure the CRDT is
supposed to prevent. With it, the new process owns a key no past process can
collide with, the replayed records survive verbatim, and the reader **sums
across generations** within a bucket.

Consequences:

- No read-before-write anywhere. Each `(subject, observer, dim, bucket,
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
// core/rating/record.go
type Counts map[Kind]uint32

type Record struct {
    Subject    string    `json:"s"`
    Observer   string    `json:"o"`
    Dim        Dimension `json:"d"`
    Bucket     int64     `json:"b"`   // unix hour
    Generation string    `json:"g"`   // hex(16 random bytes), one per process start
    Counts     Counts    `json:"c"`   // this generation's running counts for this bucket
    UpdatedAt  time.Time `json:"u"`
    Signature  string    `json:"sig"` // observer's ed25519 over SigningBytes
}

// SigningBytes is canonical and stable across architectures:
//   subject "|" observer "|" itoa(dim) "|" itoa(bucket) "|" generation "|"
//   for each kind in ascending numeric order: itoa(kind) "=" itoa(count) ","
//   "|" itoa(updatedAt.UnixMilli())
func (r Record) SigningBytes() []byte

func (r *Record) Sign(priv ed25519.PrivateKey) error

// Verify derives the pubkey from the Observer peer id and checks the
// signature — the same trick StreamModerationResultHandler uses at
// core/handler/moderation.go:103-116.
func (r Record) Verify() error

// Validate enforces the structural rules, independent of signature:
//   - Subject and Observer parse as peer ids
//   - Subject != Observer            (a node cannot rate itself)
//   - Dim is a known dimension
//   - every Kind is valid and belongs to Dim
//   - Generation is 32 hex characters
//   - Bucket is not in the future beyond one bucket, not older than retention
func (r Record) Validate(now time.Time) error

func (r Record) Total() uint64
func (r Record) Key() string // the /RATING/obs/... path above
```

Properties this buys:

- **One writer per key, for the key's whole lifetime.** Only `Observer` writes
  `.../{observerID}/...`, and only one process ever owns a given
  `{generation}`. LWW inside the key is therefore trivially safe: no
  read-modify-write, no eventual-consistency window, and no way for a restarted
  process to clobber its own replayed history (§5.2).
- **`Subject == Observer` is invalid** and dropped on read. A node cannot rate
  itself by construction, not by convention.
- **Restart-safe by the same argument as the stats store.** A stateless node
  that comes back with an empty datastore mints a fresh generation, starts a new
  sub-counter, and the DAG layers its old generations back underneath. Reader
  sums them.

### 5.4 Authenticity

Every record is verified before it enters the index, on both the startup scan
and the CRDT put hook. The hook only *updates* subjects the index already
holds: creating one from a single delta would shadow the rest of its history in
the datastore, so an unindexed subject is instead loaded whole on its next
read. Two distinct failures, handled differently:

- **`Verify()` fails** — the signature does not match the pubkey derived from
  the claimed `Observer` peer id. Nobody is attributable: anyone can forge a
  claim naming any observer. Drop it silently and count it in a local metric
  only. Over CRDT there is no relaying peer to charge, so nothing is charged.
- **`Verify()` passes but `Validate()` fails** — the signature proves the named
  observer really authored a structurally illegal record (rated itself, used a
  kind from another dimension, back-dated a bucket). That *is* attributable, so
  the observer earns `KindForgedRecord`.

This is the one place the CRDT transport is weaker than a point-to-point one:
authenticity survives any relay path, but blame for unsigned garbage does not.

### 5.5 Size and GC

Records exist only where something actually happened — empty buckets are never
written — so the dataset is proportional to observed misbehaviour, not to N².
Per node: `subjects × observers × dimensions × buckets_retained × generations`.
A node misbehaving continuously for a week against 50 observers produces ~8k
records, single-digit MB. Idle peers cost zero bytes.

```go
// runs on the flush ticker, at most once per hour
func (s *Store) gcOwnExpired() error // deletes only /RATING/obs/*/{self}/... past retention
```

Only the author deletes its own records, so one node can never erase another's
evidence — and a CRDT delete is a tombstone that propagates, which is precisely
why a node must never prune foreign records to save memory. The in-memory index
is bounded instead (§6.1); the datastore is bounded only by retention.

---

## 6. Aggregation and enforcement

### 6.1 The in-memory index — why scoring is not a CRDT query

Scoring runs on the rate-limiter hot path, i.e. once per inbound request. A
prefix query per request is not acceptable. The store therefore keeps an
in-memory index and answers scores from arithmetic only:

```go
// core/rating/index.go
type index struct {
    mu   sync.RWMutex
    // subject -> dimension -> observer -> bucket -> generation -> Counts
    data map[string]map[Dimension]map[string]map[int64]map[string]Counts
    lru  *expirable.LRU[string, struct{}] // subject recency, for eviction
}
```

- Built at startup by one full prefix scan of `/RATING/obs/`.
- Kept current by `crdt.Options.PutHook` / `DeleteHook`, which fire on every
  merged delta. `core/crdt/stats.go:175-180` currently sets both to no-op
  closures with the logging commented out; the extracted helper (§10, Stage 0)
  takes them as parameters so the rating store can use them for real.
- Records failing `Validate`/`Verify` never enter the index.
- **Bounded by subject count, not by dataset size.** Above `maxIndexedSubjects`
  (16k) the least-recently-scored subject is evicted from the index. Eviction
  is index-only — it never deletes from the CRDT, because a CRDT delete is a
  tombstone that would propagate and destroy other nodes' evidence (§5.5). An
  evicted subject simply falls back to a one-off prefix query on its next
  scoring, and re-enters the index. This is what keeps a stateless relay's
  memory flat regardless of how large the replicated dataset grows.

### 6.2 Two numbers

**Local (subjective) score — the only one that enforces.**

```
penalty_local(subject, dim) =
      Σ decayed(own observations)                           // full weight, uncapped
  +   min( Σ_i min( decayed(obs_i) × w(observer_i), CapPerObserver ),
           CapRemoteTotal )

w(observer) = score_local(observer) / MaxScore
```

```go
const (
    CapPerObserver Score = 150
    CapRemoteTotal Score = 400
    // MinAcquaintance: a remote observer's records are ignored until we
    // have been connected to it for this long in this session. A
    // drive-by accuser has no voice.
    MinAcquaintance = time.Hour
)
```

`w(observer)` is computed from that observer's **first-hand-only** local score to
keep the recursion one level deep and terminating.

**Public (aggregate) score — display only.** Unweighted median across observers
per dimension. Shown to the node's own user and in peer detail views. It never
touches a rate limiter, a priority tag or a peer score.

### 6.3 The invariant that makes slander survivable

`CapRemoteTotal = 400` means **remote observations alone can never push a peer
below 600** — the bottom of `BandWatched`. Reaching `BandDegraded` or
`BandFloor` requires first-hand evidence gathered on our own wire.

Consequence: a coordinated slander campaign against an honest node costs it a
mild priority drop and nothing else, on every node that has not itself witnessed
a problem. A genuinely misbehaving node hits the floor on exactly the peers it is
misbehaving against, which is where enforcement matters. This invariant gets its
own test (§10, Stage 1).

### 6.4 Where the score is applied

```go
// core/rating/enforce.go — pure mappings, no dependencies, trivially testable
func ConnTagValue(b Band) int        // 60 / 30 / 10 / 1
func GossipAppScore(b Band) float64  // 0 / -10 / -60 / -200
func LimitMultiplier(b Band) float64 // 1.0 / 0.5 / 0.25 / 0.1
func AllowInDHT(b Band) bool         // false only for BandFloor
```

| Surface | Change | File |
|---|---|---|
| ConnManager | new `SetRatingPriority(pid, score)` writing a **separate** `rating` tag, kept distinct from the existing `reachability` tag so the two compose additively as libp2p intends rather than overwriting each other. Reuses the existing flap LRU. | `core/node/priority.go` |
| gossipsub | `pubsub.NewGossipSub` gains `pubsub.WithPeerScore(params, thresholds)` with `AppSpecificScore` reading the local score; `GraylistThreshold: -100`. Per §6.3 only first-hand evidence reaches the graylist range. | `core/pubsub/gossip.go:221` |
| DHT | new `dht.QueryFilter` / `dht.RoutingTableFilter` options rejecting `BandFloor` peers. | `core/dht/options.go`, `core/dht/dht.go` |
| Rate limits | `limitForRoute(route)` → `limitForRoute(route, band)`, multiplying `burst` and `perMinute`, floored at 1 so no peer is ever starved outright. The per-`route\|peer` LRU bucket records the band it was built for and is rebuilt when the band changes. | `core/middleware/rate-limiter.go` |
| Moderation ballots | **Nothing.** Weighting a vote round's ballots by rating was planned and rejected: `planTally` must be a pure function of the ballots so every participant reaches the same answer, and a locally-held rating is not. See Stage 3. | `cmd/node/moderator/round/` |
| Discovery | the new per-peer discovery bucket (§7d) is scaled by the same multiplier, so an offender's discovery entries are dropped first under pressure. | `core/discovery/rate-limiter.go` |

### 6.5 Deliberately not done

- **No automatic blocklisting.** `BlocklistExponential` stays a user/operator
  action. A slandered node must never be cut off by an automatic process.
- **No route refusal.** A `BandFloor` peer is served slowly and last, never told
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
| d | The discovery leaky bucket is **global** — `newRateLimiter(32, 2)`, ~12/min for the whole service. It cannot tell "12 new peers" from "one peer 12 times", and one chatty peer starves discovery for everyone. | `core/discovery/discovery.go:129,224` | Per-source buckets plus a per-peer dedup LRU in front; per-peer bucket scaled by band (§6.4). This is where `KindDiscoveryFlood` is raised. |
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
    Band       string            `json:"band"`
    Dimensions []DimensionRating `json:"dimensions"`
    Observers  int               `json:"observers"`
    UpdatedAt  time.Time         `json:"updated_at"`
}

type DimensionRating struct {
    Name   string         `json:"name"`
    Score  int32          `json:"score"`
    Band   string         `json:"band"`
    Recent []OffenceTally `json:"recent"`
}

type OffenceTally struct {
    Kind   string    `json:"kind"`
    Count  uint32    `json:"count"`
    LastAt time.Time `json:"last_at"`
}
```

- `PRIVATE_GET_RATING` → the owner's **public aggregate** for their own node,
  read from `/RATING/obs/{self}/*`, i.e. entirely from records written by others.
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

## 9. Store API and per-node wiring

```go
// core/rating/store.go
type Config struct {
    Ctx        context.Context
    Self       warpnet.WarpPeerID
    PrivKey    ed25519.PrivateKey
    Dimensions []Dimension
    Flush      time.Duration    // default 30s
    Now        func() time.Time // injectable for tests
    Acquainted Acquaintance     // how long we have known an observer
}

// Replica is the subset of the node's one CRDT replica this store needs:
// the datastore surface plus the merged-delta hooks. crdt.Store satisfies
// it; nothing in this package imports core/crdt.
type Replica interface {
    Get(context.Context, ds.Key) ([]byte, error)
    Put(context.Context, ds.Key, []byte) error
    Delete(context.Context, ds.Key) error
    Query(context.Context, ds.Query) (ds.Results, error)
    OnPut(func(ds.Key, []byte))
    OnDelete(func(ds.Key))
}

func NewStore(cfg Config, replica Replica) (*Store, error)

// write path — non-blocking, buffered, folded into hour buckets,
// flushed every cfg.Flush. The error is the caller's own fault, not the
// peer's: an empty subject, an unknown kind, or a dimension this node's
// role cannot witness.
func (s *Store) Record(subject warpnet.WarpPeerID, k Kind) error
func (s *Store) RecordN(subject warpnet.WarpPeerID, k Kind, n uint32) error

// read path — in-memory arithmetic, memoised per subject against an
// index revision (15s ceiling), except for the cold-path reload of a
// subject the index evicted, which is where the error comes from.
func (s *Store) Score(subject warpnet.WarpPeerID) (Score, error)
func (s *Store) Band(subject warpnet.WarpPeerID) (Band, error)
func (s *Store) Public(subject warpnet.WarpPeerID) (domain.NodeRating, error)
func (s *Store) Own() (domain.NodeRating, error)

func (s *Store) Close() error
```

Every read returns `MaxScore`/`BandTrusted` **alongside** its error. Each
caller is an enforcement point, and an enforcement point that cannot see
the evidence must not act as if it had — so a failed read costs a peer
nothing, and the boundary that cannot propagate the error (a gossipsub
score callback, a middleware, a libp2p notifier) logs it and carries on.

`Config` carries no generation field: `NewStore` mints one per call, exactly as
`NewCRDTStatsStore` does at `core/crdt/stats.go:197`.

What everything else holds is not the store but the **Handle** — one per
node process, created with the node, shared by middleware, discovery,
gossip and the moderator, and the only rating API an enforcement point
sees:

```go
// core/rating/handle.go
type Handle struct{ /* atomic slot */ }

func NewHandle() *Handle
func (h *Handle) Set(r Rater)                         // attach the store, safe while serving
func (h *Handle) Record(subject warpnet.WarpPeerID, k Kind)
func (h *Handle) Band(subject warpnet.WarpPeerID) Band
```

The Handle owns everything that would otherwise be reimplemented at
every call site: the no-store default (nobody is penalised), the
atomic swap of the store built after gossip — on a moderator, after the
node is already serving — the fail-open policy on a read failure, and
the logging of refused records. Consumers hold one field and call two
methods; none of them know a store exists.

```go
// core/rating/reporter.go — the store's surface, held only by the Handle
type Rater interface {
    Record(subject warpnet.WarpPeerID, k Kind) error
    Score(subject warpnet.WarpPeerID) (Score, error)
    Band(subject warpnet.WarpPeerID) (Band, error)
}
```

Wiring — identical shape on all three, differing only in the backing datastore
and the dimension set:

| Node | Dimensions | Backing datastore | Gossip source |
|---|---|---|---|
| **member** (`cmd/node/member/node/member-node.go`) | `Network`, `Application` | `database.NewRatingRepo(db)` | `m.pubsubService.Gossip()` — already exposed at `cmd/node/member/pubsub/member-pubsub.go:184` |
| **relay** (`cmd/node/relay/node/relay-node.go`) | `Network` | the `datastore.NewMapDatastore()` it already builds at `relay-node.go:104` | needs a new `Gossip()` accessor on `cmd/node/relay/pubsub` |
| **moderator** (`cmd/node/moderator/node/moderator-node.go`, built in `cmd/node/moderator/moderator/moderator.go`) | `Network`, `Moderation` | the `datastore.NewMapDatastore()` it already builds at `moderator-node.go:82` | `ModeratorNode` has no pubsub, but the moderator *process* does (`cmd/node/moderator/pubsub/publisher.go` wraps a `*pubsub.Gossip`); add a `Gossip()` accessor there |

Each builds its one replica — `crdt.NewGossipBroadcaster(ctx, gossip)` then
`crdt.NewStore(ctx, broadcaster, store, node, router)` — and hands it to
`rating.NewNodeStore(ctx, replica, node, privKey, nodeType)`, then
`handle.Set(store)`. Neither store owns the replica: whoever built it closes it.

All rating construction lives in `core/rating` (`NewNodeStore` fills the
dimensions from the node type and the acquaintance gate from live libp2p
connections); all replica construction lives in `core/crdt`. Neither package
imports the other: `rating.Replica` is the interface `crdt.Store` happens to
satisfy, and the assemblies are the only place the two meet.

Two interface widenings are needed, both satisfied by the existing concrete
type — `*distributedHashTable` already implements `FindProvidersAsync`
(`core/dht/dht.go:328`), the node-local interfaces just do not name it:

- `RelayNode.dHashTable` is typed `DistributedHashTableCloser` (`Close()` only)
  → add `FindProvidersAsync`.
- `ModeratorNode.dHashTable` is typed `DistributedHashTableDiscoverer`
  (`ClosestPeers`, `Close`) → add `FindProvidersAsync`.

Ordering constraint on all three: the rating store must be constructed after
gossip is running, the same ordering the member node already uses for the stats
store at `member-node.go:203-212`.

No config: rating has no modes and no switch. A node cannot opt out of
being rated by its neighbours, and a switch for whether it acts on what
it sees would only produce a blind free-rider — which contradicts rating
being an inherent property of a node. The consequences are soft by
design (§6.4), so there is nothing here that needs arming carefully.

What replaces a staged rollout: the
consequences themselves. Every knob in `enforce.go` is a weighting, not
a refusal — `LimitMultiplier` never reaches zero, nothing blocklists —
so a mis-set weight costs a peer latency, and the caps in §6.3 bound how
far a wrong number can carry. Testability is in `Config` instead:
`Now` injects the clock, `Flush` drives persistence by hand, and
`enforce.go` is a set of pure mappings with no dependencies.

---

## 10. Work breakdown

Five stages, each independently reviewable, mergeable and testable.

### Stage 0 — prep, no behaviour change

| File | Change |
|---|---|
| `core/crdt/store.go` (new) | `crdt.Store` — the node's one CRDT replica: the blockstore/bitswap/DAG/`crdt.New` block extracted verbatim from `NewCRDTStatsStore`, plus `OnPut`/`OnDelete` so more than one store can share it. |
| `core/crdt/stats.go` | take the replica instead of building one; delete the inlined block. |
| `core/crdt/gossip-adapter.go` | `statsTopic` becomes `crdtTopic`: one datastore, one broadcaster, one topic. |
| `database/stats-repo.go` | unchanged — `NewStatsRepo(db)` already backs the node's one CRDT datastore. |
| `cmd/node/relay/pubsub/relay-pubsub.go` | add `Gossip() *pubsub.Gossip`. |
| `cmd/node/moderator/pubsub/publisher.go` | add `Gossip() *pubsub.Gossip`. |
| `cmd/node/relay/node/relay-node.go` | widen `DistributedHashTableCloser` with `FindProvidersAsync`. |
| `cmd/node/moderator/node/moderator-node.go` | widen `DistributedHashTableDiscoverer` with `FindProvidersAsync`. |

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
| `core/rating/doc.go` | package rationale + the honest limitations of §11, in the style of `cmd/node/moderator/audit/doc.go` |
| `core/rating/rating.go` | `Dimension`, `Score`, `Band`, `BandOf`, `DimensionsFor` |
| `core/rating/offence.go` | `Kind`, catalogue, accessors |
| `core/rating/record.go` | `Record`, `SigningBytes`, `Sign`, `Verify`, `Validate`, `Key` |
| `core/rating/index.go` | in-memory index, per-subject revisions, startup scan, LRU eviction |
| `core/rating/aggregate.go` | decay, generation summing, subjective and public aggregation, caps |
| `core/rating/enforce.go` | pure band → knob mappings |
| `core/rating/reporter.go` | `Rater` — the store's surface, held only by the Handle |
| `core/rating/handle.go` | `Handle` — the one rating object everything else holds |
| `core/rating/store.go` | `Store`, `Config`, `Replica`, generation minting, buffered writer, flush, GC |
| `core/rating/node.go` | `NewNodeStore` — the store as a node builds it, acquaintance from live connections |
| `core/handler/rating.go` | `StreamGetOwnRatingHandler`, `StreamGetRatingHandler` |
| `domain/rating.go` | `NodeRating`, `DimensionRating`, `OffenceTally` |
| `frontend/src/views/Settings/Rating.vue` | own rating, per-dimension bars, recent offences |

Edited files:

| File | Change |
|---|---|
| `event/paths.go` | two new routes |
| `event/event.go` | `GetRatingEvent` |
| `core/middleware/middleware.go` | `NewWarpMiddleware` takes the `*rating.Handle`; `record` filters self-streams and charges through it |
| `core/middleware/auth.go` | `Record` at the five sites in §4.1 |
| `core/middleware/rate-limiter.go` | `Record(KindRateLimitHit)`; `limitForRoute(route, band)`; bucket carries its band and is rebuilt on change; register the two new routes under `limitRead` |
| `core/node/node.go` | `Record` on oversize/read error in `unwrap`; the node takes the `*rating.Handle` at construction |
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
| `aggregate_test.go` | decay is deterministic and monotonic; a record exactly one half-life old contributes half its weight; generations under one bucket are summed, not overwritten; kind ceilings hold; `CapPerObserver` and `CapRemoteTotal` hold |
| `aggregate_test.go` | **the §6.3 invariant**: any number of remote observers, any number of records, score never < 600 |
| `aggregate_test.go` | first-hand evidence alone reaches `BandFloor` |
| `store_test.go` | `Record` is non-blocking under a stalled datastore; buckets fold correctly; flush writes exactly one key per (subject, dim, bucket, generation) |
| `store_test.go` | **stateless restart recovery**: a store whose datastore is wiped, restarted against a datastore pre-seeded with its own prior-generation records (as the DAG would replay them), reports the same score as before the wipe, and its new writes do not overwrite the replayed ones |
| `index_test.go` | LRU eviction past `maxIndexedSubjects` never issues a CRDT delete; an evicted subject scores identically after falling back to a prefix query |
| `enforce_test.go` | band → tag/score/multiplier/DHT mappings; the floor multiplier still serves a peer |
| `rating_test.go` | `DimensionsFor` per node type; overall = min over dimensions |
| `core/handler/rating_test.go` | own rating excludes self-authored records; `PUBLIC_GET_RATING` response shape |
| `core/discovery/discovery_test.go` | (b) a second discovery event for a known peer issues no `PUBLIC_GET_INFO`; (d) one peer cannot exhaust the global budget; (f) a backoffed peer is not redialled |
| `core/handler/info_test.go` | (a) answering info does not enqueue an already-connected peer |
| `core/pubsub/gossip_test.go` | (c) a repeated epoch is dropped |

End-to-end on testnet, via the `warpnet-testnet-verify` skill:

1. Three member nodes, one deliberately sending unsigned messages. Assert the
   two honest nodes converge on the same band for the offender, that the
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
| `core/handler/moderation.go` | on a FAIL verdict, `Record(offenderNode, KindModerationUpheld)` — the verdict already arrives signed and quorum-backed at every observer, so no new wire message is needed |
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

1. **Identity is free.** A node at `BandFloor` restarts with a new key at
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
3. **`MinAcquaintance` on a mobile-heavy network.** One hour of connection before
   an observer counts may be too long for peers that are online in short bursts.
   Worth measuring on a testnet before fixing the constant.
4. **Retuning the constants.** Whether a weight change is an ordinary release
   or a network-epoch decision should be settled before Stage 1 merges.
