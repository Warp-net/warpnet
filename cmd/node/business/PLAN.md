# Business Node + Encrypted Subscription Topics

## Context

A "business node" is a new node type with extended rights and obligations:

**Rights**:
- Posts reach nearby (by RTT) users who never explicitly followed the business

**Obligations**:
- Must have a publicly addressable IP (no NAT)
- Must act as a libp2p relay
- Must act as a moderator (run the moderator engine + answer reports)

Currently `cmd/node/business/main.go` is a one-line stub. The codebase has three working node types (member, bootstrap, moderator) which provide most of the building blocks the business node needs to compose.

### Key constraint (user-specified, hard)

1. The recipient user MUST NOT be able to distinguish a business post from a regular tweet.
2. **Member nodes MUST NOT contain any ad-aware code.** No ad-specific handler, no ad-specific config, no ad-specific 
   service, no ad event type. Everything that happens on the member side must run through pre-existing, generic protocol code.
3. The business node itself reaches into nearby member nodes and inserts itself into their existing "subscriptions" 
   (gossipsub follow topics) using protocols the member already speaks.

### The mechanism: cross-topic publish (concept name in discussion: "cross-topic-injection")

The codebase identifier will be neutral — `cross-topic-injection` (or `topic-piggyback`) for service/package names. 

Member nodes are already subscribed to per-user gossipsub topics for everyone they organically follow 
(`userUpdate-<X>` style, replaced with a pubkey-derived hex blob in Phase 0). When user X publishes a tweet, X's node 
publishes the message onto the corresponding topic; every subscriber's gossipsub mesh routes the message to them; 
subscriber's `SelfPublish` feeds it into the local handler stack; `StreamNewTweetHandler` writes to `tweetRepo` and 
calls `timelineRepo.AddTweetToTimeline(owner.UserId, tweet)`. The handler inserts whichever tweet arrives — 
it does **not** validate that the tweet's `UserId` equals the topic owner's `UserId`. That permissive default is the key.

So the business node, after picking a target audience, simply **publishes its own tweet onto the topic of a publisher that 
the audience is already subscribed to**. 
The business never needs to be followed; it never creates a `Following` CRDT entry on the member; 
the member's UI shows no new account. The tweet appears in the member's timeline authored by the business, 
alongside the regular posts from the publisher whose topic carried it.

**Concrete flow**:
1. Business subscribes to popular `userUpdate-<X>` topics itself (purely as an observer)
2. Observes gossipsub mesh peers per topic — that gives it a map `member_peer_id → {topics member is subscribed to}`. 
Standard gossipsub gossiping already exposes this; no protocol change.
3. Filters candidate audience: members within RTT threshold, top N
4. Picks topics that intersect the candidates
5. Publishes its tweet onto one of those topics. Gossipsub fans it out to **every** subscriber of that topic 
(the candidates plus everyone else following the same publisher). That fan-out is fine — collateral reach is bonus distribution, not a bug.
6. Member receives, member's existing `StreamNewTweetHandler` adds to timeline. Done.

**Member-side code delta: zero.** No follow record, no new handler, no UI change, no surprise entry in Following list.

### Pre-existing permissive behavior this depends on

The receive path does not assert `tweet.UserId == topic.publisher_id`. That's existing behavior — we don't change it, 
and we don't add a defensive check (such a check would be ad-aware and would break the feature,
which the user explicitly forbids). This characteristic of the protocol is what makes the design feasible.

## Recommended approach

### Phase 1 — Identity model

In `core/warpnet/warpnet.go`:
- New `NodeInfo.Role` field, string, values `""` (regular) or `"business"`. Carried in the wire NodeInfo, 
so any peer that does `requestNodeUser` learns the role. The business node needs this so it can filter to *member* peers 
during target selection (it should not advertise itself to other business / moderator / bootstrap nodes).
- `NodeInfo.IsBusiness()` helper mirroring `IsBootstrap()` / `IsModerator()`.

`core/discovery/discovery.go` short-circuits stay as-is: business nodes go through `handleAsMember` 
(they have a real `UserId`, host real content, must be cached in `userRepo`).

### Phase 2 — Skeleton business node binary

New files: `cmd/node/business/main.go`, `cmd/node/business/node/business-node.go`, `cmd/node/business/app.go`.

Pattern: copy `cmd/node/member/main.go` + `cmd/node/member/node/member-node.go`, strip Wails, retain:
- Persistent local store at `config.Database.Path` (analytics history)
- Auth (business owner credentials, same as member), see Phase 6
- DHT (persistent), DiscoveryService, MDNS, PubSub (`MemberPubSub` reused as-is — 
business publishes its tweets through `PublishUpdateToFollowers` like any user)
- The full member-style handler set (tweets, replies, follow, like, view) — business profile/posts must be queryable like any other user
- `NodeInfo.Role = "business"`
- Startup reachability assertion: panic if libp2p reports `ReachabilityPrivate` after AutoNAT v2 stabilises, flapping protection
- Moderator engine + report subscription (Phase 5)
- The new "ad injection" service (Phase 3)

No Wails. `app.go` exposes an HTTP server (Phase 6).

### Phase 3 — Ad injection via cross-topic publish (the only genuinely new business logic)

New files under `cmd/node/business/crosstopic/`:
- `topic-observer.go` — subscribes to a configurable set of `userUpdate-<X>` topics (popular publishers, 
by some heuristic or curated list), records gossipsub mesh peers per topic. Result: an in-memory map `peer_id → 
{topics they're subscribed to}` plus a reverse map `topic → {peer_ids subscribed}`.
- `audience-picker.go` — from `userRepo`, lists known members (NodeInfo.Role == "") with `RoundTripTime > 0`, 
ranks by RTT ascending, picks top N (`config.Node.Business.AudienceSize`). Maps each candidate's `peer_id` to the set of
topics it appears in from the observer.
- `injection-service.go` — ticker (every `config.Node.Business.RefreshInterval`):
  1. Refresh observer mesh snapshots
  2. Compute audience
  3. For each candidate member, intersect its topic set with the observer's tracked topics → pick one or more topics to publish on
  4. For each chosen topic, publish a business tweet using the existing `MemberPubSub.PublishUpdateToFollowers` 
  machinery but with the **topic argument overridden** to the chosen `userUpdate-<X>` topic. 
  (Currently `PublishUpdateToFollowers` derives the topic from `ownerId`. We add a small variant `PublishToTopic(topicName, bt)` 
  on `MemberPubSub` that takes the topic name explicitly. This is a transport-layer addition, not a domain change.) 
  The published payload is the business's tweet, signed by the business's libp2p key; gossipsub routes it to all subscribers of the topic.
  5. Persist a small audit log (which tweet was injected on which topic at which time) for analytics

This service is the **only** new networked logic in the business feature. It runs **only** on the business binary.

**Why no member-side change**: members are already subscribed to the chosen topics for their organic follows. 
The business's tweet arrives through the existing gossipsub mesh, gets unwrapped by `SelfPublish`, 
dispatched to `StreamNewTweetHandler`, written to `tweetRepo`, inserted into the local timeline. 
The handler treats it identically to any other tweet on that topic. The tweet shows the business as author 
(because that's its `UserId`); the member's UI renders it as a normal tweet from the business — exactly indistinguishable 
from organic tweets the member would see if they had followed the business.

**Side effect (acceptable)**: the gossipsub topic's other subscribers 
(i.e., everyone who follows the publisher whose topic was used as a carrier) 
also receive the ad. The business gets free additional distribution beyond its target audience. 
The user previously framed this as a feature, not a problem.

**About `PublishToTopic`**: this is a tiny addition to `cmd/node/member/pubsub/member-pubsub.go` — 
it doesn't require a new protocol, it doesn't change the message format, 
it just exposes the underlying `g.pubsub.Publish(msg, topicName)` to the business binary directly. 
Member binary doesn't need to expose it; it's used only inside the business node code path.

### Phase 4 — Domain entity — unchanged

No `IsPromoted` flag. Business tweets are identical `domain.Tweet` records. Analytics on the business side: 
select `tweetRepo.ListByUserId(businessUserId)` (existing).

### Phase 5 — Moderator role reuse

Reuse the existing engine and isolation protocol, do not fork:
- Wire `cmd/node/moderator/moderator/{moderator.go, engine.go}` and `cmd/node/moderator/isolation/isolation-protocol.go` into the business node setup
- Same `engineReadyChan` startup, same `engine.Moderate(content)` interface, same `pubsub.SubscribeReports(...)` wiring
- Same `//go:build llama` build constraint inherited
- Same `config.Node.Moderator.Path` config dependency

The moderator code is structured around a `ModeratorNode` interface — the business node implements it.

### Phase 6 — WS frontend (Wails replacement)

**Backend** (`cmd/node/business/app.go`):
- websocket server listening on `config.Node.WSPort`
- Static file http handler: serve `frontend/dist` via the existing `GetStaticEmbedded` (`embedded.go:53`)
- WS endpoint accepts the Wails JSON envelope `{path, body, message_id, node_id, timestamp}`; 
handler calls `businessNode.SelfStream(req.path, req.body)` — same middleware, same handler stack Wails uses
- HTTP API endpoint `GET /api/is-first-run` mirrors `wailsjs/go/main/App.IsFirstRun`
- WS AES-256 encryption with preshared secret - should be the same as user password for easy use
- Session cookie auth backed by the existing `PRIVATE_POST_LOGIN` handler

**Frontend** (`frontend/src/lib/transport.js` — new, small file):
- `export async function Call(request)`: if `window.go?.main?.App?.Call` exists → use Wails, otherwise `fetch('/api/call', ...)`. Identical signature.
- `export async function IsFirstRun()`: same pattern.
- Update `frontend/src/service/service.js:28` (and 1-2 other Wails importers) to import from `@/lib/transport`.

No changes to `TweetBlock.vue` — ads are indistinguishable.

### Phase 7 — Analytics surface (business UI only)
Skip

### Phase 8 — Tests

- `cmd/node/business/crosstopic/*_test.go`: mock the user repo, gossipsub mesh snapshot, and a fake publisher; assert top-N audience selection, topic-set intersection logic, publish-on-each-chosen-topic behavior, no-op when no overlap
- `test/api_sync_test.go`: untouched (no new routes)
- Warpdroid DTO: untouched (no new fields)

## Critical files

**New**:
- `cmd/node/business/main.go`, `cmd/node/business/node/business-node.go`, `cmd/node/business/app.go`
- `cmd/node/business/crosstopic/{topic-observer.go, audience-picker.go, injection-service.go}` — the only new networked logic
- `frontend/src/lib/transport.js` — Wails-or-HTTP bridge
- `Dockerfile.business` and a compose entry under `deploy/`

**Modified**:
- `core/warpnet/warpnet.go` — `NodeInfo.Role`, `IsBusiness()`
- `cmd/node/member/pubsub/member-pubsub.go` — tiny `PublishToTopic(topicName, bt)` addition (Phase 3 uses it); Phase 0 changes to derive topic name from publisher pubkey
- `frontend/src/service/service.js` and 1-2 other Wails importers — switch to `@/lib/transport`
- `frontend/src/views/Root.vue` — login redirect for business dashboard
- `config/config.go` — `Node.Business.{Enabled, HttpPort, AudienceSize, RefreshInterval, ObservedTopics}`

**Pattern note**: `cmd/node/business/*` mirrors `cmd/node/moderator/*` structure (binary + node wrapper + extras).

## What is explicitly NOT being added

- ❌ Any new pubsub topic *kind* — business parasitizes existing `userUpdate-<X>` topics
- ❌ Any new handler on member nodes — the existing `StreamNewTweetHandler` accepts whatever arrives on subscribed topics
- ❌ Any ad-specific config, code, or UI on member nodes
- ❌ Any silent-follow / hidden-following / Following-list filter — business is never followed
- ❌ `domain.Tweet.IsPromoted` field
- ❌ "Sponsored" badge in TweetBlock.vue
- ❌ Any backend route delta visible to warpdroid
- ❌ Any defensive `tweet.UserId == topic.publisher_id` check (adding it would break the feature; not adding it preserves existing behavior)

## Verification

1. **Compile**: `go build -mod=vendor ./cmd/node/business/...`
2. **Unit tests**: `go test -mod=vendor ./cmd/node/business/injection/...`
3. **End-to-end smoke**:
   - Start business node on public IP: `go run ./cmd/node/business --node.business.enabled --node.business.httpport=7070 --node.business.audiencesize=5 --node.moderator.path=/models/llama.gguf`
   - HTTP and WS frontend: `curl http://<public-ip>:7070/`
   - Log into business dashboard, post a tweet
   - Spin up three **unmodified** member nodes: M1, M2 follow some publisher P; M3 follows nobody overlapping
   - Configure business `ObservedTopics` to include P's topic
   - Wait one refresh tick (verify in business's audit log that a publish happened on P's topic)
   - On M1, M2: hit `PUBLIC_GET_TIMELINE`, verify the business's tweet is present, identical shape to organic tweets (no `is_promoted` flag — it doesn't exist)
   - On M3: verify the business's tweet is NOT present (no shared topic = no exposure)
   - On all members' UIs: confirm the business does NOT appear in any "Following" list (no follow record was ever created)
   - Send a report from a member node; verify business runs the moderator engine and publishes the verdict on `IsolationTopic`
4. **Public-only assertion**: start business node behind NAT, confirm panic on `ReachabilityPrivate` 
