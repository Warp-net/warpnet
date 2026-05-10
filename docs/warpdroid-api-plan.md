# Plan: Closing the WarpnetApi.kt ↔ Backend ↔ Vue gap

Date: 2026-05-10
Branch: `claude/analyze-warpdroid-api-TpEE4`
Source of truth for the client surface:
`warpdroid/app/src/main/java/site/warpnet/warpdroid/network/WarpnetApi.kt`
(920 lines, ~110 suspend methods, generated as a Retrofit-shaped facade
over `WarpnetRepository` so the ~50 Tusky view-models keep compiling).

## 0. Method of work

`WarpnetApi.kt` is the Mastodon/Tusky API surface that the Android client
exposes to its UI layer. It is *not* a description of what Warpnet must
become — it is the union of:

  1. things every Mastodon client takes for granted (filters, lists,
     announcements, OAuth, push subscriptions, instance metadata, polls,
     translation, scheduled posts, federated timelines, domain blocks,
     trending, bookmarks, conversations, …) which today are stubbed
     `NetworkResult.failure(UnsupportedOperationException)` /
     `Response.error(501)` / empty success;
  2. things `WarpnetRepository` already wires into Warpnet protocol IDs
     (timeline, single status, replies, like/unlike, retweet/unretweet,
     view, follow/unfollow, followers/followings, user, list-users,
     home-timeline, notifications, post status, delete status,
     relationships).

The plan below treats group (2) as **done**, classifies group (1) into
three tiers, and for each *tractable* method specifies the exact route,
DTO, repo method, handler, registration entry, Vue service method,
Vue view edits, and warpdroid touch-points (mostly already there).

Every route in the plan obeys the contract from the
`warpnet-add-handler` skill: identical path string in `event/paths.go` +
`frontend/src/service/service.js` + `warpdroid/warpnet-transport/.../ProtocolIds.kt`,
DTOs in `event/event.go` (camelCase Go fields, snake_case JSON tags),
repo with `NewTxn` / `defer Rollback` / `Commit`, handler under
`core/handler/<feature>.go` with handler-local `Storer` interface, and
registration in `cmd/node/member/node/member-node.go` next to related
routes with `//nolint:govet`.

## 0.1 Terminology — `home` vs `timeline`

Warpnet and Mastodon use the same words for different things. Throughout
this document:

* **Warpnet "home"** = the user's own tweets feed (`PUBLIC_GET_TWEETS`
  called with `user_id == self`). In Mastodon vocabulary this is
  `accountStatuses(selfId)`.
* **Warpnet "timeline"** = the user's friends-plus-recommendations feed
  (`PRIVATE_GET_TIMELINE`). In Mastodon vocabulary this is what Tusky
  calls `homeTimeline`.

So Tusky's `homeTimeline` is *not* the Warpnet home — it is the Warpnet
timeline. And Tusky's `accountStatuses(self)` is the Warpnet home. The
existing wires already do the right mapping; the naming inside Tusky
stays Mastodon-shaped, but every comment / docstring on the Warpnet
side should reach for **timeline** when meaning friends+recs and
**home** when meaning own-tweets.

## 1. Implementation patterns observed

### 1.1 Backend (Go)

* Path = `/{visibility}/{verb}/{resource}/{version}`. `private` = local
  only, `public` = node-to-node. No PUT/PATCH.
* Each handler file declares its own narrow `Storer` interface — no
  shared interface ledger. Handlers *receive* repos through these
  interfaces, never construct them.
* PUBLIC writes that mutate a remote resource follow the
  `core/handler/like.go` propagation pattern: do the local write,
  resolve the owner's nodeId via `userRepo`, call
  `streamer.GenericStream(targetNodeId, event.PUBLIC_POST_X, ev)`,
  swallow `ErrNodeIsOffline`, log any unmarshalled `ResponseError`,
  return the *local* result.
* Repo writes always go through `NewTxn` + `defer Rollback` + `Commit`.
  Empty-input validation at the top with `local_store.DBError("empty …")`.
* Stats DB is optional — guard `if statsDb == nil { … }` and treat
  failures as warnings.
* Project-internal `json` package, never `encoding/json`. Logger is
  `logrus`. Error wrapping prefixes the handler name lower-case
  (`warpnet.WarpError("foo: empty tweet id")`).
* Tests are white-box `package handler`, plain `testing`, file starts
  with `//nolint:all`; stubs are local structs with `<method>Fn` fields.
* Pagination is cursor-based: events take a `cursor` string, responses
  return `{items, cursor}`; cursor `""` means "no more pages".

### 1.2 Vue desktop client

* No store layer. State lives in `warpnetService.stateMap` (a `Map`)
  and `localStorage`. Pagination cursors are cached by route+key inside
  `stateMap`. There is no Pinia/Vuex.
* Each Warpnet call is a method on `warpnetService` that:
  builds `{path, body}`, hands it to `sendToNode(request)`, awaits the
  response, returns the unwrapped payload.
* Bodies are snake_case and must match the Go struct's JSON tags
  byte-for-byte.
* Notification refresh is a hard-coded 2 s `setInterval` that calls the
  notification path and dispatches via `subscribeNotifications(cb)`.
* Component data binds directly to the awaited result; no reactive
  store wrapper. Adding a screen ≈ create `views/X.vue` + add a route
  in `router/index.js` + call `warpnetService` directly.

### 1.3 Warpdroid (Android) client

* Wire layer (`site.warpnet.transport.*`): `ProtocolIds`,
  `WarpnetEnvelope`, Moshi DTOs, `WarpnetClient.request()`. All
  string-in/string-out through the gomobile binding.
* Adapter (`com.keylesspalace.tusky.warpnet.*`): `WarpnetRepository`
  exposes Mastodon-shaped types to the Tusky UI; `WarpnetMapper`
  contains the lossy translation between Warpnet wire DTOs and the
  Mastodon entities (`Status`, `Account`, `Notification`,
  `Relationship`, `TimelineAccount`).
* Pagination is exposed as `(items, cursor)` and re-wrapped by
  `WarpnetApi.paginated{}` into a synthetic
  `Link: <…?max_id=CURSOR>; rel="next"` header so
  `NetworkTimelineRemoteMediator` keeps working.
* `EnvelopeSigner` is `NoOpSigner` today; routes that pass through
  middleware-level signature verification cannot run from warpdroid
  end-to-end yet (see `warpdroid/warpnet-transport/.../EnvelopeSigner.kt`).

## 2. Today's coverage

42 ProtocolIds exist in `event/paths.go` (16 PRIVATE + 26 PUBLIC), all
registered in `member-node.go`, all mirrored in
`warpdroid/warpnet-transport/.../ProtocolIds.kt`, ~38 of 42 mirrored in
`frontend/src/service/service.js`. `WarpnetRepository.kt` hits 21 of
them.

| Domain | Backend | Vue (`service.js`) | warpdroid (`WarpnetRepository`) |
|---|---|---|---|
| auth (login/logout/pair) | ✅ | ✅ login | pairing only (separate path) |
| **timeline** (friends + recs; = Tusky `homeTimeline`) | ✅ PRIVATE_GET_TIMELINE | ✅ getMyTimeline | ✅ getHomeTimeline |
| **home** / per-user feed (own tweets when self; = Tusky `accountStatuses`) | ✅ PUBLIC_GET_TWEETS | ✅ getTweets | ✅ getUserTimeline |
| single tweet | ✅ PUBLIC_GET_TWEET | ✅ getTweet | ✅ getStatus |
| tweet stats | ✅ PUBLIC_GET_TWEET_STATS | ✅ getTweetStats | ✅ getTweetStats |
| post tweet | ✅ PRIVATE_POST_TWEET | ✅ createTweet | ✅ postStatus |
| delete tweet | ✅ PRIVATE_DELETE_TWEET | ✅ deleteTweet | ✅ deleteStatus |
| like / unlike | ✅ PUBLIC_POST_(UN)LIKE | ✅ (un)likeTweet | ✅ (un)likeStatus |
| retweet / unretweet | ✅ PUBLIC_POST_(UN)RETWEET | ✅ (un)retweetTweet | ✅ (un)retweetStatus |
| view | ✅ PUBLIC_POST_VIEW | ✅ viewTweet | ✅ recordView |
| reply create / get / delete | ✅ | ✅ | ✅ getReplies, getAncestors |
| follow / unfollow | ✅ | ✅ | ✅ |
| isFollowing / isFollower | ✅ | ✅ | ✅ relationshipFor |
| followers / followings | ✅ | ✅ | ✅ |
| user lookup / list | ✅ | ✅ | ✅ getAccount, listUsers |
| who-to-follow | ✅ | ✅ | (not yet wired in API.kt) |
| notifications | ✅ PRIVATE_GET_NOTIFICATIONS | ✅ | ✅ |
| chat (create/get/list/delete) | ✅ | ✅ | wired but unused by API.kt |
| messages (post/get/list/delete) | ✅ | ✅ | wired but unused by API.kt |
| image upload/get | ✅ | ✅ | wired but unused by API.kt |
| update own profile | ✅ PRIVATE_POST_USER | ✅ editMyProfile | (no method on Repository yet) |
| node info / stats | ✅ | ✅ getNodeInfo | (admin-only — N/A) |
| moderation result | ✅ ingest only | — | — |
| node challenge | ✅ ingest only | — | — |

## 3. WarpnetApi.kt method classification

The 110 methods break down as follows. Counts include overloads with an
`-WithAuth` suffix (Tusky's wire-detail variant).

### 3.1 Already Warpnet-backed (24 methods) — no work

```
homeTimeline, publicTimeline (fallback), notifications,
notificationsWithAuth, createStatus, statusContext, deleteStatus,
retweetStatus, unretweetStatus, likeStatus, unlikeStatus, recordView,
accountVerifyCredentials, searchAccounts (client-side filter),
account, accountStatuses, accountFollowers, accountFollowing,
followAccount, unfollowAccount, relationships
```

### 3.2 Soft-stubs that can stay stubs forever (29 methods)

These have no Warpnet equivalent and Tusky tolerates an empty/Unit
success. **Action: none.** Keep returning success-with-empty.

```
getCustomEmojis, getInstanceV1, getInstance, getInstanceRules,
markersWithAuth, updateMarkersWithAuth, clearNotifications,
getFilters, deleteFilter, deleteFilterKeyword, statusEdits,
announcements, dismissAnnouncement, addAnnouncementReaction,
removeAnnouncementReaction, report, scheduledTweets,
deleteScheduledStatus, blockDomain, unblockDomain, domainBlocks,
followedTags, trendingTags, trendingStatuses, quotingStatuses,
followRequests, getNotificationRequests, acceptNotificationRequest,
dismissNotificationRequest, getLists, getListsIncludesAccount,
deleteList, getAccountsInList, deleteAccountFromList,
addAccountToList, getConversations, deleteConversation,
revokeOAuthToken, unsubscribePushNotifications, removeAnnouncementReaction
```

### 3.3 Hard stubs blocking real UX (57 methods) — Tier A / B / C below

| Tier | Meaning |
|---|---|
| **A** | Real Warpnet feature, gating real Tusky UX (compose, profile, moderation, search, single-status fetch). Must land. |
| **B** | Warpnet-shaped adaptation (no perfect Mastodon equivalent) — design needed before code. Land after Tier A. |
| **C** | Skip / no-fix — fundamentally Mastodon-instance-shaped (push, OAuth, instance rules, domain blocks). Keep stubbed. |

The remainder of this document specifies Tier A and B in full.

## 4. Tier A — required handlers (per endpoint)

For each: **client method → route → DTO → repo → handler → registration
→ Vue service → Vue views**. Patches assume the file paths above.

### A1. Single status fetch by id alone — `status(statusId)`

**Why**: Tusky's "open status from URL", "view edit", "view source",
"jump from notification", and offline-cache rehydration all call
`status(id)` *without* a userId. Today this stub-fails, so deep
linking and notification taps land on a blank screen.

**Current backend**: `PUBLIC_GET_TWEET` (`StreamGetTweetHandler`)
requires `{user_id, tweet_id}` — both fields are mandatory (handler
rejects empty user_id). The owner is needed because it determines the
node to forward to.

**Plan**:

1. Resolve owner from id. Tweet IDs in Warpnet (`domain.ID`) are ULIDs
   that *do not* embed the owner. Two options:
   * **A1a (preferred)**: extend `tweetRepo` with a global secondary
     index `tweet:owner -> userID` populated on `New`/`Delete`. Lookup
     becomes O(1) local. New repo method: `OwnerOfTweet(id) (string, error)`.
   * **A1b (fallback)**: gossip a "find tweet by id" query. Heavy.
1. New route: `PUBLIC_GET_TWEET_BY_ID = "/public/get/tweetbyid/0.0.0"`.
1. DTO in `event/event.go`:
   ```go
   // GetTweetByIdEvent defines model for GetTweetByIdEvent.
   type GetTweetByIdEvent struct {
       TweetId domain.ID `json:"tweet_id"`
   }
   ```
   Response reuses `domain.Tweet` (already the response type for
   `PUBLIC_GET_TWEET`).
1. Repo change: add `OwnerOfTweet(id string) (string, error)` to
   `database/tweet-repo.go`. Backfill existing tweets via a one-shot
   migration in `member-node.go`'s startup path.
1. Handler `core/handler/tweet.go`:
   `StreamGetTweetByIdHandler(repo TweetByIdStorer, streamer GenericStreamer) WarpHandlerFunc`
   that resolves owner locally and, if not local, forwards via
   `GenericStream(ownerNodeId, PUBLIC_GET_TWEET_BY_ID, ev)`.
1. Register in `member-node.go` next to `PUBLIC_GET_TWEET`.
1. Vue: `frontend/src/service/service.js` add `PUBLIC_GET_TWEET_BY_ID`
   constant + `getTweetById(tweetId)` method. Used by deep-link router
   guard.
1. warpdroid: `ProtocolIds.kt` constant + `WarpnetRepository.getStatusById(id)`
   that calls the new path.
1. `WarpnetApi.kt::status(statusId)` → `result { warpnet.getStatusById(statusId) }`.

### A2. Status edit — `editStatus`, `statusSource`, `statusEdits`

**Why**: Tusky compose has a full edit flow. Today landing in the edit
view triggers `statusSource` which fails, so the edit button is dead.

**Plan**:

1. Treat edits as immutable revisions. New repo:
   `database/tweet-edit-repo.go`. On `editTweet`, push a new
   `domain.TweetEdit{ID, OriginalTweetID, Text, EditedAt}` record and
   update the canonical tweet's `text` + `edited_at`.
1. Routes:
   * `PRIVATE_POST_TWEET_EDIT = "/private/post/tweet/edit/0.0.0"`
   * `PUBLIC_GET_TWEET_SOURCE = "/public/get/tweet/source/0.0.0"`
     (returns plaintext source — distinct from the rendered tweet
     because Warpnet stores plain text but Mastodon shows pre-render
     `text` + post-render `content`)
   * `PUBLIC_GET_TWEET_EDITS = "/public/get/tweet/edits/0.0.0"`
1. DTOs: `EditTweetEvent {tweet_id, user_id, text}`,
   `GetTweetSourceEvent`, `GetTweetEditsEvent`,
   `TweetSourceResponse {tweet_id, text, spoiler_text}`,
   `TweetEditsResponse {edits []domain.TweetEdit}`.
1. Handlers in `core/handler/tweet.go`:
   `StreamEditTweetHandler`, `StreamGetTweetSourceHandler`,
   `StreamGetTweetEditsHandler`. Edit must validate
   `userId == tweet.UserId` (only author can edit).
1. Registration grouped with the existing tweet routes.
1. Vue: add `editTweet({tweetId, text})`, `getTweetSource(tweetId)`,
   `getTweetEdits(tweetId)`. `Home.vue` compose overlay extended with
   an "edit" mode (existing prop wiring).
1. warpdroid: `ProtocolIds.kt` + DTOs + `WarpnetRepository`:
   `editStatus(tweetId, text)` returning a re-fetched
   `Status` via the existing `getStatus`.
1. `WarpnetApi.kt::editStatus` → real path; `statusSource` → real path;
   `statusEdits` → real path (replacing the empty-list stub).

### A3. Account update / preferences — `accountUpdateCredentials`, `accountUpdateSource`, `updateAccountNote`

**Why**: profile screen "edit profile" today fails on save (write-only
calls all stub-fail). Avatar/header upload fails. Note-on-other-account
fails.

`PRIVATE_POST_USER` already exists for self-edit but only Vue calls it
today — no `WarpnetRepository` method. Three Tusky entry points map to
two server-side concerns:

| Tusky call | Maps to |
|---|---|
| accountUpdateCredentials(displayName, note, locked, avatar, header, fields) | extend PRIVATE_POST_USER body; reuse PRIVATE_POST_UPLOAD_IMAGE for avatar/header |
| accountUpdateSource(privacy, sensitive, language, quotePolicy) | new PRIVATE_POST_USER_PREFS |
| updateAccountNote(accountId, note) | new PRIVATE_POST_USER_NOTE (stored locally only — like a private memo) |

**Plan**:

1. **Credentials**: extend `domain.User` with `displayName`, `note`,
   `locked`, `avatar`, `header`, `fields []UserField` if missing
   (audit `domain/user.go`). Extend
   `event.UpdateUserEvent` (today's payload) accordingly. No new path —
   reuse `PRIVATE_POST_USER`.
1. **Source/preferences**:
   * new path `PRIVATE_POST_USER_PREFS = "/private/post/user/prefs/0.0.0"`
   * DTO `UpdateUserPrefsEvent {privacy, sensitive, language, quote_policy}`
   * Repo: `database/user-prefs-repo.go` keyed by self user-id.
   * Handler `StreamUpdatePrefsHandler` in
     `core/handler/user.go` (next to `StreamUpdateProfileHandler`).
1. **Account note** (private memo about another user):
   * new path `PRIVATE_POST_USER_NOTE = "/private/post/user/note/0.0.0"`
   * DTO `UpdateAccountNoteEvent {target_user_id, note}`
   * Repo method on existing `userRepo`:
     `SetNote(selfId, targetId, note string) error`
   * `relationshipFor` (existing) extended to read the note and surface
     it as `Relationship.note`.
1. Vue: `editMyProfile` already exists. Add `editMyPrefs(prefs)` and
   `setAccountNote(targetId, note)`. UI: extend
   `EditProfileOverlay.vue` with a Preferences tab; add a "private
   note" inline editor on `Profile.vue` for non-self profiles.
1. warpdroid: `ProtocolIds.kt`, DTOs, repo methods
   `updateCredentials(...)`, `updatePrefs(...)`, `setAccountNote(...)`.
1. `WarpnetApi.kt::accountUpdateCredentials/Source` and
   `updateAccountNote` → real paths.

### A4. Block / Unblock — 4 methods

`blockAccount`, `unblockAccount`, `blocks`, `muteAccount`,
`unmuteAccount`, `mutes`, `muteConversation`, `unmuteConversation`.

**Semantics on Warpnet**: a block is local to the blocker's node — it
hides the blocked user from the blocker's timelines and refuses
incoming follow / reply / chat from them. Mute is identical UX-wise
but keeps two-way visibility intact (Warpnet stores it separately for
analytics-friendly differentiation).

**Plan** (do block + mute together, the wire shape is identical):

1. New routes:
   * `PRIVATE_POST_BLOCK = "/private/post/block/0.0.0"`
   * `PRIVATE_POST_UNBLOCK = "/private/post/unblock/0.0.0"`
   * `PRIVATE_GET_BLOCKS = "/private/get/blocks/0.0.0"`
   * `PRIVATE_POST_MUTE = "/private/post/mute/0.0.0"`
   * `PRIVATE_POST_UNMUTE = "/private/post/unmute/0.0.0"`
   * `PRIVATE_GET_MUTES = "/private/get/mutes/0.0.0"`
   * `PRIVATE_POST_MUTE_CONVERSATION = "/private/post/mute/conversation/0.0.0"`
   * `PRIVATE_POST_UNMUTE_CONVERSATION = "/private/post/unmute/conversation/0.0.0"`
1. DTOs:
   ```go
   type BlockEvent struct {
       BlockerId domain.ID `json:"blocker_id"`
       BlockeeId domain.ID `json:"blockee_id"`
   }
   type UnblockEvent = BlockEvent
   type GetBlocksEvent struct {
       UserId domain.ID `json:"user_id"`
       Cursor string    `json:"cursor"`
       Limit  uint64    `json:"limit"`
   }
   type GetBlocksResponse struct {
       Items  []domain.User `json:"items"`
       Cursor string        `json:"cursor"`
   }
   // Mute: same shape, distinct namespace.
   type MuteEvent struct {
       MuterId domain.ID `json:"muter_id"`
       MuteeId domain.ID `json:"mutee_id"`
   }
   type MuteConversationEvent struct {
       UserId  domain.ID `json:"user_id"`
       TweetId domain.ID `json:"tweet_id"`
   }
   ```
1. New repo `database/blocks-repo.go` (and `mutes-repo.go`).
   Storage: prefix `block:<blockerId> -> set<blockeeId>` and the
   reverse `block-by:<blockeeId> -> set<blockerId>` for fast filter
   on incoming streams.
1. Handlers `core/handler/block.go`, `core/handler/mute.go`. Each
   pair of write handlers does only the local mutation; there is no
   cross-node propagation (the blocked user must not learn they were
   blocked).
1. **Cross-cutting**: timeline, replies, follow, chat, message
   handlers must consult `blocksRepo.IsBlocked(self, candidate)` and
   filter inbound. Add as a single helper in
   `core/middleware/block-filter.go`; apply in the registration
   wrapper for those routes only.
1. Registration: 8 entries grouped together near the user routes.
1. Vue: add 8 methods on `warpnetService`. New view
   `views/Settings/Blocks.vue` and `views/Settings/Mutes.vue`. New
   actions on `Profile.vue` ("Block this user", "Mute this user").
1. warpdroid: 8 ProtocolIds, 8 DTO classes, 8 `WarpnetRepository`
   methods. Tusky already has the UI screens — they just need real
   wire calls. Mapper: `domain.User -> TimelineAccount`.
1. `WarpnetApi.kt::blockAccount/unblockAccount/blocks/muteAccount/
   unmuteAccount/mutes/muteConversation/unmuteConversation` → real
   paths.

**Signing**: these are write routes; until `EnvelopeSigner` is real
(see warpdroid §A4 in the skill), warpdroid will fail at runtime with
`SigningUnavailable`. Land Vue+server first, gate Android with a TODO.

### A5. Bookmarks — `bookmarkStatus`, `unbookmarkStatus`, `bookmarks`

Local-only shelf. No propagation.

**Plan**:

1. Routes:
   * `PRIVATE_POST_BOOKMARK = "/private/post/bookmark/0.0.0"`
   * `PRIVATE_POST_UNBOOKMARK = "/private/post/unbookmark/0.0.0"`
   * `PRIVATE_GET_BOOKMARKS = "/private/get/bookmarks/0.0.0"`
1. DTO `BookmarkEvent {user_id, tweet_id, owner_user_id}` —
   `owner_user_id` so the timeline render can fetch the tweet without
   a second resolution round-trip.
1. Repo `database/bookmark-repo.go`. Sorted by `created_at` desc for
   the list endpoint. Cursor = ULID of last entry.
1. Handler `core/handler/bookmark.go` —
   `StreamBookmarkHandler / StreamUnbookmarkHandler / StreamGetBookmarksHandler`.
1. Vue: 3 methods + view `views/Bookmarks.vue` reachable from `SideNav.vue`.
1. warpdroid: 3 ProtocolIds + DTOs + repo methods. `bookmarks()` must
   re-fetch every tweet (PUBLIC_GET_TWEET_BY_ID per id). `WarpnetMapper`
   gets `WarpnetTweet -> Status` already.
1. `WarpnetApi.kt::bookmarkStatus/unbookmarkStatus/bookmarks` → real.

### A6. Pin / Unpin — `pinStatus`, `unpinStatus`

Mark a tweet as pinned to the author's profile. Must propagate (any
node viewing the profile must see the pinned-flag).

**Plan**:

1. Routes:
   * `PUBLIC_POST_PIN = "/public/post/pin/0.0.0"`
   * `PUBLIC_POST_UNPIN = "/public/post/unpin/0.0.0"`
1. DTO `PinTweetEvent {user_id, tweet_id}`. Validate `user_id ==
   tweet.user_id` (authors only).
1. Repo: extend `database/tweet-repo.go` with `Pin(userId, tweetId)`,
   `Unpin(userId, tweetId)`, `PinnedTweets(userId)` (≤5). Storage:
   per-user `pinned:<userId> -> sorted-set<tweetId>`.
1. `domain.Tweet` gets `Pinned bool` (default false). Handler returns
   the updated tweet so client's `Tweet.pinned` flips.
1. Handler `core/handler/pin.go`. Propagate via `like.go` pattern (own
   node mutates; if owner is remote, this is a noop because only the
   owner can pin own tweets).
1. `accountStatuses` (= `PUBLIC_GET_TWEETS`) accepts a new optional
   `pinned_only bool` flag and reorders pinned tweets first when not
   filtering. Wire it into the existing `GetAllTweetsEvent`.
1. Vue: `pinTweet/unpinTweet`, prop on `TweetBlock.vue` to render a
   "📌 Pinned" badge, action on tweet menu.
1. warpdroid: ProtocolIds + DTOs + Repository methods. Mapper sets
   `Status.pinned`.
1. `WarpnetApi.kt::pinStatus/unpinStatus` → real paths.

### A7. Server-side account search — `searchAccounts`, `search` (accounts only)

Today `searchAccounts` pages through `listUsers` and filters
client-side. Works for ≤200 users but breaks at network scale. `search`
(unified) can stay stub for hashtags+statuses (Tier B), but the
accounts subset is solvable now.

**Plan**:

1. Route: `PUBLIC_GET_USERS_SEARCH = "/public/get/users/search/0.0.0"`.
1. DTO: `SearchUsersEvent {query, cursor, limit}` /
   `SearchUsersResponse {items []domain.User, cursor}`.
1. Repo: extend `database/user-repo.go` with a substring index built
   on `Username` and `DisplayName`. Use Badger's prefix-scan against a
   normalized (lower-cased) suffix-array key per token, stored under
   `usersearch:<token-prefix>`. Rebuild on user create / update / delete.
1. Handler `StreamSearchUsersHandler` in `core/handler/user.go`.
1. Vue: real backing for the existing `Search.vue` "People" tab via a
   new `searchUsers(query, cursor)` method.
1. warpdroid: ProtocolIds + DTOs + `WarpnetRepository.searchAccounts`.
   `WarpnetApi.kt::searchAccounts` switches from client-filter to
   `warpnet.searchAccounts(query, limit)`.

### A8. Status engagement lists — `statusLikedBy`, `statusRetweetedBy`

Today both stub-empty-list. Tusky's "X people liked this" overlay is
dead.

**Plan**:

1. Routes:
   * `PUBLIC_GET_TWEET_LIKERS = "/public/get/tweet/likers/0.0.0"`
   * `PUBLIC_GET_TWEET_RETWEETERS = "/public/get/tweet/retweeters/0.0.0"`
1. DTOs: `GetTweetLikersEvent {tweet_id, owner_user_id, cursor, limit}`,
   reuse `GetLikersResponse = UsersResponse` (already exists).
   Likewise for retweeters.
1. Repo: like-repo and retweet-repo already index per tweet; expose a
   new method `Likers(tweetId, cursor, limit)` returning
   `[]domain.User` plus next cursor. Same for retweeters.
1. Handler in `core/handler/like.go` and `core/handler/retweet.go`.
   Forward to the tweet owner's node (the canonical record lives there).
1. Vue: 2 methods, a `LikersOverlay.vue` and `RetweetersOverlay.vue`,
   wired from `TweetBlock.vue` numeric badges.
1. warpdroid: ProtocolIds + DTOs + Repository.
   `WarpnetApi.kt::statusLikedBy/statusRetweetedBy` → real paths.

### A9. Single notification — `notification(id)`

Today stub-errors. Push-tap and "open notification" deep links land on
nothing.

**Plan**:

1. Route: `PRIVATE_GET_NOTIFICATION = "/private/get/notification/0.0.0"`.
1. DTO: `GetNotificationEvent {user_id, notification_id}`.
1. Repo: extend `database/notification-repo.go` with `Get(userId, id)`.
1. Handler `StreamGetNotificationHandler` next to the list handler.
1. Vue: `getNotification(id)` method; deep-link route
   `/notifications/:id` opening a single-notification overlay.
1. warpdroid: ProtocolIds + DTO + Repository.
   `WarpnetApi.kt::notification` → real path.

### A10. Subscribe / unsubscribe to a user's posts — `subscribeAccount`, `unsubscribeAccount`

Tusky's "notify me about new posts from this user". Cleanly maps to a
private "watchlist" the local node uses to decide what to surface as a
notification.

**Plan**:

1. Routes:
   * `PRIVATE_POST_SUBSCRIBE_USER = "/private/post/subscribe/user/0.0.0"`
   * `PRIVATE_POST_UNSUBSCRIBE_USER = "/private/post/unsubscribe/user/0.0.0"`
1. DTO: `SubscribeUserEvent {self_id, target_id}`.
1. Repo: extend `database/user-repo.go` with `Subscribe / Unsubscribe /
   IsSubscribed`.
1. Notification-generation logic in `core/handler/tweet.go`'s ingest
   path consults `IsSubscribed(viewer, author)` and emits a
   "subscriber notification" if true (no schema change — reuse the
   `Notification` DTO with type=SUBSCRIBE).
1. `relationshipFor` already exists — extend `IsFollowingResponse` /
   `Relationship` with `Subscribed bool`.
1. Vue: 2 methods + bell-icon toggle in `Profile.vue`. Tusky has the
   button already — only the wire is missing.
1. warpdroid: ProtocolIds + DTOs + Repository.
   `WarpnetApi.kt::subscribeAccount/unsubscribeAccount` → real paths.

### A11. Updating media metadata — `updateMedia`, `getMedia`

Tusky compose lets users add/edit alt-text and focal point on uploaded
images before posting.

**Plan**:

1. Today's `PRIVATE_POST_UPLOAD_IMAGE` returns
   `UploadImageResponse {key}`. Add:
   * `PRIVATE_POST_MEDIA_META = "/private/post/media/meta/0.0.0"`
   * `PRIVATE_GET_MEDIA = "/private/get/media/0.0.0"`
1. DTOs:
   ```go
   type UpdateMediaMetaEvent struct {
       Key         string  `json:"key"`
       Description string  `json:"description"`
       FocusX      float32 `json:"focus_x"`
       FocusY      float32 `json:"focus_y"`
   }
   type GetMediaEvent struct { Key string `json:"key"` }
   type GetMediaResponse struct {
       Key         string  `json:"key"`
       Url         string  `json:"url"`
       Description string  `json:"description"`
       FocusX      float32 `json:"focus_x"`
       FocusY      float32 `json:"focus_y"`
   }
   ```
1. Repo: extend `database/image-repo.go` with metadata storage
   alongside the blob.
1. Handler `core/handler/media.go` adds two functions next to
   `StreamUploadImageHandler`.
1. The existing `domain.Tweet.media[]` shape carries `description` and
   `focus` so render-time lookup is "free".
1. Vue: `updateMedia({key, description, focus})` + UI add-alt-text
   modal in `Home.vue` compose. `EditProfileOverlay` already touches
   uploads; reuse.
1. warpdroid: ProtocolIds + DTOs + Repository.
   `WarpnetApi.kt::updateMedia/getMedia` → real paths.

## 5. Tier B — adaptations (design first, then code)

These need design discussion *before* implementation. Each is sketched
to show the route shape but the data model is non-trivial.

### B1. ~~Federated public timeline~~ — moved to Tier C

`publicTimeline` was originally drafted here on the assumption a
"recent tweets" gossip topic could synthesise a Mastodon-style
Federated tab. **This is wrong for Warpnet.** Warpnet is intentionally
scoped to direct follows + per-tweet pull on demand; a network-wide
firehose contradicts the privacy and bandwidth model (every node
would receive every tweet from every peer it's ever met). The two
timeline concepts Warpnet has — **timeline** (friends+recs,
`PRIVATE_GET_TIMELINE`) and **home** (own tweets,
`PUBLIC_GET_TWEETS` with `user_id=self`) — already cover everything a
Warpnet user reads; there is no third "everyone" feed.

`publicTimeline` therefore has no Warpnet equivalent and is moved
to **Tier C / §12 deletion list**. Today's fallback (returning the
caller's own feed) is meaningless and should go.

### B2. Hashtag timeline + tag follow — `hashtagTimeline`, `tag`,
`followTag`, `unfollowTag`, `followedTags`

Hashtags are not parsed today. Need:

1. A tokenizer in `core/handler/tweet.go::StreamNewTweetHandler` that
   extracts `#token` substrings and writes them to `database/tag-repo.go`
   keyed by `tag:<name> -> sorted-set<tweetId>`.
1. `PUBLIC_GET_TAG_TIMELINE = "/public/get/tag/timeline/0.0.0"`,
   `PUBLIC_POST_FOLLOW_TAG`, `PUBLIC_POST_UNFOLLOW_TAG`,
   `PRIVATE_GET_FOLLOWED_TAGS`, `PUBLIC_GET_TAG_INFO`.
1. Tag follows = local-only watchlist (like A10).
1. Vue: `Hashtag.vue` already exists as a UI shell — wire it up.
1. warpdroid: 5 ProtocolIds + DTOs + Repository.

### B3. Trending — `trendingTags`, `trendingStatuses`

Requires a windowed counter on the local feed-repo (B1) and tag-repo
(B2). Per-domain trend = top-N tags/statuses by interaction velocity
in last 24 h. Implement as a periodic recompute (every 5 min) into a
small in-memory cache served from `core/handler/trends.go`.

Routes: `PUBLIC_GET_TRENDS_TAGS`, `PUBLIC_GET_TRENDS_STATUSES`.

### B4. Polls — `voteInPoll`

`domain.Poll` does not exist. Compose adds a Poll button (today
disabled). Adding polls requires:

1. New `domain.Poll {id, options []PollOption, expires_at,
   multiple bool}`.
1. Mutation on `domain.Tweet` to optionally include a `Poll`.
1. Routes `PUBLIC_POST_POLL_VOTE`, `PUBLIC_GET_POLL`.
1. Repo `database/poll-repo.go` storing votes per (poll_id, user_id).
1. Vote propagation: vote on author's node; aggregate count returned.
1. Vue: poll compose UI in `Home.vue`; render in `TweetBlock.vue`.
1. warpdroid: ProtocolIds + DTOs + Repository; `WarpnetMapper`
   already has poll-shaping for Tusky.

This is the largest Tier B item; budget independently.

### B5. Quote tweets — `quotingStatuses`, `removeQuote`

Warpnet has retweets but no quote tweets. Needed:

1. `domain.Tweet.QuotedTweetId domain.ID`.
1. `PUBLIC_POST_QUOTE`, `PUBLIC_GET_QUOTING`, `PUBLIC_DELETE_QUOTE`.
1. Otherwise mirrors retweet plumbing.

### B6. Conversations API — `getConversations`, `deleteConversation`

Tusky's "Conversations" tab = self-mentions + replies-to-self timeline.
Implementable as a derived view over the existing `tweet-repo` via a
new index `convo:<userId> -> sorted-set<tweetId>` populated when a
reply touches `userId`.

Route: `PRIVATE_GET_CONVERSATIONS`. Delete is local-only (hide from
this index, don't delete the underlying tweet).

### B7. Scheduled tweets — `createScheduledStatus`, `scheduledTweets`,
`deleteScheduledStatus`

Need a queue + dispatcher goroutine on the node. Routes:
`PRIVATE_POST_SCHEDULED_TWEET`, `PRIVATE_GET_SCHEDULED_TWEETS`,
`PRIVATE_DELETE_SCHEDULED_TWEET`. Repo:
`database/scheduled-tweet-repo.go`. Member-node startup spawns the
dispatcher. Vue: schedule picker in `Home.vue` compose.

### B8. Filters — `getFilter`, `getFilters`, `createFilter`,
`updateFilter`, `deleteFilter`, `addFilterKeyword`,
`updateFilterKeyword`, `deleteFilterKeyword`

Server-side keyword/regex filtering of own timeline. Local-only.
Routes: `PRIVATE_*_FILTER` set (8 routes). Repo:
`database/filter-repo.go`. Apply in
`core/handler/timeline.go` after read, before return.

### B9. Lists / custom timelines — `createList`, `updateList`,
`getLists` etc.

Same shape as Mastodon lists: a named subset of follows whose
timeline you can pull. Maps to:
`PRIVATE_*_LIST` (CRUD), `PRIVATE_GET_LIST_TIMELINE`. Repo:
`database/list-repo.go`. Membership stored as
`list:<listId> -> set<userId>`.

### B10. Follow requests (private accounts) — `followRequests`,
`authorizeFollowRequest`, `rejectFollowRequest`

Requires `domain.User.Locked bool` (already proposed in A3) + a
pending-request queue. Routes:
`PRIVATE_GET_FOLLOW_REQUESTS`, `PRIVATE_POST_FOLLOW_REQUEST_AUTHORIZE`,
`PRIVATE_POST_FOLLOW_REQUEST_REJECT`. Modifies
`StreamFollowHandler` to write into the queue iff the target is locked.

### B11. Translation — `translate`

Warpnet has no translator. Either:
* keep stubbed (recommend), or
* delegate to a node-side configured external endpoint (expensive,
  privacy-leak; requires settings UI).

### B12. Reports — `report`

User report mechanism. Already `core/handler/moderation.go` exists
for *moderator-side* receipt. Add the *client-side* submission:
`PRIVATE_POST_REPORT = "/private/post/report/0.0.0"`. DTO carries the
target user/tweet + reason; the local node forwards to the
moderation network.

## 6. Tier C — keep stubbed, no plan

These have no Warpnet equivalent and will not get one:

```
authenticateApp / fetchOAuthToken / revokeOAuthToken (Warpnet pairs
nodes, doesn't OAuth);
pushNotificationSubscription / subscribePushNotifications /
updatePushNotificationSubscription / unsubscribePushNotifications
(Warpnet polls, no Web Push gateway);
domainBlocks / blockDomain / unblockDomain (no domain concept);
announcements / dismissAnnouncement / addAnnouncementReaction (no
admin-broadcast channel);
markersWithAuth / updateMarkersWithAuth (per-timeline read markers —
Tusky's own scroll restoration suffices);
clearNotifications (Warpnet notifs are derived; nothing to clear);
notificationPolicy / updateNotificationPolicy (no central policy;
pre-filter on the node via A10/A4 instead);
getCustomEmojis / getInstanceV1 / getInstance / getInstanceRules
(stub-fixed values are fine);
followedTags only when B2 lands;
publicTimeline (Mastodon-style federated tab — no Warpnet equivalent;
contradicts the no-firehose privacy model. The today's fallback to
the caller's own feed is meaningless. Slated for deletion in §12).
```

Action: **none**. The current stubs already return success-with-empty
where Tusky tolerates it.

## 7. Vue frontend gap (Tier A complement)

For each new backend route in Tier A, the Vue side gets:

1. A new path constant in `frontend/src/service/service.js` (top of file).
1. A new method on `warpnetService`, snake_case body, awaited
   `sendToNode`.
1. A view or component edit to surface the feature. Specifically:

| Tier A item | Vue surface (new files in **bold**) |
|---|---|
| A1 single tweet | extend router with `/tweets/:id`, route → **`views/Status.vue`** |
| A2 edit/source/edits | extend `Home.vue` compose overlay, **`views/StatusEdits.vue`** |
| A3 prefs/note | **`views/Settings/Preferences.vue`**, inline note editor on `Profile.vue` |
| A4 block/mute | **`views/Settings/Blocks.vue`**, **`views/Settings/Mutes.vue`**, dropdown on `User.vue`/`TweetBlock.vue` |
| A5 bookmarks | **`views/Bookmarks.vue`**, link in `SideNav.vue` |
| A6 pin | badge in `TweetBlock.vue`, action in `TweetBlock.vue` menu |
| A7 user search | replace stub in `views/Search.vue` "People" tab |
| A8 likers/retweeters | **`components/LikersOverlay.vue`**, **`components/RetweetersOverlay.vue`** |
| A9 single notification | router `/notifications/:id`, **`components/NotificationOverlay.vue`** |
| A10 subscribe user | bell button on `Profile.vue` |
| A11 media meta | alt-text modal in `Home.vue` compose |

There is **no** store layer to wire — the existing pattern of
"component awaits `warpnetService` directly" stays.

## 8. Phased delivery

| Phase | Items | Why this order |
|---|---|---|
| 0 | none — repo prerequisites | Land `tweetRepo.OwnerOfTweet` index + backfill so A1 has a substrate. |
| 1 | A1, A9 | Single-status / single-notification: unblocks deep-linking and notification taps. Smallest possible PRs to shake out the mid-air pieces (DTO + path + handler + repo + Vue + warpdroid). |
| 2 | A4, A5, A6 | Block/mute/bookmark/pin: discrete, no cross-cutting design. Block has the cross-cutting filter middleware — land it once, reuse. |
| 3 | A2, A3, A11 | Status edit, profile edit, media meta: all hit the *write* path, all expose `EnvelopeSigner` issue on Android. Group so the binding-extension PR (signing) reviews against multiple consumers. |
| 4 | A7, A8, A10 | Search-users, engagement lists, subscribe-user: pure read APIs once A1's index is in. |
| 5 | B6, B8, B12 | Conversations, filters, reports: small adaptations, no new domain. |
| 6 | B2, B3 | Hashtags + trending: depend on tweet-tokenizer change. |
| 7 | B5, B7, B9, B10 | Quotes, scheduled, lists, follow requests: each ≥1 week design + impl. |
| 8 | B4 | Polls: largest single feature. |

Each phase is one or more PRs targeting `claude/analyze-warpdroid-api-TpEE4`'s
descendants per warpnet-add-handler conventions. Commit message
imperative present tense. The pre-commit hook bumps `version` and
`snap/snapcraft.yaml` automatically — no manual bump.

## 9. Cross-cutting concerns

### 9.1 Envelope signing

Tier A items A2, A3, A4 (writes), A5 (writes), A6, A10 (writes), A11
(writes) plus most of Tier B all hit middleware-level signature
verification on the Warpnet side. Today `EnvelopeSigner = NoOpSigner`
in warpdroid means **none of these will run end-to-end on Android**
until either:

* `warpdroid/node/mobile.go` exposes `Sign(body) string` or
  `SetIdentityKey(pem)`, *or*
* `core/middleware/middleware.go` is taught to skip verification for
  paired clients on these specific routes (less safe).

Recommended: do the binding extension once before phase 2, batch all
Android-side wiring against it. The Go binding lives in a separate
module (`warpdroid/node/`, `module github.com/Warp-net/android-binding`)
and the `.aar` is committed (`warpdroid/warpnet-transport/libs/`). Any
change there requires running `warpdroid/node/build-native.sh` locally
and committing both the `.aar` and `.jar` deliberately.

### 9.2 Cursor pagination consistency

Mastodon clients expect `Link: <…?max_id=CURSOR>; rel="next"`.
`WarpnetApi.kt::paginated{}` already synthesises this header from a
`(items, cursor)` pair. Every new list-returning route must therefore
produce a `cursor` field in its response. Concretely: do **not** ship
list endpoints that return only `items` — even if there is no real
pagination, return `cursor: ""` so the wrapper compiles.

### 9.3 Path string drift

The path is declared in three places independently. Adding a route to
two of three is the most common bug in this codebase (skill §"Common
mistakes"). Fold a grep into the PR template:

```
git grep -F '"/public/post/foo/0.0.0"' \
    event/paths.go \
    frontend/src/service/service.js \
    warpdroid/warpnet-transport/src/main/kotlin/site/warpnet/transport/ProtocolIds.kt
```

All three lines must hit on every new route. Same for renames.

### 9.4 DTO drift

Server-side `event.go` JSON tags are the wire contract. Vue
`sendToNode` body keys and warpdroid Moshi `@Json(name=…)` tags must
match byte-for-byte. Add a JVM round-trip test per new DTO using a
captured Vue payload, exactly as warpnet-add-handler §A9 prescribes.

### 9.5 Tests

Per-handler `_test.go` is non-negotiable (skill §7): minimum coverage
is invalid payload, every empty-field error, the happy path, and
remote-call branches if any. Stub style uses local `<method>Fn` fields,
no testify, `package handler` (white-box), file starts with
`//nolint:all`. Reuse the `marshal(t, …)` helper from `like_test.go`.

## 10. Quick lookup: WarpnetApi.kt → plan section

| WarpnetApi.kt method | Today | Plan |
|---|---|---|
| accountVerifyCredentials | ✅ uses getAccount fallback to stub | done |
| account / accountStatuses / accountFollowers / accountFollowing | ✅ | done |
| accountUpdateCredentials | ❌ | **A3** |
| accountUpdateSource | ❌ | **A3** |
| addAccountToList / addFilterKeyword / authorizeFollowRequest | ❌ | B9 / B8 / B10 |
| announcements / dismissAnnouncement / addAnnouncementReaction / removeAnnouncementReaction | empty stub | C |
| authenticateApp / fetchOAuthToken / revokeOAuthToken | ❌ | C |
| blockAccount / unblockAccount / blocks | ❌ | **A4** |
| blockDomain / unblockDomain / domainBlocks | ❌ | C |
| bookmarkStatus / unbookmarkStatus / bookmarks | ❌ | **A5** |
| clearNotifications | empty stub | C |
| createFilter / updateFilter / deleteFilter / getFilter / getFilters / addFilterKeyword / updateFilterKeyword / deleteFilterKeyword | ❌ | B8 |
| createList / updateList / deleteList / getLists / getListsIncludesAccount / getAccountsInList / addAccountToList / deleteAccountFromList | ❌ | B9 |
| createScheduledStatus / scheduledTweets / deleteScheduledStatus | ❌ | B7 |
| createStatus | ✅ | done |
| deleteStatus | ✅ | done |
| editStatus | ❌ | **A2** |
| followAccount / unfollowAccount | ✅ | done |
| followedTags / followTag / unfollowTag / tag / hashtagTimeline | ❌ | B2 |
| followRequests / authorizeFollowRequest / rejectFollowRequest | empty/stub | B10 |
| getCustomEmojis / getInstance(V1) / getInstanceRules | empty stub | C |
| getConversations / deleteConversation | ❌ | B6 |
| getMedia / updateMedia | ❌ | **A11** |
| getNotificationRequests / acceptNotificationRequest / dismissNotificationRequest | empty stub | C |
| homeTimeline | ✅ | done |
| likeStatus / unlikeStatus | ✅ | done |
| likes (own likes list) | empty stub | A8 (via likers index can derive own — phase 4) |
| listTimeline | empty stub | B9 |
| markersWithAuth / updateMarkersWithAuth | empty stub | C |
| muteAccount / unmuteAccount / mutes | ❌ | **A4** |
| muteConversation / unmuteConversation | ❌ | **A4** |
| notification(id) | ❌ | **A9** |
| notifications / notificationsWithAuth | ✅ | done |
| notificationPolicy / updateNotificationPolicy | ❌ | C |
| pinStatus / unpinStatus | ❌ | **A6** |
| publicTimeline | falls back to user feed (meaningless) | **§12 delete** — no Warpnet equivalent |
| pushNotificationSubscription / subscribe/update/unsubscribe | ❌ | C |
| quotingStatuses / removeQuote | empty stub | B5 |
| recordView | ✅ | done |
| rejectFollowRequest | ❌ | B10 |
| relationships | ✅ | done |
| report | empty stub | B12 |
| retweetStatus / unretweetStatus | ✅ | done |
| search (unified) | ❌ | A7 (accounts) + B2 (hashtags) + future tweets |
| searchAccounts | ✅ client-side filter | **A7** (server-side) |
| status(id) | ❌ | **A1** |
| statusContext | ✅ | done |
| statusEdits | empty list stub | **A2** |
| statusLikedBy / statusRetweetedBy | empty stub | **A8** |
| statusSource | ❌ | **A2** |
| subscribeAccount / unsubscribeAccount | ❌ | **A10** |
| translate | ❌ | B11 (skip) |
| trendingTags / trendingStatuses | empty stub | B3 |
| updateAccountNote | ❌ | **A3** |
| voteInPoll | ❌ | B4 |

## 11. Done definition for any single Tier A item

A Tier A item is "done" when **all** of the following hold (skill §8):

- [ ] AGPL header present at top of every new `.go` file.
- [ ] Path constant in `event/paths.go`.
- [ ] DTO in `event/event.go` with the godoc one-liner.
- [ ] Repo method validates empty inputs, uses
      `NewTxn` + `defer Rollback` + `Commit`.
- [ ] Handler registered in
      `cmd/node/member/node/member-node.go`, near related routes,
      with `//nolint:govet`.
- [ ] Handler validates every input field before touching the repo.
- [ ] `_test.go` covers invalid payload + each empty-field error +
      happy path.
- [ ] `go test -short -p 8 -v ./core/handler/... ./database/...` green.
- [ ] `go vet ./...` and `golangci-lint run` clean.
- [ ] `vendor/` not modified.
- [ ] Same path string in `frontend/src/service/service.js`,
      byte-identical with `event/paths.go`.
- [ ] `warpnetService` method exists with the correct snake_case body.
- [ ] At least one Vue surface (component / view / route) consumes it.
- [ ] Same path string in
      `warpdroid/warpnet-transport/.../ProtocolIds.kt`.
- [ ] DTOs added to
      `warpdroid/warpnet-transport/.../dto/WarpnetDtos.kt` with
      `@Json(name = "snake_case")`.
- [ ] `WarpnetRepository` method exists, returns Mastodon-shaped types.
- [ ] If the route requires signature verification: TODO referencing
      `EnvelopeSigner.kt`, OR signing has been landed.
- [ ] If `warpdroid/node/*.go` changed: regenerated `warpnet.aar` /
      `warpnet-sources.jar` committed.
- [ ] JVM tests for envelope shape + DTO round-trip.
- [ ] `./gradlew :app:assembleDebug` succeeds.
- [ ] `WarpnetApi.kt` switched from stub to real call.
- [ ] PR description names the path so reviewers can grep all three sources.

## 12. To delete from warpdroid (no Warpnet equivalent)

The methods below carry no Warpnet meaning and never will — implementing
them would require introducing concepts that contradict Warpnet's
architecture (no instance / no central admin / no domain federation /
no Web Push gateway / no OAuth server / no read-marker sync). Today
they live in `WarpnetApi.kt` only because Tusky was a Mastodon client
and its view-models call them by default. They are dead weight on the
client surface and should be removed in a single coordinated PR
together with the Tusky UI that calls them.

**Important** — this list is **strictly Tier C from §6**. The methods
in §3.2 that map to a Tier B item (filters, lists, conversations,
follow-requests, scheduled tweets, hashtags, trends, quotes, polls,
reports) **must stay as soft-stubs** until their Tier B lands; do not
delete them.

### 12.1 Methods to delete from `WarpnetApi.kt`

Line numbers refer to the current file (920 lines).

| Group | Methods | Lines | Why removable |
|---|---|---|---|
| OAuth | `authenticateApp`, `fetchOAuthToken`, `revokeOAuthToken` | 703–724 | Warpnet pairs nodes via QR + challenge (`PRIVATE_POST_PAIR` / `PUBLIC_POST_NODE_CHALLENGE`); no OAuth server exists, none planned. |
| Web Push | `pushNotificationSubscription`, `subscribePushNotifications`, `updatePushNotificationSubscription`, `unsubscribePushNotifications` | 825–849 | Warpnet has no push gateway; notifications are 2 s polling against `PRIVATE_GET_NOTIFICATIONS`. |
| Domain blocks | `blockDomain`, `unblockDomain`, `domainBlocks` | 668–675 | No "domain" concept in P2P; per-user block is covered by **A4**. |
| Instance metadata | `getInstanceV1`, `getInstance`, `getInstanceRules`, `getCustomEmojis` | 145–186 | No "instance" concept; the only fact Tusky actually consumes (max chars) belongs in a local constant, not a network call. |
| Announcements | `announcements`, `dismissAnnouncement`, `addAnnouncementReaction`, `removeAnnouncementReaction` | 783–796 | No central admin to broadcast announcements. |
| Read markers | `markersWithAuth`, `updateMarkersWithAuth` | 290–301 | No server-side scroll-position sync; Tusky already remembers locally. |
| Notification housekeeping | `clearNotifications`, `notificationPolicy`, `updateNotificationPolicy`, `getNotificationRequests`, `acceptNotificationRequest`, `dismissNotificationRequest` | 315, 889–910 | Warpnet notifications are derived from events (no read-acknowledge); per-user filtering happens via **A4** mute + **A10** subscribe, not via a central policy. |
| Translation | `translate` | 880–883 | No translator on the node and no privacy-acceptable way to add one. Decision in **B11**: skip. |
| Public/federated timeline | `publicTimeline` | 237–249 | No Warpnet equivalent — Warpnet is direct-follows-only by design; a network-wide firehose contradicts the privacy/bandwidth model. Today's fallback to the caller's own feed is meaningless. Tusky's "Public" / "Federated" tab goes with it. |

**Total: 27 methods to delete.**

### 12.2 Tusky UI surface to delete alongside

Removing the methods will break compilation in any Tusky view-model
or fragment that imports them. Before the deletion PR opens, audit
warpdroid for callers — typical removal targets:

| Method group | Likely Tusky surface |
|---|---|
| OAuth | the entire login activity / `LoginActivity*`, `OauthLogin` flow. Warpdroid's pairing flow (QR scan, `PairedNodeStore`) replaces it; the OAuth screens are unreachable already. |
| Web Push | `PushNotificationHelper`, `UnifiedPushBroadcastReceiver`, settings entry "Push notifications". |
| Domain blocks | "Blocked domains" preference screen + adapter. |
| Instance metadata | `InstanceInfoRepository` callers — replace `getInstanceV1().maxTootChars` reads with a single `WarpnetLimits.MAX_TWEET_CHARS = 2000` constant in `site.warpnet.transport`. The compose-screen char counter is the only consumer that matters. |
| Announcements | `AnnouncementsActivity`, the "Announcements" badge in the main menu. |
| Read markers | `NotificationsViewModel.markAs*`, `TimelineRepository.saveReadingPosition` — leave Tusky's local `SharedPreferences`-backed scroll restore, drop the network call. |
| Notification housekeeping | "Clear notifications" menu item, "Notification policy" preference screen, `NotificationRequestsActivity`. |
| Translation | "Translate" item in the status overflow menu. |
| Public timeline | The "Public" / "Federated" tab in Tusky's bottom navigation (`MainActivity` tab adapter, `TimelineFragment` instantiated with `Kind.PUBLIC_FEDERATED` / `Kind.PUBLIC_LOCAL`). Drop the tab entirely — Warpnet has only **timeline** (friends+recs) and **home** (own tweets); a third "everyone" tab has no source. |

For each removed entry point, also remove the route registration in
the navigation graph / preference XML so the feature does not appear
in the UI before the user clicks a dead button.

### 12.3 Entity classes that become unused

After the methods above are deleted, these `site.warpnet.warpdroid.entity.*`
classes are referenced nowhere and can be deleted in the same PR (verify
with `grep -r '<ClassName>' warpdroid/`):

```
AccessToken
Announcement
AppCredentials
Emoji                   // unless retained for Tusky's local-only emoji support
Instance
InstanceConfiguration
InstanceV1
Marker
NotificationPolicy      // keep if B-tier filtering re-uses the shape
NotificationRequest     // keep if B-tier filtering re-uses the shape
NotificationSubscribeResult
Translation
TweetConfiguration      // see "Instance metadata" row above
```

**Do not delete** these entities even though they appear in current
stubs — they are Tier B placeholders and will be wired in:

```
Filter, FilterKeyword          → B8
MastoList                      → B9
Conversation                   → B6
ScheduledTweet,                → B7
ScheduledTweetReply            → B7
Poll                           → B4
HashTag, TrendingTag           → B2 / B3
TweetEdit, TweetSource         → A2
DeletedTweet                   → already used by deleteStatus (A done)
```

### 12.4 Suggested PR shape

One PR titled `warpdroid: drop Mastodon-only API surface`:

1. Delete the 26 methods from `WarpnetApi.kt`.
2. Delete the entity classes from §12.3.
3. Delete unreachable Tusky activities/fragments/preferences from §12.2.
4. Remove the now-orphaned imports at the top of `WarpnetApi.kt`
   (`Announcement`, `AppCredentials`, `Emoji`, `Instance`, `InstanceV1`,
   `InstanceConfiguration`, `TweetConfiguration`, `Marker`,
   `NotificationPolicy`, `NotificationRequest`,
   `NotificationSubscribeResult`, `Translation`, `AccessToken`).
5. Drop helpers that become unused (`stubFailure`, `stubError`,
   `stubList`, the `unsupported(name)` builder) **only if** no
   remaining Tier B placeholder still references them — most likely
   the helpers stay, just with fewer call sites.
6. `./gradlew :app:assembleDebug` must pass before the PR opens.

This is a destructive, deliberately large diff. Land it after Tier A
phases 1–4 are done so the UI gaps left behind (login screen,
notifications drawer) are filled with Warpnet-native equivalents
already, not with empty stubs.
