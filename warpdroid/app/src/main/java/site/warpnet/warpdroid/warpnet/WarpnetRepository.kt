/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.warpnet

import android.util.LruCache
import site.warpnet.warpdroid.cache.TtlLruCache
import site.warpnet.warpdroid.components.pairing.PairedNodeStore
import site.warpnet.warpdroid.entity.User
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.Relationship
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.entity.TimelineUser
import site.warpnet.warpdroid.warpnet.WarpnetMapper.toAccount
import site.warpnet.warpdroid.warpnet.WarpnetMapper.toNotification
import site.warpnet.warpdroid.warpnet.WarpnetMapper.toTweet
import site.warpnet.warpdroid.warpnet.WarpnetMapper.toTimelineUser
import com.squareup.moshi.Moshi
import com.squareup.moshi.adapter
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import site.warpnet.transport.ProtocolIds
import site.warpnet.transport.WarpnetClient
import site.warpnet.transport.dto.DeleteTweetEvent
import site.warpnet.transport.dto.FollowersResponse
import site.warpnet.transport.dto.FollowingsResponse
import site.warpnet.transport.dto.GetAllRepliesEvent
import site.warpnet.transport.dto.GetAllTweetsEvent
import site.warpnet.transport.dto.GetAllUsersEvent
import site.warpnet.transport.dto.GetFollowersEvent
import site.warpnet.transport.dto.GetFollowingsEvent
import site.warpnet.transport.dto.GetIsFollowingEvent
import site.warpnet.transport.dto.GetNotificationsEvent
import site.warpnet.transport.dto.GetNotificationsResponse
import site.warpnet.transport.dto.GetTweetEvent
import site.warpnet.transport.dto.GetTweetStatsEvent
import site.warpnet.transport.dto.GetUserEvent
import site.warpnet.transport.dto.IsFollowerResponse
import site.warpnet.transport.dto.IsFollowingResponse
import site.warpnet.transport.dto.LikeEvent
import site.warpnet.transport.dto.LikesCountResponse
import site.warpnet.transport.dto.NewFollowEvent
import site.warpnet.transport.dto.NewUnfollowEvent
import site.warpnet.transport.dto.RepliesResponse
import site.warpnet.transport.dto.TweetStatsResponse
import site.warpnet.transport.dto.TweetsResponse
import site.warpnet.transport.dto.UnretweetEvent
import site.warpnet.transport.dto.UsersResponse
import site.warpnet.transport.dto.ViewEvent
import site.warpnet.transport.dto.ViewsCountResponse
import site.warpnet.transport.dto.WarpnetTweet
import site.warpnet.transport.dto.WarpnetUser

/**
 * The only entry point from view models into the Warpnet transport.
 *
 * Scope is deliberately small: this is the set of operations that the
 * existing Warpdroid view models actually need to render the home timeline,
 * one user profile, posting/deleting tweets, follow/unfollow and
 * notifications. Endpoints without Warpnet equivalents
 * (filters, announcements, trending, lists, reports, scheduled statuses,
 * translations, polls, media uploads) are **not** exposed here — the
 * corresponding Warpdroid features are removed in Phase 5 rather than faked.
 *
 * Author lookups are resolved lazily via [resolveUser], which layers a
 * per-call cache over a process-wide TTL'd [userCache]; a 40-tweet
 * timeline from 3 distinct authors hits `GET_USER` at most three times,
 * and repeat views across pages/screens reuse the cached profiles
 * instead of re-paying a relay round-trip.
 */
@OptIn(ExperimentalStdlibApi::class)
@Singleton
class WarpnetRepository @Inject constructor(
    private val client: WarpnetClient,
    private val pairedNodeStore: PairedNodeStore,
    private val accountManager: site.warpnet.warpdroid.db.AccountManager,
    moshi: Moshi,
) {
    private val userAdapter = moshi.adapter<WarpnetUser>()
    private val tweetAdapter = moshi.adapter<WarpnetTweet>()
    private val tweetsRespAdapter = moshi.adapter<TweetsResponse>()
    private val repliesRespAdapter = moshi.adapter<RepliesResponse>()
    private val notificationsRespAdapter = moshi.adapter<GetNotificationsResponse>()
    private val notificationRespAdapter = moshi.adapter<site.warpnet.transport.dto.WarpnetNotification>()
    private val likesCountAdapter = moshi.adapter<LikesCountResponse>()
    private val tweetStatsRespAdapter = moshi.adapter<TweetStatsResponse>()
    private val usersRespAdapter = moshi.adapter<UsersResponse>()
    private val followersRespAdapter = moshi.adapter<FollowersResponse>()
    private val followingsRespAdapter = moshi.adapter<FollowingsResponse>()
    private val isFollowingRespAdapter = moshi.adapter<IsFollowingResponse>()
    private val isFollowerRespAdapter = moshi.adapter<IsFollowerResponse>()

    private val getUserAdapter = moshi.adapter<GetUserEvent>()
    private val getAllTweetsAdapter = moshi.adapter<GetAllTweetsEvent>()
    private val getAllUsersAdapter = moshi.adapter<GetAllUsersEvent>()
    private val getTweetAdapter = moshi.adapter<GetTweetEvent>()
    private val getTweetStatsAdapter = moshi.adapter<GetTweetStatsEvent>()
    private val getRepliesAdapter = moshi.adapter<GetAllRepliesEvent>()
    private val getNotifsAdapter = moshi.adapter<GetNotificationsEvent>()
    private val getNotifAdapter = moshi.adapter<site.warpnet.transport.dto.GetNotificationEvent>()
    private val markNotifReadAdapter = moshi.adapter<site.warpnet.transport.dto.MarkNotificationReadEvent>()
    private val bookmarkEventAdapter = moshi.adapter<site.warpnet.transport.dto.BookmarkEvent>()
    private val unbookmarkEventAdapter = moshi.adapter<site.warpnet.transport.dto.UnbookmarkEvent>()
    private val getBookmarksEventAdapter = moshi.adapter<site.warpnet.transport.dto.GetBookmarksEvent>()
    private val getBookmarksRespAdapter = moshi.adapter<site.warpnet.transport.dto.GetBookmarksResponse>()
    private val pinTweetAdapter = moshi.adapter<site.warpnet.transport.dto.PinTweetEvent>()
    private val blockEventAdapter = moshi.adapter<site.warpnet.transport.dto.BlockEvent>()
    private val muteEventAdapter = moshi.adapter<site.warpnet.transport.dto.MuteEvent>()
    private val getBlocksEventAdapter = moshi.adapter<site.warpnet.transport.dto.GetBlocksEvent>()
    private val getBlocksRespAdapter = moshi.adapter<site.warpnet.transport.dto.GetBlocksResponse>()
    private val getTweetLikersAdapter = moshi.adapter<site.warpnet.transport.dto.GetTweetLikersEvent>()
    private val subscribeUserAdapter = moshi.adapter<site.warpnet.transport.dto.SubscribeUserEvent>()
    private val updateMediaMetaAdapter = moshi.adapter<site.warpnet.transport.dto.UpdateMediaMetaEvent>()
    private val getMediaAdapter = moshi.adapter<site.warpnet.transport.dto.GetMediaEvent>()
    private val getMediaRespAdapter = moshi.adapter<site.warpnet.transport.dto.GetMediaResponse>()
    private val getImageEventAdapter = moshi.adapter<site.warpnet.transport.dto.GetImageEvent>()
    private val getImageRespAdapter = moshi.adapter<site.warpnet.transport.dto.GetImageResponse>()
    private val searchUsersAdapter = moshi.adapter<site.warpnet.transport.dto.SearchUsersEvent>()
    private val editTweetAdapter = moshi.adapter<site.warpnet.transport.dto.EditTweetEvent>()
    private val getFollowReqsAdapter = moshi.adapter<site.warpnet.transport.dto.GetFollowRequestsEvent>()
    private val getFollowReqsRespAdapter = moshi.adapter<site.warpnet.transport.dto.GetFollowRequestsResponse>()
    private val followReqActionAdapter = moshi.adapter<site.warpnet.transport.dto.FollowRequestActionEvent>()
    private val filterAdapter = moshi.adapter<site.warpnet.transport.dto.WarpnetFilter>()
    private val getFilterAdapter = moshi.adapter<site.warpnet.transport.dto.GetFilterEvent>()
    private val getFiltersAdapter = moshi.adapter<site.warpnet.transport.dto.GetFiltersEvent>()
    private val getFiltersRespAdapter = moshi.adapter<site.warpnet.transport.dto.GetFiltersResponse>()
    private val deleteFilterAdapter = moshi.adapter<site.warpnet.transport.dto.DeleteFilterEvent>()
    private val addFilterKwAdapter = moshi.adapter<site.warpnet.transport.dto.AddFilterKeywordEvent>()
    private val updateFilterKwAdapter = moshi.adapter<site.warpnet.transport.dto.UpdateFilterKeywordEvent>()
    private val deleteFilterKwAdapter = moshi.adapter<site.warpnet.transport.dto.DeleteFilterKeywordEvent>()
    private val filterKeywordAdapter = moshi.adapter<site.warpnet.transport.dto.WarpnetFilterKeyword>()
    private val getFollowersAdapter = moshi.adapter<GetFollowersEvent>()
    private val getFollowingsAdapter = moshi.adapter<GetFollowingsEvent>()
    private val getIsFollowingAdapter = moshi.adapter<GetIsFollowingEvent>()
    private val newFollowAdapter = moshi.adapter<NewFollowEvent>()
    private val newUnfollowAdapter = moshi.adapter<NewUnfollowEvent>()
    private val likeEventAdapter = moshi.adapter<LikeEvent>()
    private val viewEventAdapter = moshi.adapter<ViewEvent>()
    private val viewsCountAdapter = moshi.adapter<ViewsCountResponse>()
    private val newTweetAdapter = moshi.adapter<WarpnetTweet>()
    private val deleteTweetAdapter = moshi.adapter<DeleteTweetEvent>()
    private val newChatAdapter = moshi.adapter<site.warpnet.transport.dto.NewChatEvent>()
    private val chatAdapter = moshi.adapter<site.warpnet.transport.dto.WarpnetChat>()
    private val getChatsAdapter = moshi.adapter<site.warpnet.transport.dto.GetChatsEvent>()
    private val getChatsRespAdapter = moshi.adapter<site.warpnet.transport.dto.GetChatsResponse>()
    private val deleteChatAdapter = moshi.adapter<site.warpnet.transport.dto.DeleteChatEvent>()
    private val getMessagesAdapter = moshi.adapter<site.warpnet.transport.dto.GetMessagesEvent>()
    private val getMessagesRespAdapter = moshi.adapter<site.warpnet.transport.dto.GetMessagesResponse>()
    private val newMessageAdapter = moshi.adapter<site.warpnet.transport.dto.WarpnetMessage>()
    private val newMessageEventAdapter = moshi.adapter<site.warpnet.transport.dto.NewMessageEvent>()
    private val deleteMessageAdapter = moshi.adapter<site.warpnet.transport.dto.DeleteMessageEvent>()
    private val unretweetAdapter = moshi.adapter<UnretweetEvent>()
    private val reportEventAdapter = moshi.adapter<site.warpnet.transport.dto.WarpnetReportEvent>()

    // -----------------------------------------------------------------
    // Users
    // -----------------------------------------------------------------

    suspend fun getAccount(userId: String): User =
        getUser(userId).toAccount()

    suspend fun getTimelineUser(userId: String): TimelineUser =
        getUser(userId).toTimelineUser()

    private suspend fun getUser(userId: String, nodeId: String = ""): WarpnetUser {
        // Fall back to the paired fat node as the resolution hint when the
        // caller doesn't supply one, so every getUser path — timeline and
        // quoted-tweet authors, ancestors, profile opens, follower lists —
        // can resolve a user that isn't cached locally, not just the lists
        // that thread a hint explicitly.
        val hint = nodeId.ifEmpty { pairedNodeStore.load()?.pinnedPeerId.orEmpty() }
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_USER,
            getUserAdapter.toJson(GetUserEvent(userId = userId, nodeId = hint)),
        )
        return userAdapter.fromJson(raw)
            ?: throw IllegalStateException("getUser returned empty body for $userId")
    }

    // -----------------------------------------------------------------
    // Timelines
    // -----------------------------------------------------------------

    /** Aggregated home feed. Second element is the next-page cursor, empty when exhausted. */
    suspend fun getHomeTimeline(cursor: String = "", limit: Int = 40): Pair<List<Tweet>, String> {
        // The fat-node timeline handler rejects an empty user id; scope the
        // request to the user we paired with so the server can build the
        // "home" feed for that identity.
        val userId = pairedNodeStore.load()?.userId
            ?: throw IllegalStateException("getHomeTimeline called before a node has been paired")
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_TIMELINE,
            getAllTweetsAdapter.toJson(GetAllTweetsEvent(userId = userId, cursor = cursor, limit = limit)),
        )
        val page = tweetsRespAdapter.fromJson(raw) ?: return emptyList<Tweet>() to ""
        return hydrateTweets(page.tweets) to page.cursor
    }

    /** Public per-user feed. Second element is the next-page cursor, empty when exhausted. */
    suspend fun getUserTimeline(userId: String, cursor: String = "", limit: Int = 40): Pair<List<Tweet>, String> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_TWEETS,
            getAllTweetsAdapter.toJson(GetAllTweetsEvent(userId = userId, cursor = cursor, limit = limit)),
        )
        val page = tweetsRespAdapter.fromJson(raw) ?: return emptyList<Tweet>() to ""
        return hydrateTweets(page.tweets) to page.cursor
    }

    suspend fun getStatus(tweetId: String, userId: String): Tweet {
        val tweet = fetchTweetRaw(tweetId, userId)
        val base = tweet.toTweet(author = runCatching { getUser(tweet.userId) }.getOrNull())
        val stats = runCatching { getTweetStats(tweetId = tweet.id, userId = userId) }.getOrNull()
        return if (stats == null) base else base.copy(
            likesCount = stats.likesCount.clampToInt(),
            retweetsCount = stats.retweetsCount.clampToInt(),
            repliesCount = stats.repliesCount.clampToInt(),
            viewsCount = stats.viewsCount.clampToInt(),
        )
    }

    suspend fun getTweetStats(tweetId: String, userId: String): TweetStatsResponse {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_TWEET_STATS,
            getTweetStatsAdapter.toJson(GetTweetStatsEvent(tweetId = tweetId, userId = userId)),
        )
        return tweetStatsRespAdapter.fromJson(raw) ?: TweetStatsResponse(tweetId = tweetId)
    }

    suspend fun getReplies(rootId: String, parentId: String = "", cursor: String = ""): List<Tweet> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_REPLIES,
            getRepliesAdapter.toJson(GetAllRepliesEvent(rootId = rootId, parentId = parentId, cursor = cursor)),
        )
        val page = repliesRespAdapter.fromJson(raw) ?: return emptyList()
        // Backend returns a ReplyNode tree ({reply, children}); flatten
        // depth-first into the flat List<WarpnetTweet> the UI expects.
        val flat = mutableListOf<site.warpnet.transport.dto.WarpnetTweet>()
        fun walk(nodes: List<site.warpnet.transport.dto.WarpnetReplyNode>) {
            for (n in nodes) {
                flat += n.reply
                if (n.children.isNotEmpty()) walk(n.children)
            }
        }
        walk(page.replies)
        return hydrateTweets(flat)
    }

    /**
     * Walk the parent chain upward from [tweetId], fetching each ancestor
     * via PUBLIC_GET_TWEET. Returns ancestors ordered root-first so the
     * thread view can render oldest-to-newest.
     *
     * [maxDepth] bounds the walk to defend against malformed cycles and
     * runaway threads; beyond it the chain is silently truncated.
     */
    suspend fun getAncestors(tweetId: String, userId: String, maxDepth: Int = 32): List<Tweet> {
        val chain = mutableListOf<Tweet>()
        var current = runCatching { fetchTweetRaw(tweetId, userId) }.getOrNull() ?: return emptyList()
        val cache = mutableMapOf<String, WarpnetUser>()
        var steps = 0
        while (!current.parentId.isNullOrEmpty() && steps < maxDepth) {
            val parent = runCatching { fetchTweetRaw(current.parentId!!, userId) }.getOrNull() ?: break
            chain += parent.toTweet(resolveUser(parent.userId, cache))
            current = parent
            steps++
        }
        return chain.asReversed()
    }

    private suspend fun fetchTweetRaw(tweetId: String, userId: String): WarpnetTweet {
        tweetCache.get(tweetId)?.let { return it }
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_TWEET,
            getTweetAdapter.toJson(GetTweetEvent(tweetId = tweetId, userId = userId)),
        )
        val tweet = tweetAdapter.fromJson(raw)
            ?: throw IllegalStateException("fetchTweetRaw returned empty body for $tweetId")
        if (isAgedForCache(tweet)) tweetCache.put(tweetId, tweet)
        return tweet
    }

    /**
     * Only cache tweets last modified at least [TWEET_CACHE_MIN_AGE_MILLIS]
     * ago. A recently created — or recently edited — tweet is still churning,
     * so we always re-fetch its body live and never serve a stale snapshot.
     * The gate keys on updated_at when present (falling back to created_at):
     * an edit bumps updated_at, so a tweet created long ago but edited
     * recently — including a remote edit we can't invalidate locally — stays
     * out of the cache until it settles. A tweet without a parseable
     * timestamp is treated as not cacheable. Only the tweet body is cached
     * here — engagement stats churn and stay live (see getStatus /
     * hydrateTweets, which fetch counts separately).
     */
    private fun isAgedForCache(tweet: WarpnetTweet): Boolean {
        val lastModifiedMillis = (tweet.updatedAt ?: tweet.createdAt)
            ?.let { runCatching { java.time.Instant.parse(it).toEpochMilli() }.getOrNull() }
            ?: return false
        return System.currentTimeMillis() - lastModifiedMillis >= TWEET_CACHE_MIN_AGE_MILLIS
    }

    /**
     * Seed [tweetCache] with the aged bodies on a freshly fetched page so a
     * later thread/ancestor open is a cache hit. Retweet wrappers are
     * skipped: they reuse the original tweet's id but carry retweetedBy, so
     * caching one would shadow the canonical body under the same key.
     */
    private fun seedTweetCache(tweets: List<WarpnetTweet>) {
        tweets.forEach { t ->
            if (t.retweetedBy.isNullOrEmpty() && isAgedForCache(t)) tweetCache.put(t.id, t)
        }
    }

    // -----------------------------------------------------------------
    // Posting
    // -----------------------------------------------------------------

    suspend fun postStatus(text: String, authorUserId: String, authorUsername: String, parentId: String? = null): Tweet {
        // createdAt is left null so the backend stamps the creation time
        // (database/tweet-repo.go:152). Emitting "" instead fails the
        // server-side time.Time decode before the zero-value fallback
        // runs, so the post would silently disappear.
        val draft = WarpnetTweet(
            id = "",
            rootId = parentId ?: "",
            text = text,
            userId = authorUserId,
            username = authorUsername,
            parentId = parentId,
        )
        val raw = client.request(ProtocolIds.PRIVATE_POST_TWEET, newTweetAdapter.toJson(draft))
        val created = tweetAdapter.fromJson(raw)
            ?: throw IllegalStateException("postStatus returned empty body")
        return created.toTweet(author = runCatching { getUser(created.userId) }.getOrNull())
    }

    suspend fun deleteStatus(tweetId: String, userId: String) {
        client.request(
            ProtocolIds.PRIVATE_DELETE_TWEET,
            deleteTweetAdapter.toJson(DeleteTweetEvent(tweetId = tweetId, userId = userId)),
        )
        tweetCache.invalidate(tweetId)
    }

    // -----------------------------------------------------------------
    // Likes
    // -----------------------------------------------------------------

    /**
     * [userId] is the **tweet author's** id (matches Warpnet's
     * `event.LikeEvent.user_id` semantics); [ownerId] is the **liker's**
     * id (the paired-node user). The backend handler rejects requests
     * where either is empty (core/handler/like.go:79-87), so callers
     * must supply both.
     */
    suspend fun likeStatus(tweetId: String, userId: String, ownerId: String): Long {
        val raw = client.request(
            ProtocolIds.PUBLIC_POST_LIKE,
            likeEventAdapter.toJson(LikeEvent(tweetId = tweetId, userId = userId, ownerId = ownerId)),
        )
        return likesCountAdapter.fromJson(raw)?.likesCount ?: 0
    }

    suspend fun unlikeStatus(tweetId: String, userId: String, ownerId: String): Long {
        val raw = client.request(
            ProtocolIds.PUBLIC_POST_UNLIKE,
            likeEventAdapter.toJson(LikeEvent(tweetId = tweetId, userId = userId, ownerId = ownerId)),
        )
        return likesCountAdapter.fromJson(raw)?.likesCount ?: 0
    }

    // -----------------------------------------------------------------
    // Views
    // -----------------------------------------------------------------

    /**
     * Record that the local pairing user just viewed a tweet authored
     * by [authorId]. The author's node is the sole authority for the
     * counter (CRDT replicates it everywhere else); self-views are
     * dropped server-side and the same (tweet, viewer) pair is deduped
     * within a 30-minute window.
     *
     * Returns the post-increment count, or `null` if the response body
     * couldn't be parsed (e.g. shape mismatch or empty body). Transport
     * failures propagate as exceptions so the caller — typically
     * [site.warpnet.warpdroid.network.WarpnetApi.recordView] — can
     * log them; do not call this directly from the UI without a guard.
     */
    suspend fun recordView(tweetId: String, authorId: String, viewerId: String): Long? {
        if (tweetId.isBlank() || authorId.isBlank() || viewerId.isBlank()) return null
        val raw = client.request(
            ProtocolIds.PUBLIC_POST_VIEW,
            viewEventAdapter.toJson(
                ViewEvent(tweetId = tweetId, userId = authorId, viewerId = viewerId),
            ),
        )
        return viewsCountAdapter.fromJson(raw)?.count
    }

    // -----------------------------------------------------------------
    // Reports
    // -----------------------------------------------------------------

    /**
     * Publish a moderation report to the global reports gossip topic.
     * Moderator nodes subscribed to that topic pick it up, fetch the
     * offending content directly from [targetNodeId], run the engine,
     * and (if the verdict is bad) publish a shadow-ban verdict on the
     * offender's followers topic. The offender's own node never sees
     * the verdict.
     *
     * [type] mirrors the fat node's ModerationObjectType enum. The
     * backend currently accepts only 0 (user profile) and 1 (tweet);
     * reply (2) and image (3) reports are validated out server-side.
     * [objectId] is required for tweet reports and is sent as "" for
     * user reports.
     * [reason] is a free-form string capped at 256 chars by the
     * backend. The Android UI presents a fixed set of labels as a
     * convenience but any short non-empty string is accepted.
     */
    suspend fun reportContent(
        type: Int,
        objectId: String,
        targetUserId: String,
        targetNodeId: String,
        reason: String,
    ) {
        client.request(
            ProtocolIds.PUBLIC_POST_REPORT,
            reportEventAdapter.toJson(
                site.warpnet.transport.dto.WarpnetReportEvent(
                    type = type,
                    objectId = objectId,
                    targetUserId = targetUserId,
                    targetNodeId = targetNodeId,
                    reason = reason,
                ),
            ),
        )
    }

    // -----------------------------------------------------------------
    // Retweets
    // -----------------------------------------------------------------

    // Warpnet's NewRetweetEvent is domain.Tweet, so the payload is the
    // retweeter's copy of the tweet rather than a (tweet_id, user_id) pair.
    //
    // [sourceAuthorId] is the user_id of the original tweet's author —
    // needed for cross-node propagation so the source-author's node
    // receives the retweet event. When non-null and [comment] is
    // non-blank, the retweet becomes a quote: a regular tweet
    // authored by the retweeter (text = comment) that references the
    // source via quoted_tweet_id / quoted_user_id.
    suspend fun retweetStatus(
        tweetId: String,
        retweeterId: String,
        retweeterUsername: String,
        sourceAuthorId: String? = null,
        comment: String? = null,
    ): Long {
        val isQuote = !comment.isNullOrBlank() && !sourceAuthorId.isNullOrBlank()
        val payload = if (isQuote) {
            WarpnetTweet(
                id = tweetId,
                rootId = "",
                text = comment!!.trim(),
                userId = retweeterId,
                username = retweeterUsername,
                retweetedBy = retweeterId,
                quotedTweetId = tweetId,
                quotedUserId = sourceAuthorId!!,
            )
        } else {
            WarpnetTweet(
                id = tweetId,
                rootId = "",
                text = "",
                userId = sourceAuthorId ?: retweeterId,
                username = retweeterUsername,
                retweetedBy = retweeterId,
            )
        }
        val raw = client.request(
            ProtocolIds.PUBLIC_POST_RETWEET,
            newTweetAdapter.toJson(payload),
        )
        return likesCountAdapter.fromJson(raw)?.likesCount ?: 0
    }

    suspend fun unretweetStatus(tweetId: String, retweeterId: String): Long {
        val raw = client.request(
            ProtocolIds.PUBLIC_POST_UNRETWEET,
            unretweetAdapter.toJson(UnretweetEvent(tweetId = tweetId, retweeterId = retweeterId)),
        )
        return likesCountAdapter.fromJson(raw)?.likesCount ?: 0
    }

    // -----------------------------------------------------------------
    // Follow graph
    // -----------------------------------------------------------------

    suspend fun followAccount(followerId: String, followeeId: String): Relationship {
        client.request(
            ProtocolIds.PUBLIC_POST_FOLLOW,
            newFollowAdapter.toJson(NewFollowEvent(followerId = followerId, followeeId = followeeId)),
        )
        return WarpnetMapper.relationshipFrom(targetUserId = followeeId, following = true, followedBy = false)
    }

    suspend fun unfollowAccount(followerId: String, followeeId: String): Relationship {
        client.request(
            ProtocolIds.PUBLIC_POST_UNFOLLOW,
            newUnfollowAdapter.toJson(NewUnfollowEvent(followerId = followerId, followeeId = followeeId)),
        )
        return WarpnetMapper.relationshipFrom(targetUserId = followeeId, following = false, followedBy = false)
    }

    // -----------------------------------------------------------------
    // Notifications
    // -----------------------------------------------------------------

    /**
     * Caller's own notifications feed. Second element is the next-page cursor,
     * empty when exhausted.
     *
     * The fat node resolves the recipient from the paired session, so the
     * wire DTO carries only cursor/limit; [userId] is accepted purely to keep
     * the existing call sites in [site.warpnet.warpdroid.network.WarpnetApi]
     * compiling and to surface the resolved-from-pairing identity to callers.
     */
    suspend fun getNotifications(userId: String, cursor: String = "", limit: Int = 40): Pair<List<Notification>, String> {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_NOTIFICATIONS,
            getNotifsAdapter.toJson(GetNotificationsEvent(cursor = cursor, limit = limit)),
        )
        val page = notificationsRespAdapter.fromJson(raw) ?: return emptyList<Notification>() to ""
        return page.notifications.map { it.toNotification() } to page.cursor
    }

    suspend fun getNewNotifications(cursor: String = "", limit: Int = 40): Pair<List<Notification>, String> {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_NEW_NOTIFICATIONS,
            getNotifsAdapter.toJson(GetNotificationsEvent(cursor = cursor, limit = limit)),
        )
        val page = notificationsRespAdapter.fromJson(raw) ?: return emptyList<Notification>() to ""
        return page.notifications.map { it.toNotification() } to page.cursor
    }

    /**
     * Fetch a single notification by id. The fat node resolves the recipient
     * from the paired session; only the notification id travels on the wire.
     */
    suspend fun getNotification(notificationId: String): Notification? {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_NOTIFICATION,
            getNotifAdapter.toJson(site.warpnet.transport.dto.GetNotificationEvent(notificationId = notificationId)),
        )
        val wire = notificationRespAdapter.fromJson(raw) ?: return null
        return wire.toNotification()
    }

    /** Mark a single notification as read on the fat node. */
    suspend fun markNotificationRead(notificationId: String) {
        client.request(
            ProtocolIds.PRIVATE_POST_NOTIFICATION_READ,
            markNotifReadAdapter.toJson(
                site.warpnet.transport.dto.MarkNotificationReadEvent(notificationId = notificationId),
            ),
        )
    }

    // -----------------------------------------------------------------
    // Filters (local keyword/regex hiding rules)
    // -----------------------------------------------------------------

    suspend fun getFilter(userId: String, filterId: String): site.warpnet.transport.dto.WarpnetFilter? {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_FILTER,
            getFilterAdapter.toJson(
                site.warpnet.transport.dto.GetFilterEvent(userId = userId, filterId = filterId),
            ),
        )
        return filterAdapter.fromJson(raw)
    }

    suspend fun getFilters(userId: String, cursor: String = "", limit: Int = 40): Pair<List<site.warpnet.transport.dto.WarpnetFilter>, String> {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_FILTERS,
            getFiltersAdapter.toJson(
                site.warpnet.transport.dto.GetFiltersEvent(userId = userId, cursor = cursor, limit = limit),
            ),
        )
        val page = getFiltersRespAdapter.fromJson(raw)
            ?: return emptyList<site.warpnet.transport.dto.WarpnetFilter>() to ""
        return page.filters to page.cursor
    }

    suspend fun createFilter(filter: site.warpnet.transport.dto.WarpnetFilter): site.warpnet.transport.dto.WarpnetFilter {
        val raw = client.request(
            ProtocolIds.PRIVATE_POST_FILTER,
            filterAdapter.toJson(filter),
        )
        return filterAdapter.fromJson(raw)
            ?: throw IllegalStateException("createFilter returned empty body")
    }

    suspend fun updateFilter(filter: site.warpnet.transport.dto.WarpnetFilter): site.warpnet.transport.dto.WarpnetFilter {
        val raw = client.request(
            ProtocolIds.PRIVATE_POST_FILTER_UPDATE,
            filterAdapter.toJson(filter),
        )
        return filterAdapter.fromJson(raw)
            ?: throw IllegalStateException("updateFilter returned empty body")
    }

    suspend fun deleteFilter(userId: String, filterId: String) {
        client.request(
            ProtocolIds.PRIVATE_DELETE_FILTER,
            deleteFilterAdapter.toJson(
                site.warpnet.transport.dto.DeleteFilterEvent(userId = userId, filterId = filterId),
            ),
        )
    }

    suspend fun addFilterKeyword(userId: String, filterId: String, keyword: String, wholeWord: Boolean): site.warpnet.transport.dto.WarpnetFilterKeyword {
        val raw = client.request(
            ProtocolIds.PRIVATE_POST_FILTER_KEYWORD,
            addFilterKwAdapter.toJson(
                site.warpnet.transport.dto.AddFilterKeywordEvent(
                    userId = userId, filterId = filterId, keyword = keyword, wholeWord = wholeWord,
                ),
            ),
        )
        return filterKeywordAdapter.fromJson(raw)
            ?: throw IllegalStateException("addFilterKeyword returned empty body")
    }

    suspend fun updateFilterKeyword(userId: String, keywordId: String, keyword: String, wholeWord: Boolean): site.warpnet.transport.dto.WarpnetFilterKeyword {
        val raw = client.request(
            ProtocolIds.PRIVATE_POST_FILTER_KEYWORD_UPDATE,
            updateFilterKwAdapter.toJson(
                site.warpnet.transport.dto.UpdateFilterKeywordEvent(
                    userId = userId, keywordId = keywordId, keyword = keyword, wholeWord = wholeWord,
                ),
            ),
        )
        return filterKeywordAdapter.fromJson(raw)
            ?: throw IllegalStateException("updateFilterKeyword returned empty body")
    }

    suspend fun deleteFilterKeyword(userId: String, keywordId: String) {
        client.request(
            ProtocolIds.PRIVATE_DELETE_FILTER_KEYWORD,
            deleteFilterKwAdapter.toJson(
                site.warpnet.transport.dto.DeleteFilterKeywordEvent(userId = userId, keywordId = keywordId),
            ),
        )
    }

    // -----------------------------------------------------------------
    // Follow requests (pending follows on locked accounts)
    // -----------------------------------------------------------------

    suspend fun getFollowRequests(userId: String, cursor: String = "", limit: Int = 40): Pair<List<TimelineUser>, String> {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_FOLLOW_REQUESTS,
            getFollowReqsAdapter.toJson(
                site.warpnet.transport.dto.GetFollowRequestsEvent(userId = userId, cursor = cursor, limit = limit),
            ),
        )
        val page = getFollowReqsRespAdapter.fromJson(raw) ?: return emptyList<TimelineUser>() to ""
        return hydrateAccounts(page.followerIds) to page.cursor
    }

    suspend fun authorizeFollowRequest(userId: String, followerId: String) {
        client.request(
            ProtocolIds.PRIVATE_POST_FOLLOW_REQUEST_AUTHORIZE,
            followReqActionAdapter.toJson(
                site.warpnet.transport.dto.FollowRequestActionEvent(userId = userId, followerId = followerId),
            ),
        )
    }

    suspend fun rejectFollowRequest(userId: String, followerId: String) {
        client.request(
            ProtocolIds.PRIVATE_POST_FOLLOW_REQUEST_REJECT,
            followReqActionAdapter.toJson(
                site.warpnet.transport.dto.FollowRequestActionEvent(userId = userId, followerId = followerId),
            ),
        )
    }

    // -----------------------------------------------------------------
    // Tweet edits — mutate Tweet.text in place + append an immutable
    // revision to the per-tweet edit history.
    // -----------------------------------------------------------------

    suspend fun editTweet(tweetId: String, userId: String, text: String): WarpnetTweet {
        val raw = client.request(
            ProtocolIds.PRIVATE_POST_TWEET_EDIT,
            editTweetAdapter.toJson(
                site.warpnet.transport.dto.EditTweetEvent(
                    tweetId = tweetId,
                    userId = userId,
                    text = text,
                ),
            ),
        )
        val edited = tweetAdapter.fromJson(raw)
            ?: throw IllegalStateException("editTweet returned empty body for $tweetId")
        // The edit rewrote the body, so drop any cached snapshot of the old
        // text. isAgedForCache keys on updated_at, so the just-edited tweet
        // is "recently modified" and stays uncached until it settles for an
        // hour; the guarded put is a no-op now but keeps the invariant local.
        tweetCache.invalidate(tweetId)
        if (isAgedForCache(edited)) tweetCache.put(tweetId, edited)
        return edited
    }

    // -----------------------------------------------------------------
    // Server-side user search (replaces the previous client-side
    // listUsers + filter dance — the fat node runs the substring scan).
    // -----------------------------------------------------------------

    suspend fun searchAccounts(query: String, cursor: String = "", limit: Int = 40): Pair<List<TimelineUser>, String> {
        if (query.isBlank()) return emptyList<TimelineUser>() to ""
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_USERS_SEARCH,
            searchUsersAdapter.toJson(
                site.warpnet.transport.dto.SearchUsersEvent(query = query, cursor = cursor, limit = limit),
            ),
        )
        val page = usersRespAdapter.fromJson(raw) ?: return emptyList<TimelineUser>() to ""
        return page.users.map { it.toTimelineUser() } to page.cursor
    }

    // -----------------------------------------------------------------
    // Media metadata (alt-text + focal point on uploaded images)
    // -----------------------------------------------------------------

    suspend fun updateMediaMeta(
        userId: String,
        key: String,
        description: String,
        focusX: Float,
        focusY: Float,
    ) {
        client.request(
            ProtocolIds.PRIVATE_POST_MEDIA_META,
            updateMediaMetaAdapter.toJson(
                site.warpnet.transport.dto.UpdateMediaMetaEvent(
                    userId = userId,
                    key = key,
                    description = description,
                    focusX = focusX,
                    focusY = focusY,
                ),
            ),
        )
    }

    suspend fun getMediaMeta(userId: String, key: String): site.warpnet.transport.dto.GetMediaResponse {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_MEDIA,
            getMediaAdapter.toJson(site.warpnet.transport.dto.GetMediaEvent(userId = userId, key = key)),
        )
        return getMediaRespAdapter.fromJson(raw)
            ?: throw IllegalStateException("getMediaMeta returned empty body for $key")
    }

    // -----------------------------------------------------------------
    // Subscribe / Unsubscribe to a user's posts (local watchlist)
    // -----------------------------------------------------------------

    suspend fun subscribeUser(selfId: String, targetId: String) {
        client.request(
            ProtocolIds.PRIVATE_POST_SUBSCRIBE_USER,
            subscribeUserAdapter.toJson(
                site.warpnet.transport.dto.SubscribeUserEvent(selfId = selfId, targetId = targetId),
            ),
        )
    }

    suspend fun unsubscribeUser(selfId: String, targetId: String) {
        client.request(
            ProtocolIds.PRIVATE_POST_UNSUBSCRIBE_USER,
            subscribeUserAdapter.toJson(
                site.warpnet.transport.dto.SubscribeUserEvent(selfId = selfId, targetId = targetId),
            ),
        )
    }

    // -----------------------------------------------------------------
    // Engagement lists (who liked / retweeted a tweet)
    // -----------------------------------------------------------------

    suspend fun getTweetLikers(tweetId: String, ownerUserId: String, cursor: String = "", limit: Int = 40): Pair<List<TimelineUser>, String> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_TWEET_LIKERS,
            getTweetLikersAdapter.toJson(
                site.warpnet.transport.dto.GetTweetLikersEvent(
                    tweetId = tweetId,
                    ownerUserId = ownerUserId,
                    cursor = cursor,
                    limit = limit,
                ),
            ),
        )
        val page = usersRespAdapter.fromJson(raw) ?: return emptyList<TimelineUser>() to ""
        return page.users.map { it.toTimelineUser() } to page.cursor
    }

    suspend fun getTweetRetweeters(tweetId: String, ownerUserId: String, cursor: String = "", limit: Int = 40): Pair<List<TimelineUser>, String> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_TWEET_RETWEETERS,
            getTweetLikersAdapter.toJson(
                site.warpnet.transport.dto.GetTweetLikersEvent(
                    tweetId = tweetId,
                    ownerUserId = ownerUserId,
                    cursor = cursor,
                    limit = limit,
                ),
            ),
        )
        val page = usersRespAdapter.fromJson(raw) ?: return emptyList<TimelineUser>() to ""
        return page.users.map { it.toTimelineUser() } to page.cursor
    }

    // -----------------------------------------------------------------
    // Block / Mute / Conversation-mute (local social-graph filters)
    //
    // The fat node also escalates a social block into a peer-level
    // libp2p blocklist for the target's NodeId — see core/handler/block.go.
    // -----------------------------------------------------------------

    suspend fun blockUser(blockerId: String, blockeeId: String) {
        client.request(
            ProtocolIds.PRIVATE_POST_BLOCK,
            blockEventAdapter.toJson(
                site.warpnet.transport.dto.BlockEvent(blockerId = blockerId, blockeeId = blockeeId),
            ),
        )
    }

    suspend fun unblockUser(blockerId: String, blockeeId: String) {
        client.request(
            ProtocolIds.PRIVATE_POST_UNBLOCK,
            blockEventAdapter.toJson(
                site.warpnet.transport.dto.BlockEvent(blockerId = blockerId, blockeeId = blockeeId),
            ),
        )
    }

    suspend fun getBlocks(userId: String, cursor: String = "", limit: Int = 40): Pair<List<String>, String> {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_BLOCKS,
            getBlocksEventAdapter.toJson(
                site.warpnet.transport.dto.GetBlocksEvent(userId = userId, cursor = cursor, limit = limit),
            ),
        )
        val page = getBlocksRespAdapter.fromJson(raw) ?: return emptyList<String>() to ""
        return page.ids to page.cursor
    }

    suspend fun muteUser(muterId: String, muteeId: String) {
        client.request(
            ProtocolIds.PRIVATE_POST_MUTE,
            muteEventAdapter.toJson(
                site.warpnet.transport.dto.MuteEvent(muterId = muterId, muteeId = muteeId),
            ),
        )
    }

    suspend fun unmuteUser(muterId: String, muteeId: String) {
        client.request(
            ProtocolIds.PRIVATE_POST_UNMUTE,
            muteEventAdapter.toJson(
                site.warpnet.transport.dto.MuteEvent(muterId = muterId, muteeId = muteeId),
            ),
        )
    }

    suspend fun getMutes(userId: String, cursor: String = "", limit: Int = 40): Pair<List<String>, String> {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_MUTES,
            getBlocksEventAdapter.toJson(
                site.warpnet.transport.dto.GetBlocksEvent(userId = userId, cursor = cursor, limit = limit),
            ),
        )
        val page = getBlocksRespAdapter.fromJson(raw) ?: return emptyList<String>() to ""
        return page.ids to page.cursor
    }


    // -----------------------------------------------------------------
    // Pin / Unpin (author-only; the fat node enforces the author check)
    // -----------------------------------------------------------------

    suspend fun pinTweet(userId: String, tweetId: String) {
        client.request(
            ProtocolIds.PUBLIC_POST_PIN,
            pinTweetAdapter.toJson(
                site.warpnet.transport.dto.PinTweetEvent(userId = userId, tweetId = tweetId),
            ),
        )
        tweetCache.invalidate(tweetId)
    }

    suspend fun unpinTweet(userId: String, tweetId: String) {
        client.request(
            ProtocolIds.PUBLIC_POST_UNPIN,
            pinTweetAdapter.toJson(
                site.warpnet.transport.dto.PinTweetEvent(userId = userId, tweetId = tweetId),
            ),
        )
        tweetCache.invalidate(tweetId)
    }

    // -----------------------------------------------------------------
    // Bookmarks (local-only shelf, no propagation)
    // -----------------------------------------------------------------

    /** Pin [tweetId] (authored by [ownerUserId]) to the local bookmark shelf. */
    suspend fun bookmarkTweet(userId: String, tweetId: String, ownerUserId: String) {
        if (userId.isBlank() || tweetId.isBlank() || ownerUserId.isBlank()) return
        runCatching {
            client.request(
                ProtocolIds.PRIVATE_POST_BOOKMARK,
                bookmarkEventAdapter.toJson(
                    site.warpnet.transport.dto.BookmarkEvent(
                        userId = userId,
                        tweetId = tweetId,
                        ownerUserId = ownerUserId,
                    ),
                ),
            )
        }.onFailure { e -> android.util.Log.w(TAG, "bookmarkTweet($tweetId) failed", e) }
    }

    /** Remove a previously-bookmarked tweet from the shelf. */
    suspend fun unbookmarkTweet(userId: String, tweetId: String) {
        if (userId.isBlank() || tweetId.isBlank()) return
        runCatching {
            client.request(
                ProtocolIds.PRIVATE_POST_UNBOOKMARK,
                unbookmarkEventAdapter.toJson(
                    site.warpnet.transport.dto.UnbookmarkEvent(userId = userId, tweetId = tweetId),
                ),
            )
        }.onFailure { e -> android.util.Log.w(TAG, "unbookmarkTweet($tweetId) failed", e) }
    }

    /**
     * Fetch one page of bookmarked tweets. The wire returns identifiers only —
     * each tweet body is re-fetched in parallel and surfaced as a Tweet.
     *
     * Wrapped in runCatching so a single backend / decode failure surfaces as
     * an empty page rather than crashing the whole timeline-viewing activity —
     * the bookmark route hits PRIVATE_GET_BOOKMARKS, which depends on local
     * state that may not exist pre-pairing or after a fresh install.
     */
    suspend fun getBookmarks(userId: String, cursor: String = "", limit: Int = 40): Pair<List<site.warpnet.warpdroid.entity.Tweet>, String> {
        if (userId.isBlank()) {
            return emptyList<site.warpnet.warpdroid.entity.Tweet>() to ""
        }
        return runCatching {
            val raw = client.request(
                ProtocolIds.PRIVATE_GET_BOOKMARKS,
                getBookmarksEventAdapter.toJson(
                    site.warpnet.transport.dto.GetBookmarksEvent(userId = userId, cursor = cursor, limit = limit),
                ),
            )
            val page = getBookmarksRespAdapter.fromJson(raw)
                ?: return@runCatching emptyList<site.warpnet.warpdroid.entity.Tweet>() to ""
            if (page.items.isEmpty()) {
                return@runCatching emptyList<site.warpnet.warpdroid.entity.Tweet>() to page.cursor
            }
            val tweets = page.items.mapNotNull { bm ->
                runCatching { getStatus(tweetId = bm.tweetId, userId = bm.ownerUserId) }.getOrNull()
            }
            tweets to page.cursor
        }.getOrElse { e ->
            android.util.Log.w(TAG, "getBookmarks($userId) failed", e)
            emptyList<site.warpnet.warpdroid.entity.Tweet>() to ""
        }
    }

    // -----------------------------------------------------------------
    // Chats (1:1 DMs on the fat node)
    // -----------------------------------------------------------------

    // Create (or fetch, if it already exists) the 1:1 chat with [otherUserId].
    // The fat node composes the deterministic chat id and returns the chat, so
    // a fresh conversation can be opened before any message is sent.
    suspend fun createChat(ownerId: String, otherUserId: String): site.warpnet.transport.dto.WarpnetChat? {
        val raw = client.request(
            ProtocolIds.PUBLIC_POST_CHAT,
            newChatAdapter.toJson(
                site.warpnet.transport.dto.NewChatEvent(ownerId = ownerId, otherUserId = otherUserId),
            ),
        )
        return chatAdapter.fromJson(raw)
    }

    suspend fun getChats(userId: String, cursor: String = "", limit: Int = 40): Pair<List<site.warpnet.transport.dto.WarpnetChat>, String> {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_CHATS,
            getChatsAdapter.toJson(
                site.warpnet.transport.dto.GetChatsEvent(userId = userId, cursor = cursor, limit = limit),
            ),
        )
        val page = getChatsRespAdapter.fromJson(raw)
            ?: return emptyList<site.warpnet.transport.dto.WarpnetChat>() to ""
        return page.chats to page.cursor
    }

    suspend fun deleteChat(userId: String, chatId: String) {
        client.request(
            ProtocolIds.PRIVATE_DELETE_CHAT,
            deleteChatAdapter.toJson(
                site.warpnet.transport.dto.DeleteChatEvent(userId = userId, chatId = chatId),
            ),
        )
    }

    suspend fun getMessages(
        ownerId: String,
        chatId: String,
        cursor: String = "",
        limit: Int = 40,
    ): Pair<List<site.warpnet.transport.dto.WarpnetMessage>, String> {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_MESSAGES,
            getMessagesAdapter.toJson(
                site.warpnet.transport.dto.GetMessagesEvent(
                    ownerId = ownerId,
                    chatId = chatId,
                    cursor = cursor,
                    limit = limit,
                ),
            ),
        )
        val page = getMessagesRespAdapter.fromJson(raw)
            ?: return emptyList<site.warpnet.transport.dto.WarpnetMessage>() to ""
        return page.messages to page.cursor
    }

    suspend fun sendMessage(
        chatId: String,
        senderId: String,
        receiverId: String,
        text: String,
    ): site.warpnet.transport.dto.WarpnetMessage? {
        val raw = client.request(
            ProtocolIds.PUBLIC_POST_MESSAGE,
            newMessageEventAdapter.toJson(
                site.warpnet.transport.dto.NewMessageEvent(
                    chatId = chatId,
                    senderId = senderId,
                    receiverId = receiverId,
                    text = text,
                ),
            ),
        )
        return newMessageAdapter.fromJson(raw)
    }

    suspend fun deleteMessage(chatId: String, messageId: String) {
        client.request(
            ProtocolIds.PRIVATE_DELETE_MESSAGE,
            deleteMessageAdapter.toJson(
                site.warpnet.transport.dto.DeleteMessageEvent(chatId = chatId, id = messageId),
            ),
        )
    }

    /**
     * Fetch a Warpnet-stored image blob by (owning user, blob key). The fat
     * node returns the file as `"<mime>,<base64>"` because the Vue UI inlines
     * it straight into a data URL — warpdroid splits the prefix off and
     * base64-decodes the body so [WarpnetAvatarLoader] can hand the raw
     * bytes to Glide. Returns null on empty key or decode failure; the
     * Glide pipeline falls back to the placeholder drawable.
     */
    suspend fun getImageBytes(userId: String, key: String): ByteArray? {
        if (userId.isBlank() || key.isBlank()) return null
        val cacheKey = "$userId/$key"
        imageCache.get(cacheKey)?.let { return it }
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_IMAGE,
            getImageEventAdapter.toJson(
                site.warpnet.transport.dto.GetImageEvent(userId = userId, key = key),
            ),
        )
        val file = getImageRespAdapter.fromJson(raw)?.file.orEmpty()
        if (file.isEmpty()) return null
        // "<mime>,<base64>" — drop the prefix; if no comma is present the
        // whole payload is treated as base64 to stay forward-compatible.
        val comma = file.indexOf(',')
        val b64 = if (comma >= 0) file.substring(comma + 1) else file
        val bytes = runCatching { android.util.Base64.decode(b64, android.util.Base64.DEFAULT) }.getOrNull()
        if (bytes != null && bytes.isNotEmpty()) imageCache.put(cacheKey, bytes)
        return bytes
    }

    // -----------------------------------------------------------------
    // Followers / followings
    // -----------------------------------------------------------------

    /**
     * Fetches a page of accounts that follow [userId]. Warpnet returns only
     * peer IDs, so each is resolved to a full [WarpnetUser] via [resolveUser].
     * Unresolvable IDs (dropped peers, deleted accounts) are skipped rather
     * than surfaced as stub rows.
     */
    suspend fun getFollowers(userId: String, cursor: String = "", limit: Int = 40): Pair<List<TimelineUser>, String> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_FOLLOWERS,
            getFollowersAdapter.toJson(GetFollowersEvent(userId = userId, cursor = cursor, limit = limit)),
        )
        val page = followersRespAdapter.fromJson(raw) ?: return emptyList<TimelineUser>() to ""
        return hydrateAccounts(page.followers) to page.cursor
    }

    suspend fun getFollowings(userId: String, cursor: String = "", limit: Int = 40): Pair<List<TimelineUser>, String> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_FOLLOWINGS,
            getFollowingsAdapter.toJson(GetFollowingsEvent(userId = userId, cursor = cursor, limit = limit)),
        )
        val page = followingsRespAdapter.fromJson(raw) ?: return emptyList<TimelineUser>() to ""
        return hydrateAccounts(page.followings) to page.cursor
    }

    /**
     * Probe the two directional follow relations for [targetUserId] from the
     * caller's perspective. Synthesises a Warpnet [Relationship] because
     * Warpnet has no concept of blocking / muting / pending requests.
     * Probe failures degrade to `false` so the UI can still render.
     */
    suspend fun relationshipFor(targetUserId: String): Relationship {
        val payload = getIsFollowingAdapter.toJson(GetIsFollowingEvent(userId = targetUserId))
        val following = runCatching {
            val raw = client.request(ProtocolIds.PUBLIC_POST_IS_FOLLOWING, payload)
            isFollowingRespAdapter.fromJson(raw)?.isFollowing ?: false
        }.getOrElse { false }
        val followedBy = runCatching {
            val raw = client.request(ProtocolIds.PUBLIC_POST_IS_FOLLOWER, payload)
            isFollowerRespAdapter.fromJson(raw)?.isFollower ?: false
        }.getOrElse { false }
        return WarpnetMapper.relationshipFrom(targetUserId, following, followedBy)
    }

    // -----------------------------------------------------------------
    // Search
    // -----------------------------------------------------------------

    /**
     * Enumerate users visible to [requesterUserId]. Warpnet has no server-side
     * text search, so callers filter client-side after receiving the page.
     * The `user_id` field is documented as "default owner" — we forward the
     * caller's id so the server can scope visibility if it chooses to.
     */
    suspend fun listUsers(requesterUserId: String, cursor: String = "", limit: Int = 40): Pair<List<TimelineUser>, String> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_USERS,
            getAllUsersAdapter.toJson(GetAllUsersEvent(userId = requesterUserId, cursor = cursor, limit = limit)),
        )
        val page = usersRespAdapter.fromJson(raw) ?: return emptyList<TimelineUser>() to ""
        return page.users.map { it.toTimelineUser() } to page.cursor
    }

    // Account recommendations ("who to follow"). The fat node already drops the
    // owner, offline nodes and already-followed users, so the carousel renders
    // the page as-is. Limit mirrors the desktop front-end (20).
    suspend fun whoToFollow(userId: String, cursor: String = "", limit: Int = 20): Pair<List<TimelineUser>, String> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_WHOTOFOLLOW,
            getAllUsersAdapter.toJson(GetAllUsersEvent(userId = userId, cursor = cursor, limit = limit)),
        )
        val page = usersRespAdapter.fromJson(raw) ?: return emptyList<TimelineUser>() to ""
        return page.users.map { it.toTimelineUser() } to page.cursor
    }

    // -----------------------------------------------------------------
    // Internals
    // -----------------------------------------------------------------

    private suspend fun hydrateAccounts(userIds: List<String>): List<TimelineUser> {
        if (userIds.isEmpty()) return emptyList()
        val cache = mutableMapOf<String, WarpnetUser>()
        return userIds.mapNotNull { id -> resolveUser(id, cache)?.toTimelineUser() }
    }

    private suspend fun hydrateTweets(tweets: List<WarpnetTweet>): List<Tweet> = coroutineScope {
        if (tweets.isEmpty()) return@coroutineScope emptyList()
        // Warm the body cache with the aged tweets on this page (timeline,
        // replies) so opening one as a thread/ancestor later is a cache hit;
        // stats are still fetched live below.
        seedTweetCache(tweets)
        val cache = mutableMapOf<String, WarpnetUser>()
        // Stats are fetched per tweet in parallel so a 30-tweet timeline
        // doesn't pay 30x serialised round-trip latency. Failures degrade
        // to zero counts; the toTweet baseline already matches that.
        val viewerId = pairedNodeStore.load()?.userId.orEmpty()
        // Pre-seed the resolveUser cache with the viewer's own
        // [AccountEntity] so own-user tweets surface with the avatar the
        // user already set on their profile, instead of falling back to
        // the empty-avatar stub if the per-tweet getUser() lookup races
        // the post-pairing profile refresh or returns a stripped wire
        // shape. The TimelineUser fields mirror what
        // WarpnetMapper.toTimelineUser would produce.
        if (viewerId.isNotBlank()) {
            seedOwnUser(viewerId, cache)
        }
        // Quoted-source tweets fan out in parallel too. The map keys by
        // (quotedTweetId, quotedUserId) so two quotes of the same source
        // collapse to one RPC. Inner getStatus() doesn't recurse into
        // hydrateTweets, so the nested Tweet comes back with quote=null
        // and we don't pay an unbounded fetch tree.
        val quoteJobs = tweets
            .mapNotNull { t ->
                val qId = t.quotedTweetId.orEmpty()
                val qUser = t.quotedUserId.orEmpty()
                if (qId.isBlank() || qUser.isBlank()) null else (qId to qUser)
            }
            .distinct()
            .associateWith { (qId, qUser) ->
                async { runCatching { getStatus(tweetId = qId, userId = qUser) }.getOrNull() }
            }

        val baseTweets: suspend (WarpnetTweet) -> Tweet = { t ->
            val base = t.toTweet(resolveUser(t.userId, cache))
            attachQuote(t, base, quoteJobs)
        }

        if (viewerId.isBlank()) {
            return@coroutineScope tweets.map { baseTweets(it) }
        }
        // Retweets reuse the original tweet id, so distinct() avoids
        // firing the same stats RPC twice on a single timeline page.
        val stats = tweets.map { it.id }.distinct().associateWith { id ->
            async { runCatching { getTweetStats(tweetId = id, userId = viewerId) }.getOrNull() }
        }
        tweets.map { t ->
            val withQuote = baseTweets(t)
            val s = stats[t.id]?.await() ?: return@map withQuote
            withQuote.copy(
                likesCount = s.likesCount.clampToInt(),
                retweetsCount = s.retweetsCount.clampToInt(),
                repliesCount = s.repliesCount.clampToInt(),
                viewsCount = s.viewsCount.clampToInt(),
            )
        }
    }

    /**
     * Attach the quoted-source tweet to the rendered [Tweet] when the
     * wire row has a quoted_tweet_id. State mirrors what the Vue
     * frontend produces from the same pair of timestamps:
     *
     *  - source fetch failed → DELETED (the row stays visible as a
     *    placeholder card so the user knows the quote pointed at
     *    something that's gone).
     *  - source.editedAt is after the quote's createdAt → REVOKED. The
     *    quoter snapshotted a particular wording; if the author has
     *    since rewritten it, the quote no longer represents the live
     *    text and we render the "unavailable" placeholder.
     *  - otherwise → ACCEPTED.
     */
    private suspend fun attachQuote(
        wire: WarpnetTweet,
        base: Tweet,
        sources: Map<Pair<String, String>, kotlinx.coroutines.Deferred<Tweet?>>,
    ): Tweet {
        val key = (wire.quotedTweetId.orEmpty() to wire.quotedUserId.orEmpty())
        if (key.first.isBlank() || key.second.isBlank()) return base
        val source = sources[key]?.await()
        val state = when {
            source == null -> site.warpnet.warpdroid.entity.Quote.State.DELETED
            source.editedAt != null && source.editedAt.after(base.createdAt) ->
                site.warpnet.warpdroid.entity.Quote.State.REVOKED
            else -> site.warpnet.warpdroid.entity.Quote.State.ACCEPTED
        }
        return base.copy(
            quote = site.warpnet.warpdroid.entity.Quote(
                state = state,
                quotedStatus = source,
            ),
        )
    }

    private fun Long.clampToInt(): Int = coerceIn(0, Int.MAX_VALUE.toLong()).toInt()

    private suspend fun resolveUser(
        userId: String,
        cache: MutableMap<String, WarpnetUser>,
    ): WarpnetUser? {
        if (userId.isBlank()) return null
        cache[userId]?.let { return it }
        // Reuse profiles across pages and screens via the shared cache so a
        // follower/author lookup isn't a fresh relay round-trip every time.
        // Tweet stats are NOT cached here — they're fetched separately and
        // churn.
        return userCache.getOrLoad(userId) {
            runCatching { getUser(userId) }.getOrNull()
        }?.also { cache[userId] = it }
    }

    /**
     * Pre-seed [cache] with a synthesised [WarpnetUser] for the viewer
     * built from the local [AccountEntity]. Used so own-tweet avatars
     * render even when getUser() is mid-flight or stripped of
     * avatar_key — the AccountEntity stores the already-resolved
     * `warpnet://avatar/{userId}/{key}` URL so we just decompose it.
     */
    private fun seedOwnUser(viewerId: String, cache: MutableMap<String, WarpnetUser>) {
        if (cache.containsKey(viewerId)) return
        val account = accountManager.activeAccount ?: return
        // accountId is the wire-level Warpnet user id (set after pairing);
        // skip seeding when it doesn't match the viewer (eg. stub state
        // before pairing populated the entity).
        if (account.accountId != viewerId) return
        val avatarKey = avatarKeyFromUrl(account.profilePictureUrl, viewerId)
        val backgroundKey = avatarKeyFromUrl(account.profileHeaderUrl, viewerId)
        cache[viewerId] = WarpnetUser(
            id = viewerId,
            username = account.displayName.ifBlank { account.username },
            avatarKey = avatarKey,
            backgroundImageKey = backgroundKey.orEmpty(),
        )
    }

    /**
     * Extract the blob key from a `warpnet://avatar/{userId}/{key}` URL
     * produced by [site.warpnet.warpdroid.warpnet.WarpnetMapper.warpnetImageUrl].
     * Returns null for blank URLs or shapes we don't recognise so the
     * seeded [WarpnetUser] surfaces as "no avatar" rather than garbage.
     */
    private fun avatarKeyFromUrl(url: String, viewerId: String): String? {
        if (url.isBlank()) return null
        val prefix = "warpnet://avatar/$viewerId/"
        if (!url.startsWith(prefix)) return null
        val tail = url.removePrefix(prefix)
        return tail.takeIf { it.isNotBlank() }
    }

    // Shared profile cache (see resolveUser). Profiles are slow-changing, so
    // a 5-minute TTL trades a little staleness for a large drop in relay
    // round-trips; the 256-entry LRU bound caps memory on low-end devices.
    private val userCache = TtlLruCache<String, WarpnetUser>(
        maxSize = 256,
        ttlMillis = 5L * 60L * 1000L,
    )

    // Tweet-body cache. Once a tweet is older than an hour its text and
    // timestamps are effectively immutable, so we park the body to skip the
    // PUBLIC_GET_TWEET round-trip on re-opens (threads, ancestors, quote
    // sources). Invalidation strategy:
    //  - editTweet / deleteStatus / pinTweet / unpinTweet drop (and edit
    //    re-seeds) the entry, so a local mutation never serves a stale
    //    snapshot;
    //  - the TTL is a backstop for changes we don't observe locally (e.g. an
    //    edit made from another device);
    //  - the LRU bound caps memory on low-end devices.
    // Stats are never cached here — see isAgedForCache.
    private val tweetCache = TtlLruCache<String, WarpnetTweet>(
        maxSize = 512,
        ttlMillis = TWEET_CACHE_TTL_MILLIS,
    )

    // Image-blob cache for getImageBytes. Keys are SHA-256 content hashes,
    // so (userId, key) -> bytes is immutable: no TTL or invalidation needed.
    // Bounded by total bytes, not entry count, since image sizes vary widely.
    private val imageCache = object : LruCache<String, ByteArray>(IMAGE_CACHE_MAX_BYTES) {
        override fun sizeOf(key: String, value: ByteArray): Int = value.size
    }

    private companion object {
        const val TAG = "WarpnetRepository"
        // Minimum tweet age before its body is eligible for caching.
        const val TWEET_CACHE_MIN_AGE_MILLIS = 60L * 60L * 1000L
        // Backstop expiry for cached tweet bodies.
        const val TWEET_CACHE_TTL_MILLIS = 6L * 60L * 60L * 1000L
        // Total image-blob bytes kept in memory before LRU eviction.
        const val IMAGE_CACHE_MAX_BYTES = 8 * 1024 * 1024
    }
}
