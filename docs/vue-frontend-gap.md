# Vue Frontend Gap vs warpdroid — Comparison Report

Generated 2026-05-16 against branch `claude/analyze-warpdroid-api-XsAhS`.
Scope: every method on `WarpnetApi.kt` (warpdroid) mapped to its
counterpart on `warpnetService` (Vue `service.js`) and the UI surface
that uses it.

Counts: warpdroid exposes 104 API methods across 11 plan-defined
feature groups; Vue exposes 83 service methods across 11 views.
Numerical gap is ~21 methods, but the **UI gap is much wider** — Vue
is missing entire sections (Bookmarks list, Settings, single-status
deep-link, status edits, etc.) that warpdroid renders today.

This report is read-only. No code changed.

---

## Legend

| Tag | Meaning |
|---|---|
| ✅ | Vue has full UI + service for this |
| 🟡 | Vue has service method but **no UI** uses it |
| 🟡 partial | Vue has incomplete UI surface (one entry point but not all) |
| ❌ | Missing in Vue entirely |
| ⏭ §12 | Tier C — slated for delete from warpdroid in `docs/warpdroid-api-plan.md` §12; **do not add to Vue** |
| 🅱 §6 | Tier B from plan — keep as soft-stub on warpdroid until the B item lands |

---

## A. Tweets (read + write)

| Feature | warpdroid method | Vue method | Vue UI | Status |
|---|---|---|---|---|
| Get single tweet | `status` | `getTweet` | none — there is no `Status.vue` / `/tweets/:id` route | 🟡 service only |
| Tweet stats | `status` (returns stats inline) | `getTweetStats` | `TweetBlock.vue` reads counts | ✅ |
| Get thread context | `statusContext` | `getReplies` | `Conversation.vue` (replies tab) | 🟡 partial — Vue has only one direction (replies); ancestors not shown |
| Home timeline | `homeTimeline` | `getMyTimeline` | `Home.vue` | ✅ |
| User timeline | `accountStatuses` | `getTweets` | `Profile.vue` | ✅ |
| Create tweet | `createStatus` | `createTweet` | `Home.vue` compose box | ✅ |
| Reply | (`createStatus` with `inReplyToId`) | `replyTweet` | `ReplyOverlay.vue` | ✅ |
| Delete tweet | `deleteStatus` | `deleteTweet` | `TweetBlock.vue` overflow menu | ✅ |
| Edit tweet | `editStatus` | `editTweet` | none — no edit overlay anywhere | ❌ UI missing |
| Source for edit | `statusSource` | none | none | ❌ both missing |
| Edit revision history | `statusEdits` | none | none | ⏭ §12 — handler removed, do not add |
| Retweet / un-retweet | `retweetStatus` / `unretweetStatus` | `retweetTweet` / `unretweetTweet` | `TweetBlock.vue` button | ✅ |
| Like / unlike | `likeStatus` / `unlikeStatus` | `likeTweet` / `unlikeTweet` | `TweetBlock.vue` button | ✅ |
| Bookmark / un | `bookmarkStatus` / `unbookmarkStatus` | `bookmarkTweet` / `unbookmarkTweet` | `TweetBlock.vue` overflow menu (action only) | 🟡 partial — no Bookmarks list view |
| List of bookmarks | `bookmarks` | `getBookmarks` | none — there is no `Bookmarks.vue` view | 🟡 service only |
| Pin / un | `pinStatus` / `unpinStatus` | `pinTweet` / `unpinTweet` | `Profile.vue` overflow (action only) | 🟡 partial — no pinned badge on tweets, no pinned-first sort on profile |
| Likers list | `statusLikedBy` | `getTweetLikers` | none — no `LikersOverlay.vue` | 🟡 service only |
| Retweeters list | `statusRetweetedBy` | `getTweetRetweeters` | none — no `RetweetersOverlay.vue` | 🟡 service only |
| Record view | `recordView` | `viewTweet` | called from `TweetBlock.vue` mount | ✅ |
| Quote tweet | `quotingStatuses` | `quoteTweet` / `getQuoting` / `deleteQuote` | partial in `TweetBlock.vue` | 🅱 §6 B5 — partial OK |

## B. Accounts / users

| Feature | warpdroid | Vue | Vue UI | Status |
|---|---|---|---|---|
| Get account | `account` | `getProfile` | `Profile.vue`, `User.vue` | ✅ |
| Verify own credentials | `accountVerifyCredentials` | `signInUser` (kind-of), `getProfile` | implicit on app load | ✅ |
| Update profile | `accountUpdateCredentials` | `editMyProfile` | `EditProfileOverlay.vue` | ✅ |
| Update source (privacy default) | `accountUpdateSource` | none | none | ❌ — needs settings view |
| Followers list | `accountFollowers` | `getFollowers` | `Followers.vue` | ✅ |
| Following list | `accountFollowing` | `getFollowings` | `Following.vue` | ✅ |
| Follow / unfollow | `followAccount` / `unfollowAccount` | `followUser` / `unfollowUser` | `User.vue`, `Profile.vue` | ✅ |
| isFollowing / isFollower | (via `relationships`) | `isFollowing` / `isFollower` | `Profile.vue` follow button state | ✅ |
| Relationships (batch) | `relationships` | none | none | ❌ — Vue falls back to per-user `isFollowing` |
| Account note (private) | `updateAccountNote` | none | none | ⏭ §12 — already deleted from backend |
| Block / unblock | `blockAccount` / `unblockAccount` | `blockUser` / `unblockUser` | `User.vue` / `Profile.vue` action only | 🟡 partial — no Blocks list view |
| List of blocks | `blocks` | `getBlocks` | none — no `Settings/Blocks.vue` | 🟡 service only |
| Mute / unmute | `muteAccount` / `unmuteAccount` | `muteUser` / `unmuteUser` | `User.vue` / `Profile.vue` action only | 🟡 partial — no Mutes list view |
| List of mutes | `mutes` | `getMutes` | none — no `Settings/Mutes.vue` | 🟡 service only |
| Subscribe / unsubscribe | `subscribeAccount` / `unsubscribeAccount` | `subscribeUser` / `unsubscribeUser` | none — no bell button on `Profile.vue` | 🟡 service only |
| Account search | `searchAccounts` | `searchUsers` | `Search.vue` "People" tab + `SearchBar.vue` | ✅ |
| User list (browse) | none | `getUsers` | `WhoToFollow.vue` | ✅ — Vue-only feature |
| Who-to-follow | none | `getWhoToFollow` | `WhoToFollow.vue` | ✅ — Vue-only feature |

## C. Notifications

| Feature | warpdroid | Vue | Vue UI | Status |
|---|---|---|---|---|
| Notifications list | `notifications`, `notificationsWithAuth` | `getNotifications` | `Notifications.vue` | ✅ |
| Single notification | `notification` | `getNotification` | none — no `NotificationOverlay.vue` / deep-link route | 🟡 service only |
| Filtered-notifications tray (B13) | `getNotificationRequests`, `acceptNotificationRequest`, `dismissNotificationRequest` | none | none | 🅱 §6 B13 — pending |
| Push subscription | `pushNotificationSubscription` etc. | none | none | ⏭ §12 — no push gateway |
| Notification policy preference | (removed) | none | none | ⏭ §12 — deleted from backend |
| Clear all | (removed) | none | none | ⏭ §12 — deleted |
| Read markers | (removed) | none | none | ⏭ §12 — deleted |

## D. Conversations / DMs

| Feature | warpdroid | Vue | Vue UI | Status |
|---|---|---|---|---|
| Conversations list | `getConversations` | `getChats` / `getChat` | `Conversations.vue`, `Conversation.vue`, `Messages.vue` | ✅ — Vue uses chat-shaped API, warpdroid uses Mastodon Conversation entity |
| Delete conversation | `deleteConversation` | `deleteChat` | `Conversations.vue` | ✅ |
| Send DM | (uses `createStatus` with visibility=DIRECT) | `sendDirectMessage` | `NewMessageOverlay.vue` | ✅ |
| List DMs in a chat | none | `getDirectMessages` | `Conversation.vue` | ✅ — Vue-only API |

## E. Search

| Feature | warpdroid | Vue | Vue UI | Status |
|---|---|---|---|---|
| Generic search | `search` (statuses + accounts + tags) | `searchUsers` | `Search.vue` "People" tab | 🟡 partial — Vue can't search statuses or tags |

## F. Filters

| Feature | warpdroid | Vue | Vue UI | Status |
|---|---|---|---|---|
| List filters | `getFilters` | `getFilters` | none — no Settings/Filters view | 🟡 service only |
| Get one | `getFilter` | `getFilter` | none | 🟡 service only |
| Create / update / delete | `createFilter`, `updateFilter`, `deleteFilter` | same names | none | 🟡 service only |
| Keyword CRUD | `addFilterKeyword` etc. | same names | none | 🟡 service only |

Filters back-end is complete on both sides; Vue is missing the entire
preference screen to manage them.

## G. Follow requests (locked-account flow)

| Feature | warpdroid | Vue | Vue UI | Status |
|---|---|---|---|---|
| List pending requests | `followRequests` | `getFollowRequests` | none — no inbound-requests view | 🟡 service only |
| Authorize / reject | `authorizeFollowRequest` / `rejectFollowRequest` | same names | none | 🟡 service only |

## H. Lists (B9)

| Feature | warpdroid | Vue | Vue UI | Status |
|---|---|---|---|---|
| All list CRUD + membership + list timeline | `getLists`, `createList`, `updateList`, `deleteList`, `addAccountToList`, `deleteAccountFromList`, `getAccountsInList`, `getListsIncludesAccount`, `listTimeline` | none | none | 🅱 §6 B9 — pending |

## I. Hashtags / trending (B2/B3)

| Feature | warpdroid | Vue | Vue UI | Status |
|---|---|---|---|---|
| Hashtag timeline | `hashtagTimeline` | none | `Hashtag.vue` exists but unwired | 🅱 §6 B2 |
| Followed tags | `followedTags`, `followTag`, `unfollowTag`, `tag` | none | none | 🅱 §6 B2 |
| Trending tags | `trendingTags` | none | none | 🅱 §6 B3 |
| Trending statuses | `trendingStatuses` | none | none | 🅱 §6 B3 |

## J. Media

| Feature | warpdroid | Vue | Vue UI | Status |
|---|---|---|---|---|
| Upload image | (multipart, not on `WarpnetApi`) | `uploadImage`, `uploadImages` | `Home.vue` compose | ✅ |
| Get image | `getMedia` | `getImage` | `TweetBlock.vue` inline | ✅ |
| Get media meta | (via `getMedia` payload) | `getMediaMeta` | none — no alt-text modal | 🟡 service only |
| Update media meta (alt-text + focal point) | `updateMedia` | `updateMediaMeta` | none — no edit-media modal | 🟡 service only |

## K. Reports (B12)

| Feature | warpdroid | Vue | Vue UI | Status |
|---|---|---|---|---|
| Report a user / status | `report` | none | none | 🅱 §6 B12 |

## L. Scheduled tweets (B7)

| Feature | warpdroid | Vue | Vue UI | Status |
|---|---|---|---|---|
| List / create / delete scheduled | `scheduledTweets`, `createScheduledStatus`, `deleteScheduledStatus` | none | none | 🅱 §6 B7 |

## M. Stubbed / to-delete from warpdroid (`⏭ §12`)

These are **not** Vue gaps — they should disappear from warpdroid in
batch 2 of the deletion PR. Adding them to Vue would be churn.

- OAuth: `authenticateApp`, `fetchOAuthToken`, `revokeOAuthToken`
- Web push: `pushNotificationSubscription`, `subscribePushNotifications`, `updatePushNotificationSubscription`, `unsubscribePushNotifications`
- Domain blocks: deleted
- Instance metadata: `getInstance`, `getInstanceV1`, `getInstanceRules` (kept on warpdroid until §12 batch 2)
- Announcements: deleted
- Read markers: deleted
- Translation: deleted
- Public/federated timeline: `publicTimeline` (kept until §12 batch 2)
- Mute conversation: `muteConversation` / `unmuteConversation` (deleted)
- Custom emojis: deleted
- Notification policy: deleted

---

## Summary of actionable Vue gaps

Sorted by §8 phased-delivery order (smallest first).

### Phase 1 — deep-linking

1. **A1 — Single tweet view**
   - New file: `frontend/src/views/Status.vue`
   - Router: add `/tweets/:id` route
   - Wires `warpnetService.getTweet`, `getReplies`, `getTweetStats`
   - Unlocks notification taps and "open conversation in new tab"
2. **A9 — Single notification overlay**
   - New file: `frontend/src/components/NotificationOverlay.vue` (or extend `Notifications.vue`)
   - Wires `warpnetService.getNotification`
   - Backend route already exists (`PRIVATE_GET_NOTIFICATION`)

### Phase 2 — discrete actions

3. **A4 — Block/mute settings views**
   - New: `views/Settings/Blocks.vue`, `views/Settings/Mutes.vue`
   - Wires `getBlocks` / `getMutes` + per-row `unblockUser` / `unmuteUser`
   - Adds Settings entry to `SideNav.vue`
   - Block/mute action buttons already exist on `User.vue` / `Profile.vue`
4. **A5 — Bookmarks list**
   - New: `views/Bookmarks.vue`
   - Wires `getBookmarks`
   - Adds Bookmarks link to `SideNav.vue`
5. **A6 — Pin polish**
   - Pinned-tweet badge in `TweetBlock.vue`
   - Pinned-first sort on `Profile.vue`

### Phase 3 — write-path & profile

6. **A2 — Edit tweet overlay**
   - Extend `Home.vue` compose to accept "edit mode"
   - Wires `statusSource` (load original) + `editTweet`
7. **A3 — Preferences view**
   - New: `views/Settings/Preferences.vue`
   - Wires `accountUpdateSource` (default-visibility, default-sensitive, default-language)
8. **A11 — Alt-text modal**
   - Modal in `Home.vue` compose
   - Wires `getMediaMeta` + `updateMediaMeta`

### Phase 4 — read APIs

9. **A7 — Statuses + hashtags in search**
   - Extend `Search.vue` with "Posts" and "Tags" tabs (currently only "People")
   - Wires `search` (warpdroid) — needs a new Vue service method that hits the same backend route
10. **A8 — Likers / retweeters overlays**
    - New: `components/LikersOverlay.vue`, `components/RetweetersOverlay.vue`
    - Triggered from like/retweet count tap in `TweetBlock.vue`
11. **A10 — Subscribe bell on Profile**
    - Bell button on `Profile.vue` next to Follow
    - Wires `subscribeUser` / `unsubscribeUser`

### Phase 5+ — B-tier (need backend changes too)

- B6 conversations adaptations
- B8 filters preference UI (Vue service is complete; just needs the screen)
- B12 reports
- B13 filtered-notifications tray
- B2/B3 hashtags + trending
- B5 quotes polish
- B7 scheduled tweets
- B9 lists
- B10 follow-request inbound tray (Vue service is complete; just needs the screen)
- B4 polls

---

## Quick stats

- **Vue gaps with backend already in place** (just needs UI): 14 items
  (single-tweet, single-notification, bookmarks list, blocks list,
  mutes list, subscribe bell, edit tweet, source, likers, retweeters,
  filters screen, follow-requests tray, media-meta modal, pinned
  polish).
- **Vue gaps blocked by Tier B backend work**: 9 items (lists, reports,
  scheduled tweets, polls, hashtags, trending, quotes refinement,
  conversations adaptation, filtered-notifications tray).
- **Vue methods that exist but never called by any `.vue` file**:
  `getBlocks`, `getBookmarks`, `getMediaMeta`, `getMutes`,
  `getFollowRequests`, `getFilter`, `getFilters`, `createFilter`,
  `updateFilter`, `deleteFilter`, `addFilterKeyword`,
  `updateFilterKeyword`, `deleteFilterKeyword`, `subscribeUser`,
  `unsubscribeUser`, `getTweetLikers`, `getTweetRetweeters`,
  `getNotification`, `updateMediaMeta`, `authorizeFollowRequest`,
  `rejectFollowRequest`, `editTweet`, `getTweetStats` (≈ 23 methods).
  These are the easiest UI items — service layer already returns the
  shape, the only work left is the component.
