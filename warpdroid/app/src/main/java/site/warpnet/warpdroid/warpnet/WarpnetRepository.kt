/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.warpnet

import site.warpnet.warpdroid.components.pairing.PairedNodeStore
import site.warpnet.warpdroid.entity.Account
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.Relationship
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.entity.TimelineAccount
import site.warpnet.warpdroid.warpnet.WarpnetMapper.toAccount
import site.warpnet.warpdroid.warpnet.WarpnetMapper.toNotification
import site.warpnet.warpdroid.warpnet.WarpnetMapper.toTweet
import site.warpnet.warpdroid.warpnet.WarpnetMapper.toTimelineAccount
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
 * Author lookups are resolved lazily via [resolveUser] with a per-call
 * cache, so a 40-tweet timeline from 3 distinct authors hits
 * `GET_USER` at most three times.
 */
@OptIn(ExperimentalStdlibApi::class)
@Singleton
class WarpnetRepository @Inject constructor(
    private val client: WarpnetClient,
    private val pairedNodeStore: PairedNodeStore,
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
    private val bookmarkEventAdapter = moshi.adapter<site.warpnet.transport.dto.BookmarkEvent>()
    private val unbookmarkEventAdapter = moshi.adapter<site.warpnet.transport.dto.UnbookmarkEvent>()
    private val getBookmarksEventAdapter = moshi.adapter<site.warpnet.transport.dto.GetBookmarksEvent>()
    private val getBookmarksRespAdapter = moshi.adapter<site.warpnet.transport.dto.GetBookmarksResponse>()
    private val pinTweetAdapter = moshi.adapter<site.warpnet.transport.dto.PinTweetEvent>()
    private val blockEventAdapter = moshi.adapter<site.warpnet.transport.dto.BlockEvent>()
    private val muteEventAdapter = moshi.adapter<site.warpnet.transport.dto.MuteEvent>()
    private val getBlocksEventAdapter = moshi.adapter<site.warpnet.transport.dto.GetBlocksEvent>()
    private val getBlocksRespAdapter = moshi.adapter<site.warpnet.transport.dto.GetBlocksResponse>()
    private val muteConvAdapter = moshi.adapter<site.warpnet.transport.dto.MuteConversationEvent>()
    private val getTweetLikersAdapter = moshi.adapter<site.warpnet.transport.dto.GetTweetLikersEvent>()
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
    private val unretweetAdapter = moshi.adapter<UnretweetEvent>()

    // -----------------------------------------------------------------
    // Users
    // -----------------------------------------------------------------

    suspend fun getAccount(userId: String): Account =
        getUser(userId).toAccount()

    suspend fun getTimelineAccount(userId: String): TimelineAccount =
        getUser(userId).toTimelineAccount()

    private suspend fun getUser(userId: String): WarpnetUser {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_USER,
            getUserAdapter.toJson(GetUserEvent(userId = userId)),
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
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_TWEET,
            getTweetAdapter.toJson(GetTweetEvent(tweetId = tweetId, userId = userId)),
        )
        val tweet = tweetAdapter.fromJson(raw)
            ?: throw IllegalStateException("getStatus returned empty body for $tweetId")
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
        return hydrateTweets(page.replies)
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
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_TWEET,
            getTweetAdapter.toJson(GetTweetEvent(tweetId = tweetId, userId = userId)),
        )
        return tweetAdapter.fromJson(raw)
            ?: throw IllegalStateException("fetchTweetRaw returned empty body for $tweetId")
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
    // Retweets
    // -----------------------------------------------------------------

    // Warpnet's NewRetweetEvent is domain.Tweet, so the payload is the
    // retweeter's copy of the tweet rather than a (tweet_id, user_id) pair.
    suspend fun retweetStatus(tweetId: String, retweeterId: String, retweeterUsername: String): Long {
        // createdAt left null for the same reason as postStatus.
        val payload = WarpnetTweet(
            id = tweetId,
            rootId = "",
            text = "",
            userId = retweeterId,
            username = retweeterUsername,
            retweetedBy = retweeterId,
        )
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
        if (page.notifications.isEmpty()) return emptyList<Notification>() to page.cursor

        val cache = mutableMapOf<String, WarpnetUser>()
        val mapped = page.notifications.mapNotNull { n ->
            val author = resolveUser(n.fromUserId, cache) ?: return@mapNotNull null
            n.toNotification(author)
        }
        return mapped to page.cursor
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
        val author = resolveUser(wire.fromUserId, mutableMapOf()) ?: return null
        return wire.toNotification(author)
    }

    // -----------------------------------------------------------------
    // Engagement lists (who liked / retweeted a tweet)
    // -----------------------------------------------------------------

    suspend fun getTweetLikers(tweetId: String, ownerUserId: String, cursor: String = "", limit: Int = 40): Pair<List<TimelineAccount>, String> {
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
        val page = usersRespAdapter.fromJson(raw) ?: return emptyList<TimelineAccount>() to ""
        return page.users.map { it.toTimelineAccount() } to page.cursor
    }

    suspend fun getTweetRetweeters(tweetId: String, ownerUserId: String, cursor: String = "", limit: Int = 40): Pair<List<TimelineAccount>, String> {
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
        val page = usersRespAdapter.fromJson(raw) ?: return emptyList<TimelineAccount>() to ""
        return page.users.map { it.toTimelineAccount() } to page.cursor
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

    suspend fun muteConversation(userId: String, tweetId: String) {
        client.request(
            ProtocolIds.PRIVATE_POST_MUTE_CONVERSATION,
            muteConvAdapter.toJson(
                site.warpnet.transport.dto.MuteConversationEvent(userId = userId, tweetId = tweetId),
            ),
        )
    }

    suspend fun unmuteConversation(userId: String, tweetId: String) {
        client.request(
            ProtocolIds.PRIVATE_POST_UNMUTE_CONVERSATION,
            muteConvAdapter.toJson(
                site.warpnet.transport.dto.MuteConversationEvent(userId = userId, tweetId = tweetId),
            ),
        )
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
    }

    suspend fun unpinTweet(userId: String, tweetId: String) {
        client.request(
            ProtocolIds.PUBLIC_POST_UNPIN,
            pinTweetAdapter.toJson(
                site.warpnet.transport.dto.PinTweetEvent(userId = userId, tweetId = tweetId),
            ),
        )
    }

    // -----------------------------------------------------------------
    // Bookmarks (local-only shelf, no propagation)
    // -----------------------------------------------------------------

    /** Pin [tweetId] (authored by [ownerUserId]) to the local bookmark shelf. */
    suspend fun bookmarkTweet(userId: String, tweetId: String, ownerUserId: String) {
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
    }

    /** Remove a previously-bookmarked tweet from the shelf. */
    suspend fun unbookmarkTweet(userId: String, tweetId: String) {
        client.request(
            ProtocolIds.PRIVATE_POST_UNBOOKMARK,
            unbookmarkEventAdapter.toJson(
                site.warpnet.transport.dto.UnbookmarkEvent(userId = userId, tweetId = tweetId),
            ),
        )
    }

    /**
     * Fetch one page of bookmarked tweets. The wire returns identifiers only —
     * each tweet body is re-fetched in parallel and surfaced as a Tweet.
     */
    suspend fun getBookmarks(userId: String, cursor: String = "", limit: Int = 40): Pair<List<site.warpnet.warpdroid.entity.Tweet>, String> {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_BOOKMARKS,
            getBookmarksEventAdapter.toJson(
                site.warpnet.transport.dto.GetBookmarksEvent(userId = userId, cursor = cursor, limit = limit),
            ),
        )
        val page = getBookmarksRespAdapter.fromJson(raw)
            ?: return emptyList<site.warpnet.warpdroid.entity.Tweet>() to ""
        if (page.items.isEmpty()) {
            return emptyList<site.warpnet.warpdroid.entity.Tweet>() to page.cursor
        }
        val tweets = page.items.mapNotNull { bm ->
            runCatching { getStatus(tweetId = bm.tweetId, userId = bm.ownerUserId) }.getOrNull()
        }
        return tweets to page.cursor
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
    suspend fun getFollowers(userId: String, cursor: String = "", limit: Int = 40): Pair<List<TimelineAccount>, String> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_FOLLOWERS,
            getFollowersAdapter.toJson(GetFollowersEvent(userId = userId, cursor = cursor, limit = limit)),
        )
        val page = followersRespAdapter.fromJson(raw) ?: return emptyList<TimelineAccount>() to ""
        return hydrateAccounts(page.followers) to page.cursor
    }

    suspend fun getFollowings(userId: String, cursor: String = "", limit: Int = 40): Pair<List<TimelineAccount>, String> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_FOLLOWINGS,
            getFollowingsAdapter.toJson(GetFollowingsEvent(userId = userId, cursor = cursor, limit = limit)),
        )
        val page = followingsRespAdapter.fromJson(raw) ?: return emptyList<TimelineAccount>() to ""
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
    suspend fun listUsers(requesterUserId: String, cursor: String = "", limit: Int = 40): Pair<List<TimelineAccount>, String> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_USERS,
            getAllUsersAdapter.toJson(GetAllUsersEvent(userId = requesterUserId, cursor = cursor, limit = limit)),
        )
        val page = usersRespAdapter.fromJson(raw) ?: return emptyList<TimelineAccount>() to ""
        return page.users.map { it.toTimelineAccount() } to page.cursor
    }

    // -----------------------------------------------------------------
    // Internals
    // -----------------------------------------------------------------

    private suspend fun hydrateAccounts(userIds: List<String>): List<TimelineAccount> {
        if (userIds.isEmpty()) return emptyList()
        val cache = mutableMapOf<String, WarpnetUser>()
        return userIds.mapNotNull { id -> resolveUser(id, cache)?.toTimelineAccount() }
    }

    private suspend fun hydrateTweets(tweets: List<WarpnetTweet>): List<Tweet> = coroutineScope {
        if (tweets.isEmpty()) return@coroutineScope emptyList()
        val cache = mutableMapOf<String, WarpnetUser>()
        // Stats are fetched per tweet in parallel so a 30-tweet timeline
        // doesn't pay 30x serialised round-trip latency. Failures degrade
        // to zero counts; the toTweet baseline already matches that.
        val viewerId = pairedNodeStore.load()?.userId.orEmpty()
        if (viewerId.isBlank()) {
            return@coroutineScope tweets.map { it.toTweet(resolveUser(it.userId, cache)) }
        }
        // Retweets reuse the original tweet id, so distinct() avoids
        // firing the same stats RPC twice on a single timeline page.
        val stats = tweets.map { it.id }.distinct().associateWith { id ->
            async { runCatching { getTweetStats(tweetId = id, userId = viewerId) }.getOrNull() }
        }
        tweets.map { t ->
            val base = t.toTweet(resolveUser(t.userId, cache))
            val s = stats[t.id]?.await() ?: return@map base
            base.copy(
                likesCount = s.likesCount.clampToInt(),
                retweetsCount = s.retweetsCount.clampToInt(),
                repliesCount = s.repliesCount.clampToInt(),
                viewsCount = s.viewsCount.clampToInt(),
            )
        }
    }

    private fun Long.clampToInt(): Int = coerceIn(0, Int.MAX_VALUE.toLong()).toInt()

    private suspend fun resolveUser(userId: String, cache: MutableMap<String, WarpnetUser>): WarpnetUser? {
        if (userId.isBlank()) return null
        cache[userId]?.let { return it }
        return runCatching { getUser(userId) }.getOrNull()?.also { cache[userId] = it }
    }
}
