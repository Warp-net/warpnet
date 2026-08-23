# Node rating — design and implementation plan

Status: plan, nothing implemented yet.

A node's rating is an inherent property of every Warpnet node, computed and
stored **by its neighbours** — never by itself — replicated over CRDT, decaying
back to full trust over time, and fed back into libp2p, the DHT, gossipsub and
the per-peer rate limiters through callbacks.

This document is the implementation plan. It states what exists today, what
gets built, in which order, and — in the last section — what the scheme does
**not** protect against.

---

## 1. Decisions already taken

| # | Question | Decision |
|---|---|---|
| 1 | Trust model | **Hybrid.** Signed first-hand observations in CRDT. Enforcement uses each node's own *subjective* score (first-hand at full weight, remote observers weighted and capped). A separate unweighted *public aggregate* is display-only and never enforces. |
| 2 | What a low rating may do | **Priority (ConnManager + DHT filters) and gossipsub peer score, plus tightening of per-peer rate limits.** No refusal of routes. No automatic blocklisting. |
| 3 | Relays and moderators | **Observations go to the network, state stays in memory.** No persistent store is added to relay/moderator nodes; they publish signed observations and rebuild their view from the CRDT DAG after a restart. |
| 4 | Order of work | **Staged: network → application → moderation.** Stage 1 also carries the discovery-amplification fixes, without which every honest node looks like a flooder. |

---

## 2. What already exists

The plan is deliberately built on machinery that is already in the tree:

| Existing | File | Reused for |
|---|---|---|
| PN-counter over `go-ds-crdt`, generation-tagged single-writer keys, bitswap/DAG wiring | `core/crdt/stats.go` | The CRDT datastore wiring is extracted and reused; the rating store is a sibling, not a counter. |
| Gossip broadcaster adapter, topic `/warpnet/stats/1.0.0` | `core/crdt/gossip-adapter.go` | Parametrised by topic so rating gets `/warpnet/rating/1.0.0`. |
| `UpsertTag`-based connection priority with a flap LRU | `core/node/priority.go` | Gains a second, independent `rating` tag. |
| Per-`route|peer` leaky buckets | `core/middleware/rate-limiter.go` | Bucket parameters become a function of the peer's band. |
| Signature / freshness / private-route checks | `core/middleware/auth.go:54-85` | The main source of first-hand network observations. |
| Payload-size and frame errors | `core/node/node.go` (`unwrap`) | Malformed-frame and oversize observations. |
| Exponential blocklist | `database/node-repo.go` | Left alone. Rating does **not** drive it (decision 2). |
| Moderator spot-check ledger with `Outcome`/`Standing` | `cmd/node/moderator/audit/` | Becomes the moderation dimension's observation source; its `Standing` is superseded by the rating score. |
| Verdict signature verification | `core/handler/moderation.go:103-116` | Moderation-dimension observations about moderators. |
| `dht.AddPeerCallbacks` / `RemovePeerCallbacks` | `core/dht/options.go` | Joined by rating-aware `QueryFilter` / `RoutingTableFilter`. |
| `pubsub.WithPeerScore` with `AppSpecificScore` | `vendor/github.com/libp2p/go-libp2p-pubsub/gossipsub.go:367` | The libp2p callback the rating drives. Currently unused: `NewGossipSub` is called with no options. |

No rating, reputation or trust-score code exists in the tree today. `audit.Ledger`
is the closest thing and is explicitly local, in-memory, and wired to nothing —
`cmd/node/moderator/audit/doc.go` says so and explains why.

---

## 3. The model

### 3.1 Dimensions and roles

```go
// core/rating/rating.go
type Dimension uint8

const (
    Network     Dimension = iota // every node type
    Application                  // member nodes
    Moderation                   // moderator nodes
)

// DimensionsFor maps warpnet.NodeInfo.Type to the axes that node tracks.
//   relay     -> {Network}
//   member    -> {Network, Application}
//   moderator -> {Network, Moderation}
func DimensionsFor(nodeType string) []Dimension
```

A node tracks and publishes only the dimensions its role owns, and reads only the
dimensions of the peer's role. A relay observing a member still only ever writes
`Network` observations — it has no way to witness anything else.

### 3.2 Score

```go
type Score int32

const (
    MaxScore Score = 1000 // a node never seen before: full trust
    MinScore Score = 0
)
```

Bands, used everywhere a threshold is needed:

| Band | Range | Meaning |
|---|---|---|
| `BandTrusted` | 800–1000 | default state, no effect |
| `BandWatched` | 500–799 | mild deprioritisation |
| `BandDegraded` | 200–499 | halved rate limits, low priority |
| `BandFloor` | 0–199 | minimum priority, gossipsub graylist range |

A node's overall score is the **minimum** across the dimensions its role tracks:
a moderator that is clean on the wire but hands out forged verdicts is not a
"mostly fine" node.

### 3.3 New nodes start at maximum

There is no probation period, by requirement. A subject with no observations
scores `MaxScore`. This is a deliberate Sybil trade-off: identity is free, so a
probation period would only punish honest newcomers while a patient attacker
waits it out. The protection lives in the enforcement ceiling (§6) instead —
the worst a fresh malicious identity achieves is being deprioritised faster than
it can mint new ones.

### 3.4 Decay — self-healing

Every observation lands in an hour bucket. The score is a pure read-time
function of the observation set:

```
penalty(subject, dim) = Σ  weight(kind) × count × 2^( -age_hours / halfLife )
                       obs

score(subject, dim)   = clamp(MaxScore - penalty, MinScore, MaxScore)
```

| Dimension | Half-life | Rationale |
|---|---|---|
| `Network` | 12 h | transport misbehaviour is often a bad build or a bad link; recover fast |
| `Application` | 7 d | an upheld moderation verdict should outlive a news cycle |
| `Moderation` | 7 d | a moderator's standing must not be washable overnight |

Read-time decay means: no background sweeper, no rewrite traffic, no clock
skew between nodes changing anyone's *stored* data, and identical results on
every node given the same observation set. Buckets older than `8 × halfLife`
contribute nothing and are deleted from the CRDT **by their author only**, so
one node can never erase another's evidence.

---

## 4. Offence catalogue

Weights are penalty points at age zero. All are calibration targets — see §10,
shadow mode.

### 4.1 Network (all node types)

| Kind | Weight | First-hand only | Existing detection site |
|---|---|---|---|
| `BadSignature` | 250 | yes | `core/middleware/auth.go:69` |
| `MissingSignature` | 250 | yes | `core/middleware/auth.go:59` |
| `MalformedFrame` | 120 | yes | `core/middleware/auth.go:54`, `core/node/node.go:246` |
| `OversizePayload` | 150 | yes | `core/node/node.go:241` (`stream.ErrPayloadTooLarge`) |
| `StaleOrReplayed` | 120 | yes | `core/middleware/auth.go:76` |
| `PrivateRouteDenied` | 200 | yes | `core/middleware/auth.go:82` |
| `RateLimitHit` | 15 | yes | `core/middleware/rate-limiter.go:110` |
| `DiscoveryFlood` | 25 | yes | new, `core/discovery` per-peer bucket (§7) |
| `ConnectionFlap` | 10 | yes | `core/node/priority.go` flap LRU |
| `DialFailure` | 2 | yes | `core/discovery/discovery.go:268` — liveness only, capped so it can never reach `BandDegraded` on its own |
| `ForgedObservation` | 400 | yes | a rating record whose signature does not verify against the pubkey derived from its `Observer` id (§5.3) |

`BadSignature`, `MissingSignature` and `PrivateRouteDenied` are the only network
kinds that are self-evidently deliberate. They carry the weights that can reach
`BandFloor` quickly, and only ever from first-hand evidence.

### 4.2 Application (member nodes)

| Kind | Weight | Source |
|---|---|---|
| `ModerationUpheld` | 300 | a FAIL verdict from the moderator quorum naming this node's owner — already delivered to every observer at `core/handler/moderation.go` |
| `ForeignAuthorship` | 350 | `warpnet.VerifyAuthorship` → `ErrForeignAuthor`: publishing events on behalf of another user |
| `WriteFlood` | 20 | sustained rate-limit hits on write routes (tweet / reply / react / message) |
| `FalseReportBurst` | 60 | reports filed by this node that the quorum cleared, counted only above a per-window threshold so an honest mistaken report costs nothing |

`ModerationUpheld` is the join point between the existing moderation pipeline
and the rating: the verdict already arrives signed and quorum-backed at every
observer, so no new wire message is needed — the rating layer subscribes to the
same handler and records an observation about the offender's **node**.

### 4.3 Moderation (moderator nodes)

| Kind | Weight | Source |
|---|---|---|
| `VerdictBadSignature` | 500 | `core/handler/moderation.go:113` — `ev.Verify(pubKey)` fails |
| `VerdictNoModeratorID` | 500 | `core/handler/moderation.go:105-111` |
| `VerdictMalformed` | 200 | missing object/user id for the verdict type |
| `VerdictUnsolicited` | 250 | a verdict or ballot for a round this moderator was not selected for — `cmd/node/moderator/round/tally.go` already computes selection |
| `VerdictOutlier` | 60 | a ballot that disagreed with the round's final quorum result; low weight because honest model diversity produces these |
| `AuditWrong` | 60 | `audit.OutcomeWrong` |
| `AuditInvalid` | 500 | `audit.OutcomeInvalid` — bad signature, foreign responder id, mismatched challenge binding |
| `AuditUnreachable` | 5 | `audit.OutcomeUnreachable` — may be the network's fault; capped like `DialFailure` |

The existing `audit.Ledger` thresholds (`minSample`, `banAgreeBelow`, …) are
replaced by the score/band mechanism. `audit.Outcome` stays as the auditor's
classification vocabulary; `Standing` is removed in favour of `rating.Band`.

**This closes gap (1) in `audit/doc.go`** ("a single auditor must not judge"):
audit outcomes become signed observations replicated to every moderator, and a
moderator's score is the weighted aggregate over independent auditors, not one
node's opinion. Gaps (2)–(5) of that document remain open and are restated in
§11.

---

## 5. Storage

### 5.1 Where

A second CRDT store beside the stats one:

- New datastore prefix `RATING`, mirroring `database.NewStatsRepo` — add
  `database.NewRatingRepo(db)` in `database/stats-repo.go` (~6 lines, same
  `NodeRepo` with a different prefix).
- New gossip topic `/warpnet/rating/1.0.0`, so rating replication never
  competes with stat counters for the stats topic's buffer.
- `crdt.NewGossipBroadcaster` currently hardcodes `statsTopic`. Add
  `crdt.NewGossipBroadcasterOn(ctx, gossip, topic)` and make the existing
  constructor a one-line wrapper — keeps the diff minimal and every current
  caller untouched.
- The ~60 lines of blockstore/bitswap/DAG/crdt.New wiring in
  `NewCRDTStatsStore` are extracted into an unexported
  `newCRDTDatastore(ctx, broadcaster, store, node, router, prefix)` used by both
  stores. No behaviour change to stats.

Relays and moderators pass `datastore.NewMapDatastore()` (which they already
construct) instead of the Badger-backed repo. They replicate during the session
and rebuild from peers after a restart — the same recovery path
`core/crdt/stats.go` documents for total local-data loss.

### 5.2 Key layout

```
/RATING/obs/{subjectID}/{observerID}/{dimension}/{bucketHour}
```

Value:

```go
type ObservationRecord struct {
    Subject   string            `json:"s"`
    Observer  string            `json:"o"`
    Dim       Dimension         `json:"d"`
    Bucket    int64             `json:"b"`   // unix hour
    Counts    map[Kind]uint32   `json:"c"`   // absolute counts for this bucket
    UpdatedAt time.Time         `json:"u"`
    Signature string            `json:"sig"` // observer's ed25519 over canonical bytes
}
```

Properties this buys:

- **One writer per key.** Same invariant as the stats store: only `Observer`
  writes `.../{observerID}/...`, so LWW inside the key is safe and there is no
  read-modify-write hazard against the eventually-consistent local view.
- **`Subject == Observer` is invalid** and dropped on read. A node cannot rate
  itself, by construction, not by convention.
- **Bounded restart loss.** No `generation` segment (unlike stats): a process
  restarting mid-hour seeds its in-memory bucket by reading back that one key
  before the first write. Worst case it loses part of one hour of one dimension
  from one observer — irrelevant to a decaying score, whereas for stats a lost
  increment is permanent, which is why stats needs generations and rating does
  not.
- **Idempotent proxy writes.** A relay with no CRDT replica can gossip a signed
  record and let member nodes persist it under the same key. Two members writing
  it produce the same key with the same bytes. A proxy must read the key first
  and skip the write if the stored record already has an equal or higher total
  count, so a stale replay cannot roll a bucket backwards.

### 5.3 Authenticity

Every record is verified on read the same way a moderation verdict is
(`core/handler/moderation.go:103-116`): derive the pubkey from the `Observer`
peer id, verify the signature over the canonical bytes. A record that fails is
dropped, and if we know which peer handed it to us, that peer earns a
`ForgedObservation` mark. Authenticity therefore does not depend on who relayed
the record, which is what makes proxy persistence safe.

### 5.4 Size

Worst case per node: `peers × observers × dimensions × buckets_retained`. With
1-hour buckets, 8 half-lives of retention and empty buckets never written, a
node that misbehaves continuously for a week against 50 observers produces
~8k records — a few MB. Idle peers cost zero bytes. Author-side GC of expired
buckets keeps it flat.

---

## 6. Aggregation and enforcement

### 6.1 Two numbers

**Local (subjective) score — the only one that enforces.**

```
penalty_local(subject, dim) =
      Σ  decayed(obs)                                   // our own observations, full weight
   own
  +   min( Σ_remote_i  min( decayed(obs_i) × w(observer_i), CapPerObserver ),
           CapRemoteTotal )

w(observer) = score_local(observer) / MaxScore          // a distrusted accuser barely counts
CapPerObserver  = 150
CapRemoteTotal  = 400
```

**Public (aggregate) score — display only.** Unweighted median across observers.
Shown to the node's own user and, later, in peer detail views. It never touches
a rate limiter, a priority tag or a peer score.

### 6.2 The invariant that makes slander survivable

`CapRemoteTotal = 400` means **remote observations alone can never push a peer
below 600** — the bottom of `BandWatched`. Reaching `BandDegraded` or
`BandFloor` requires first-hand evidence gathered on our own wire.

Consequence: a coordinated slander campaign against an honest node costs that
node a mild priority drop and nothing else, on every node that has not itself
witnessed a problem. A genuinely misbehaving node hits the floor on exactly the
peers it is misbehaving against, which is where the enforcement matters.

Additional guards on remote observations:
- only counted from observers we have been connected to for ≥ 1 h in this
  session (a drive-by accuser has no voice);
- an observer's contribution about any single subject is capped as above,
  regardless of how many kinds it reports;
- `w(observer)` uses our *local* score for that observer, so an accuser we have
  first-hand reason to distrust is discounted before its accusations land.

### 6.3 Where the score is applied

| Surface | Change | File |
|---|---|---|
| ConnManager | new `SetRatingPriority(pid, score)` writing a **separate** `rating` tag — `Trusted 60 / Watched 30 / Degraded 10 / Floor 1`. Kept distinct from the existing `reachability` tag so the two compose additively as libp2p intends rather than overwriting each other. Reuses the existing flap LRU. | `core/node/priority.go` |
| gossipsub | `pubsub.NewGossipSub` gains `pubsub.WithPeerScore(params, thresholds)` with `AppSpecificScore: func(p peer.ID) float64` reading the local score — `Trusted 0 / Watched −10 / Degraded −60 / Floor −200`, `GraylistThreshold −100`. Per §6.2 only first-hand evidence can reach the graylist range. | `core/pubsub/gossip.go:221` |
| DHT | `dht.QueryFilter` and `dht.RoutingTableFilter` (both present in the vendored kad-dht, `dht_options.go:271,280`) reject `BandFloor` peers from queries and from the routing table. Exposed as new `dht.Option`s alongside `AddPeerCallbacks`. | `core/dht/options.go`, `core/dht/dht.go` |
| Rate limits | `limitForRoute(route)` becomes `limitForRoute(route, band)`, multiplying `burst` and `perMinute` by `Trusted 1.0 / Watched 0.5 / Degraded 0.25 / Floor 0.1`. The per-`route|peer` LRU bucket stores the band it was built for and is rebuilt when the band changes. | `core/middleware/rate-limiter.go` |
| Discovery | the per-peer discovery bucket (§7) is scaled by the same band, so an offender's discovery entries are the first dropped under pressure. | `core/discovery/` |

### 6.4 Deliberately not done

- **No automatic blocklisting.** `BlocklistExponential` stays a user/operator
  action. A slandered node must never be cut off from the network by an
  automatic process.
- **No route refusal.** A `BandFloor` peer is served slowly and last; it is
  never told "no".
- **No rating in `NodeInfo`.** A node self-reporting its rating is worthless.
  "Rating is an inherent property of a node" is realised by every node type
  owning a `rating.Store` in its core and every peer having a score in it — not
  by a field on the wire.

---

## 7. Discovery self-amplification — prerequisite for Stage 1

The suspicion is correct: the current discovery path makes every node generate
traffic against its peers that the network dimension would score as flooding.
These must be fixed in the same stage, or rating will penalise honest nodes.

| # | Problem | Site | Fix |
|---|---|---|---|
| a | Answering `PUBLIC_GET_INFO` enqueues the requester for discovery, which then requests *its* info back. `DiscoveryHandlerStream` short-circuits only when the peerstore already holds addrs, which is false on first contact — so every first contact costs an info ping-pong. | `core/handler/info.go:56`, `core/discovery/discovery.go:202` | Do not enqueue from the info handler for a peer we are already connected to; the connection is the discovery. |
| b | `handleAsMember` issues `requestNodeInfo` on **every** discovery event for a peer, including peers already connected and already known. | `core/discovery/discovery.go:290` | Per-peer "recently probed" LRU (30 min TTL) in front of `requestNodeInfo`; skip entirely when connected and the user row is fresh. |
| c | `publishPeerInfo` republishes up to 11 AddrInfos every 5 min, and every topic is `topic.Relay()`-ed, multiplying fan-out. Receivers treat every entry as a fresh discovery. With N nodes this is O(N²) info requests network-wide. | `core/pubsub/gossip.go:534`, `:274` | Publish own AddrInfo plus only recently *verified* peers; carry a monotonic epoch so receivers drop repeats; receivers skip entries already in the peerstore. |
| d | The discovery leaky bucket is **global** — `newRateLimiter(32, 2)`, ~12/min for the whole service. It cannot tell "12 new peers" from "one peer 12 times", and one chatty peer starves discovery for everyone. | `core/discovery/discovery.go:129,224` | Per-source buckets plus a per-peer dedup LRU in front; make the per-peer bucket rating-aware (§6.3). This is also where `DiscoveryFlood` observations are raised. |
| e | The DHT `PeerAdded` hook runs `d.dht.FindPeer(ctx, id)` — a full DHT walk per routing-table insert — purely to log the addresses. | `core/dht/dht.go:146` | Drop the `FindPeer`; log the id. Move callbacks off the routing-table hook onto a buffered channel so a slow callback cannot stall the table. |
| f | Discovery dials with `SimpleConnect` (raw `host.Connect`), bypassing `WarpNode.Connect`'s backoff check, so a dead peer republished by gossip is redialled forever. | `core/discovery/discovery.go:262`, `core/node/node.go:181` | Route discovery dials through the backoff-aware path. |

(a)–(f) are independently reviewable and independently testable; they can land
as their own commits ahead of the rating core.

---

## 8. Showing the rating to its own user

The node reads `/RATING/obs/{ownID}/*` — records written entirely by other
nodes — and presents the **public aggregate**, not its own subjective view
(which is empty for itself by construction).

- New route `PRIVATE_GET_RATING` → `{ overall, per_dimension[], recent_kinds[], observer_count, trend }`.
  `recent_kinds` is what makes the feature useful: "37 rate-limit hits and 4
  malformed frames in the last 6 hours" tells the user what to fix.
- New route `PUBLIC_GET_RATING` (Stage 2) → this node's signed view of a given
  subject. Needed for thin clients (warpdroid), for cold-start on relays and
  moderators whose state is in memory, and later for quorum work.
- Desktop UI: a new `frontend/src/views/Settings/Rating.vue` beside
  `Blocks.vue` / `Mutes.vue`, plus a compact badge in `InfoOverlay.vue`.
- warpdroid: out of scope for this plan; `PUBLIC_GET_RATING` is designed so it
  can be added without protocol changes.

---

## 9. Per-node-type wiring

`rating.Store` is constructed in each node's `Start()` with the dimensions its
role owns:

**Member** (`cmd/node/member/node/member-node.go`) — full store on the Badger
repo, both `Network` and `Application`, joined to the rating topic through the
existing `m.pubsubService.Gossip()`. Reporter injected into the middleware chain
and the moderation handler.

**Relay** (`cmd/node/relay/node/relay-node.go`) — `Network` only, backed by the
`MapDatastore` it already creates. Needs a `Gossip()` accessor on
`cmd/node/relay/pubsub` (member's `MemberPubSub` already has one at
`member-pubsub.go:184`). Publishes observations; its own view dies with the
process and is rebuilt from peers.

**Moderator** (`cmd/node/moderator/node/moderator-node.go`) — `Network` and
`Moderation`, backed by its `MapDatastore`. `ModeratorNode` itself has no
pubsub, but the moderator *process* does (`cmd/node/moderator/pubsub/publisher.go`
wraps a `*pubsub.Gossip`); add a `Gossip()` accessor there and build the store
on it. The auditor's outcomes are routed into the store instead of into
`audit.Ledger`.

---

## 10. Stages

Each stage is independently mergeable and independently testable.

### Stage 1 — core + network dimension + discovery fixes

**Shadow mode first.** New flag `--node.rating.mode` (`off` | `shadow` |
`enforce`, default `shadow` for one release). In `shadow` the store records,
replicates and displays everything, and the enforcement callbacks log the
decision they *would* have taken without applying it. Constants in §4/§6 are
calibration targets; shadow mode on testnet is how they get calibrated before
`enforce` becomes the default.

1. `core/rating/`: `rating.go` (Dimension, Score, Band, DimensionsFor),
   `offence.go` (Kind catalogue + weights), `record.go` (`ObservationRecord`,
   canonical bytes, sign/verify), `store.go` (CRDT store, buffered reporter,
   hour-bucket folding, 30 s flush, author-side GC), `aggregate.go` (decay,
   subjective and public aggregation, caps).
2. `core/crdt/`: extract `newCRDTDatastore`; add `NewGossipBroadcasterOn`.
   `database/`: add `NewRatingRepo`.
3. Observation sources: `core/middleware/auth.go`, `core/middleware/rate-limiter.go`,
   `core/node/node.go` (`unwrap`), `core/node/priority.go` (flap).
4. Enforcement callbacks: `core/node/priority.go` (`rating` tag),
   `core/pubsub/gossip.go` (`WithPeerScore`), `core/dht/` (`QueryFilter`,
   `RoutingTableFilter`), `core/middleware/rate-limiter.go` (band multiplier).
5. Discovery fixes (a)–(f) from §7.
6. `PRIVATE_GET_RATING` + `frontend/src/views/Settings/Rating.vue`.
7. Wiring for all three node types (§9).

Tests: unit tests for decay determinism, cap enforcement, the "remote-only score
never drops below 600" invariant, self-observation rejection, forged-record
rejection, idempotent proxy writes, band→limit mapping. Discovery fixes get
regression tests asserting the info ping-pong and the repeat-probe no longer
occur. End-to-end on testnet via the `warpnet-testnet-verify` skill: three
nodes, one deliberately sending unsigned messages, assert the other two
converge on the same band and that a fourth node with no first-hand contact
stays above 600.

### Stage 2 — application dimension (member nodes)

1. `ModerationUpheld` recorded from `core/handler/moderation.go` when a FAIL
   verdict names a known node.
2. `ForeignAuthorship` from `warpnet.VerifyAuthorship` failures.
3. `WriteFlood` from sustained write-route limit hits.
4. `FalseReportBurst` with a per-window threshold.
5. `PUBLIC_GET_RATING`.
6. Rating badge in the peer detail overlay.

Tests: a verdict against a peer moves only that peer's `Application` score;
`overall = min(dimensions)`; an honest single false report costs nothing.

### Stage 3 — moderation dimension (moderator nodes)

1. `audit.Outcome` → rating observations; delete `audit.Standing`, keep
   `Outcome`; `audit.Ledger` becomes a thin adapter over `rating.Store`.
2. `VerdictBadSignature` / `VerdictNoModeratorID` / `VerdictMalformed` recorded
   from `core/handler/moderation.go` — note these are observed by *member*
   nodes about *moderators*, which is exactly the cross-role case the CRDT is
   for.
3. `VerdictOutlier` and `VerdictUnsolicited` from `cmd/node/moderator/round/`.
4. Moderator score consulted when weighting ballots in `round/tally.go` —
   weighting only; never disqualification (gap (2) of `audit/doc.go` is still
   open, so a low score must not silence a moderator).
5. Update `docs/MODERATION.md` with the user-facing explanation.

Tests: `troika_integration_test.go` extended with a dishonest moderator; assert
its score converges across independent auditors and that its ballots lose weight
without being dropped.

---

## 11. What this does not protect against

Stated plainly, in the spirit of `cmd/node/moderator/audit/doc.go`:

1. **Identity is free.** A node at `BandFloor` restarts with a new key at
   `MaxScore`. Rating raises the cost of sustained abuse from one identity; it
   does not price identity. Only a stake, proof of work or a vouching web would,
   and none is in scope.
2. **Remote observations are advisory, therefore partly ignorable.** The caps in
   §6.2 that defeat slander also mean a real offender is only fully sanctioned
   by the peers it has actually attacked. That is the intended trade.
3. **The observer weighting is circular.** `w(observer)` uses our local score for
   that observer, which is itself partly built from others' observations. The
   caps bound the damage but do not remove the circularity; a large honest-looking
   clique can still shift the public aggregate. The aggregate is display-only
   for exactly this reason.
4. **Model diversity still muddies the moderation dimension.** `VerdictOutlier`
   carries a low weight because gap (2) of `audit/doc.go` — establishing model
   identity rather than assuming it — remains unsolved. Until it is, a
   moderator's score must weight ballots, never disqualify them.
5. **Rating is not evidence.** A record proves an observer *claimed* something,
   not that it happened. Nothing here reaches the standard needed to ban a node,
   which is precisely why §6.4 forbids automatic blocklisting.

---

## 12. Open questions for review

1. **Retention vs. usefulness of `recent_kinds`.** Author-side GC at
   `8 × halfLife` drops network records after ~4 days. Is that enough history
   for a user to diagnose their own node, or should the *display* keep a longer,
   coarser summary (daily buckets, no per-kind detail) beyond the enforcement
   window?
2. **Should relays observe the application dimension at all?** They see enough
   traffic to notice write floods, but scoring content-adjacent behaviour from a
   node with no user context invites false positives. Plan says no; worth a
   second opinion.
3. **`enforce` as default.** The plan ships `shadow` by default for one release.
   Whether the switch is a config default change or a network-epoch decision
   should be settled before Stage 1 merges.
