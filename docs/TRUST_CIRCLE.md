# Trust circle

Status: the circle, its route and the Vue and warpdroid blur are in the tree
(§10). Still open: the tier factor (§7.2), warpfone, and the mainnet
measurement that fixes θ.

A trust circle is the set of users this node's owner trusts, derived from the
follow graph as seen from this node. Every node computes its own; no list is
shared. It complements the node rating (`docs/RATING.md`): the rating is a
penalty on proven misbehaviour and starts every node at full trust; the circle
is vouching that starts every stranger at zero. The rating may feed the circle,
never the other way round — see §7.

This is step 1 of the "neighbour watch" plan (2026-11-11): the circle plus
blurred media from strangers. Witness reports, the deny store and neighbour
penalties build on it later and are out of scope here.

---

## 1. Why

| Problem | Today | With a circle |
|---|---|---|
| Identity is free (`RATING.md` §11.1) | A new key starts trusted everywhere | A new key is a stranger to everyone until a real person follows it back |
| Shock media from strangers | Shown in full in every surface | Blurred until one tap; bytes not even fetched |
| Weighing human reports later | No basis for "whose report counts" | Each report weighs by its author's place in my circle |

The idea is the Nostr "web of trust" (follows-of-follows), with two changes:
it is computed by the viewer's own node, not by a relay or a score provider,
and it only ever changes how content is *shown*, never whether it is carried.

## 2. Decisions this sketch assumes

1. **Content only.** The circle decides blur in clients now and report weight
   later. It never refuses a route, a stream, a gossip message or a peer, and
   never lowers a rating score (§7).
2. **Blur, never hide.** Text is untouched. Media is blurred with a "Show"
   control. The owner can switch blurring off.
3. **Computed on the node, not in clients.** Three clients (Vue, warpdroid,
   warpfone) would otherwise repeat the graph walk, and paired devices already
   proxy to the fat node. Later stages (witness weighting) run with no UI at
   all. Rejected alternative: per-client computation — triple code, three
   different answers to "is this author a stranger".
4. **In memory only.** The circle is a cache of other nodes' public lists and
   is rebuilt after a restart. No new Badger prefix, no CRDT.

## 3. The model

### 3.1 Edges and their authenticity

| Edge | Source | Trust in the claim |
|---|---|---|
| I follow X | own `FollowRepo` | full — my own action |
| X follows me | own `FollowRepo`; written by `StreamFollowHandler` only after `VerifyAuthorship` on X's node | full — first-hand, authenticated |
| m ↔ Y (for my mutual m) | m's node: `PUBLIC_GET_FOLLOWINGS` with `is_mutual_only` | as much as m — m vouches for its own list |

The second hop is m's word. That is the premise: I chose to follow m back, so
m's list counts, divided by its size. Nothing is ever asked of a stranger's
node, so a stranger cannot talk its way in by claiming it follows my friends.

### 3.2 Membership and score

```
M         = my mutuals                     (I follow X  and  X follows me)
D         = M ∪ {users I follow}           (direct)
mut(m)    = m's mutuals, as m's node lists them, for m ∈ M

score(A)  = 1                                        if A = me or A ∈ D
          = min(1, Σ_{m ∈ M, A ∈ mut(m)} 1/|mut(m)|)  otherwise

A is trusted  ⇔  score(A) ≥ θ
```

- **Trust splits by degree.** A mutual with 8 mutuals gives each of them 1/8;
  one who follows back everybody (3000) gives 1/3000. Follow-back-all accounts
  stop being a free entrance.
- **Only in-circle edges count.** Followers outside M contribute nothing, so
  bought or Sybil followers do not raise anyone's score.
- **Vouchers are kept.** For each A the circle keeps which m let it in
  (`vouchers`). Step 2 needs this to count witnesses entering through the same
  edge once.
- `θ` is a placeholder (0.02: one vouching mutual with ≤ 50 mutuals, or several
  bigger ones). It is set from the mainnet measurement (§9.1), not guessed.

### 3.3 Bridged and special users

- Bridged accounts (`bridge.IsBridgedUserID`) are never expanded: their lists
  come from the gateway, not from the person. They are trusted only when
  followed directly.

## 4. Computation

```go
// core/trust/circle.go
type listing struct {
    mutuals map[string]struct{} // m's own mutuals, bridged ids left out
    degree  int                 // |mut(m)|
}

type Circle struct {
    mu       sync.RWMutex
    listings map[string]listing // mutual -> its listing; replaced whole on refresh
}

func (r *Circle) Trust(userId string) domain.Trust
func (r *Circle) Refresh() error    // rebuilds the second hop
func (r *Circle) RefreshNew() error // lists mutuals not listed yet
func (r *Circle) Run()           // refresh loop until the node's context ends
```

- **Direct part is read live** from `FollowRepo` (`IsFollowing`, `IsFollower`)
  on every query, so a follow or unfollow takes effect at once, with no hook
  into the follow handlers. A voucher that stopped being a mutual stops
  counting at once too.
- **Second hop is rebuilt** by `Refresh` every 6 h with jitter, and once ~1 min
  after login. Between full refreshes `RefreshNew` runs every 5 min and lists
  only mutuals that have no listing yet, so a new mutual vouches within
  minutes; an unreachable one is retried on the next tick. For each m ∈ M: page through `PUBLIC_GET_FOLLOWINGS` with
  `is_mutual_only` on m's node, which answers with its own followings that
  follow it back. Requests are spaced so a
  refresh is a trickle, not a burst. An offline m keeps its previous lists until
  the next refresh; after a restart it contributes nothing until it is reached.
- **Caps.** Only the number of mutuals weighs a vouch: following many means
  reading much, not following back many, and costs nothing. m's node filters
  its own followings against its own followers locally, so the count is exact
  whatever the size of either list: one request up to 1 000 mutuals. Past
  2 000 mutuals reading stops after the third page and its members are not
  stored: at < 1/2000 each, they cannot reach θ through it alone. Bounds
  memory at |M| × 2 000 ids. A node that ignores `is_mutual_only` (it does not
  echo it) vouches for nobody until it updates.
- **Budget.** 200 mutuals × 200 mutuals each ≈ 40 k ids ≈ 5 MiB with map
  overhead. Must hold under the 512 MiB test-node limit with room to spare.
- `Refresh` returns its error to the loop that calls it, which logs once.

## 5. Wire surface

One private route, batched, because clients render a page of tweets at a time.

```go
// event/paths.go
PRIVATE_GET_TRUST = "/private/get/trust/0.0.0"

// event
type GetTrustEvent struct {
    UserIds []string `json:"user_ids"` // ≤ 100
}
type GetTrustResponse struct {
    Users map[string]domain.Trust `json:"users"`
}

// domain
type Trust struct {
    IsTrusted bool    `json:"is_trusted"`
    Score     float64 `json:"score"`
    Vouchers  int     `json:"vouchers"` // count only; ids stay on the node
}
```

- Handler `StreamGetTrustHandler` in `core/handler/trust.go`; the circle is a
  one-method interface declared there.
- `GetFollowingsEvent.is_mutual_only` (echoed in `FollowingsResponse`): a node
  asked for its own owner's followings returns only those that follow back,
  filling each page from as many raw pages as it takes. Other requests ignore
  it.
- Answers from memory only; never dials anyone on the request path.
- New route ⇒ regenerate warpfone protocol ids
  (`warpfone/scripts/generate-protocol-ids.sh`), or `test-warpfone` fails.
- Trust is viewer-relative, so it is **not** a field on `domain.Tweet` or
  `domain.User`: those are stored and replicated, the circle is not.

## 6. Clients

Common rule: media from an untrusted author is blurred and its bytes are not
fetched until "Show". Not fetching matters as much as blurring: a stranger's
image never lands in `SetForeignImageWithTTL` on this node unless the owner
asked for it.

| Client | Change |
|---|---|
| Vue (`frontend/src/components/TweetBlock.vue`) | `warpnetService.isTrusted` batches the authors asked for in one tick into one request and caches answers for 5 min; a failed request counts as trusted. Untrusted media is not fetched; a placeholder "Media from an unfamiliar author · Show" stands in. The opt-out toggle in `Settings/Preferences.vue` is kept in `localStorage`, not in the public profile metadata. Ships only after `frontend/dist` is rebuilt. |
| warpdroid (`warpnet/WarpnetRepository.kt`, `hydrateTweets`) | One `getTrust` batch per timeline page for the authors with media; an untrusted author's tweet gets `sensitive = true`. Tusky's sensitive-media blur and its "always show sensitive media" preference do the rest. Single tweets opened outside a page are not marked. Not yet checked on a device whether Tusky preloads the full image behind the blur. |
| warpfone (`Network/Warpnet/WarpnetMapper.swift`) | The port dropped Ice Cubes' sensitive-media handling; needs its own blurred placeholder. Lowest priority of the three. |

Gentle reveal (grayscale first, full colour on a second tap) is left for the
witness step, where it matters most.

### 6.1 Other consequences of being outside the circle (Vue)

Every one is a view choice on the owner's own client: nothing is refused,
deleted or reported, and one tap shows what was set aside. "Unfamiliar"
means outside the circle and not someone the owner chatted with:
`warpnetService.isUnfamiliar` is false for an accepted chat
(`chat_accepted::<user id>` in `localStorage`) and for an unanswered lookup,
so a failure never hides anything.

| Where | What changes |
|---|---|
| `views/Notifications.vue` | Follows, reactions, retweets, replies, mentions and messages from unfamiliar people go to an "Unfamiliar" tab with its unread count. Moderation and sales notifications always stay in the main list. |
| `components/SideNav.vue` | Unread items from unfamiliar people do not light the notification or message badge. Only the first page is checked, so older unread ones still count. |
| `views/Conversations.vue`, `views/Messages.vue` (`lib/chat-requests.mixin.js`) | Chats with unfamiliar people sit under "Message requests". Starting a chat or sending a message in it accepts the other user. |
| `views/Profile.vue` | A member of the circle gets a medal right of the `@handle` on their profile. Tweets carry no medal. |
| `views/Tweet.vue` | Under the owner's own thread, replies from unfamiliar people fold into "Show N replies from unfamiliar people". |
| `views/Search.vue`, `views/WhoToFollow.vue`, `components/WhoToFollow.vue` | `warpnetService.circleFirst` moves people in the circle ahead within each page, keeping the node's order within each part. |

Link previews need nothing: Warpnet fetches none, and the YouTube embed
already loads only on play.

## 7. Relation to the rating

The rating's `Application` dimension already scores the author's node by its
content — `moderation_upheld` is charged to the node whose tweet a verdict
condemned. So the line is not "hops versus authors". It is this:

### 7.1 Circle → rating: no

"Not in my circle" must not become an `Application` offence.

1. **Opposite defaults.** The rating subtracts from `MaxScore` on evidence and
   starts newcomers at full trust by requirement (`RATING.md` §3.3). The circle
   adds from zero on vouching. As a penalty it would be the probation period
   §3.3 rejected.
2. **Every dimension enforces on transport.** The overall score is the minimum
   across dimensions (`core/rating/engine.go`), and that minimum drives
   `ConnTag`, the gossip score, rate limits and DHT admission. A friendless
   newcomer would be throttled and dropped from routing for having no friends.
3. **Nothing to replicate.** Rating records are signed observations shared over
   CRDT under anti-slander caps. The circle is the owner's own follow graph;
   another node's opinion of my friends means nothing.
4. **Subjects are not 1:1.** The gateway node carries many bridged users;
   scoring it by author would punish all of them, and the gateway is never
   blocked.

### 7.2 Rating → circle: yes

- **Discount a vouching mutual.** A mutual whose node this node rates
  `TierDegraded` or `TierFloor` on `Application` vouches with reduced weight:
  `1/|mut(m)| × tier factor`. A read of the rating, no write to it.
- **Reuse the machinery for later steps.** Witness credibility (step 2) and the
  penalty on repeat introducers of offenders (step 5) need decaying, signed,
  per-subject records — exactly `core/rating/entries.go`. `KindFalseReportBurst`
  already sits in the `Application` catalogue for the first of them. Whether
  these penalties should also reach transport, through `Application`, or stay
  in a circle-only store with the same arithmetic is an open question (§9).

## 8. What this does not protect against

1. **A mutual who vouches for Sybils.** One bad mutual with a small list lets
   its clique in. Degree division limits it; neighbour penalties (later step)
   make it costly. Until then it is accepted.
2. **Small networks.** Few mutuals ⇒ small circles ⇒ most media blurred. The
   toggle exists for that.
3. **Lying lists.** m can list anyone as its mutual. The weight is m's own
   1/|mut(m)|, so lying buys nothing that honest vouching would not.
4. **Text.** Untouched by design.

## 9. Open questions

1. **θ and the degree function** (1/deg vs 1/log deg). Needs a mainnet measure:
   mutuals a new account gains in its first week, follow-back rate, overlap of
   circles between nodes.
2. **Blur by default, or opt-in at first?** Default-on is what makes the circle
   useful against shock media; opt-in is gentler for a small network.
3. **Private profiles.** If a user's lists become private, it drops out of
   others' second hop. Acceptable?
4. **Refresh cost on the remote side.** Each mutual answers two paged list
   calls per peer every 6 h. Fine at today's size; revisit if a popular node
   gets thousands of refreshing mutuals.
5. **Where introducer and witness penalties live** (§7.2): as `Application`
   kinds that also reach transport, or in a circle-only store reusing the
   rating's record and decay code. Note that of the `Application` catalogue
   only `foreign_authorship` and `moderation_upheld` are emitted today;
   `write_flood` and `false_report_burst` have no emitter.
6. **Tier factor** for a degraded or floored vouching mutual (§7.2).

## 10. Work breakdown

1. Done: `core/trust` with `Circle`, `Refresh` and scoring; tests for direct,
   second hop, degree split, mutuals cap, readers and popular mutuals, auto-followers, old nodes, bridged, unreachable mutuals and paging; mutual-only followings in the followings handler.
2. Done: wiring in the member node, one refresh loop started in `Start`.
3. Done: route, handler, events, domain type, warpfone ids, handler test.
4. Done in `src`: Vue deferred fetch, placeholder and toggle. `frontend/dist`
   is not rebuilt yet.
5. Done: warpdroid repository and DTOs. Not yet checked on a device.
6. Done in `src`: Vue notifications tab, badges, message requests, folded
   replies, circle-first ordering (§6.1). Not yet checked on a live stand.
7. Open: warpfone placeholder; §6.1 in warpdroid and warpfone; tier factor
   (§7.2).
8. Open: mainnet measurement (§9.1) before θ is fixed and before any later step
   (witnesses, bans) relies on the circle.
