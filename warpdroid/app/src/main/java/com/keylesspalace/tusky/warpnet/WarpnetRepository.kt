/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package com.keylesspalace.tusky.warpnet

import com.keylesspalace.tusky.entity.Account
import com.keylesspalace.tusky.entity.Notification
import com.keylesspalace.tusky.entity.Relationship
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.entity.TimelineAccount
import com.keylesspalace.tusky.warpnet.WarpnetMapper.toAccount
import com.keylesspalace.tusky.warpnet.WarpnetMapper.toNotification
import com.keylesspalace.tusky.warpnet.WarpnetMapper.toStatus
import com.keylesspalace.tusky.warpnet.WarpnetMapper.toTimelineAccount
import com.squareup.moshi.Moshi
import com.squareup.moshi.adapter
import javax.inject.Inject
import javax.inject.Singleton
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
import site.warpnet.transport.dto.WarpnetTweet
import site.warpnet.transport.dto.WarpnetUser

/**
 * The only entry point from view models into the Warpnet transport.
 *
 * Scope is deliberately small: this is the set of operations that the
 * existing Tusky view models actually need to render the home timeline,
 * one user profile, posting/deleting tweets, follow/unfollow and
 * notifications. Endpoints without Warpnet equivalents
 * (filters, announcements, trending, lists, reports, scheduled statuses,
 * translations, polls, media uploads) are **not** exposed here — the
 * corresponding Tusky features are removed in Phase 5 rather than faked.
 *
 * Author lookups are resolved lazily via [resolveUser] with a per-call
 * cache, so a 40-tweet timeline from 3 distinct authors hits
 * `GET_USER` at most three times.
 */
@OptIn(ExperimentalStdlibApi::class)
@Singleton
class WarpnetRepository @Inject constructor(
    private val client: WarpnetClient,
    moshi: Moshi,
) {
    private val userAdapter = moshi.adapter<WarpnetUser>()
    private val tweetAdapter = moshi.adapter<WarpnetTweet>()
    private val tweetsRespAdapter = moshi.adapter<TweetsResponse>()
    private val repliesRespAdapter = moshi.adapter<RepliesResponse>()
    private val notificationsRespAdapter = moshi.adapter<GetNotificationsResponse>()
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
    private val getFollowersAdapter = moshi.adapter<GetFollowersEvent>()
    private val getFollowingsAdapter = moshi.adapter<GetFollowingsEvent>()
    private val getIsFollowingAdapter = moshi.adapter<GetIsFollowingEvent>()
    private val newFollowAdapter = moshi.adapter<NewFollowEvent>()
    private val newUnfollowAdapter = moshi.adapter<NewUnfollowEvent>()
    private val likeEventAdapter = moshi.adapter<LikeEvent>()
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
    suspend fun getHomeTimeline(cursor: String = "", limit: Int = 40): Pair<List<Status>, String> {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_TIMELINE,
            getAllTweetsAdapter.toJson(GetAllTweetsEvent(userId = "", cursor = cursor, limit = limit)),
        )
        val page = tweetsRespAdapter.fromJson(raw) ?: return emptyList<Status>() to ""
        return hydrateStatuses(page.tweets) to page.cursor
    }

    /** Public per-user feed. Second element is the next-page cursor, empty when exhausted. */
    suspend fun getUserTimeline(userId: String, cursor: String = "", limit: Int = 40): Pair<List<Status>, String> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_TWEETS,
            getAllTweetsAdapter.toJson(GetAllTweetsEvent(userId = userId, cursor = cursor, limit = limit)),
        )
        val page = tweetsRespAdapter.fromJson(raw) ?: return emptyList<Status>() to ""
        return hydrateStatuses(page.tweets) to page.cursor
    }

    suspend fun getStatus(tweetId: String, userId: String): Status {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_TWEET,
            getTweetAdapter.toJson(GetTweetEvent(tweetId = tweetId, userId = userId)),
        )
        val tweet = tweetAdapter.fromJson(raw)
            ?: throw IllegalStateException("getStatus returned empty body for $tweetId")
        val base = tweet.toStatus(author = runCatching { getUser(tweet.userId) }.getOrNull())
        val stats = runCatching { getTweetStats(tweetId = tweet.id, userId = userId) }.getOrNull()
        return if (stats == null) base else base.copy(
            favouritesCount = stats.likesCount.toInt(),
            reblogsCount = stats.retweetsCount.toInt(),
            repliesCount = stats.repliesCount.toInt(),
        )
    }

    suspend fun getTweetStats(tweetId: String, userId: String): TweetStatsResponse {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_TWEET_STATS,
            getTweetStatsAdapter.toJson(GetTweetStatsEvent(tweetId = tweetId, userId = userId)),
        )
        return tweetStatsRespAdapter.fromJson(raw) ?: TweetStatsResponse(tweetId = tweetId)
    }

    suspend fun getReplies(rootId: String, parentId: String = "", cursor: String = ""): List<Status> {
        val raw = client.request(
            ProtocolIds.PUBLIC_GET_REPLIES,
            getRepliesAdapter.toJson(GetAllRepliesEvent(rootId = rootId, parentId = parentId, cursor = cursor)),
        )
        val page = repliesRespAdapter.fromJson(raw) ?: return emptyList()
        return hydrateStatuses(page.replies)
    }

    /**
     * Walk the parent chain upward from [tweetId], fetching each ancestor
     * via PUBLIC_GET_TWEET. Returns ancestors ordered root-first so the
     * thread view can render oldest-to-newest.
     *
     * [maxDepth] bounds the walk to defend against malformed cycles and
     * runaway threads; beyond it the chain is silently truncated.
     */
    suspend fun getAncestors(tweetId: String, userId: String, maxDepth: Int = 32): List<Status> {
        val chain = mutableListOf<Status>()
        var current = runCatching { fetchTweetRaw(tweetId, userId) }.getOrNull() ?: return emptyList()
        val cache = mutableMapOf<String, WarpnetUser>()
        var steps = 0
        while (!current.parentId.isNullOrEmpty() && steps < maxDepth) {
            val parent = runCatching { fetchTweetRaw(current.parentId!!, userId) }.getOrNull() ?: break
            chain += parent.toStatus(resolveUser(parent.userId, cache))
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

    suspend fun postStatus(text: String, authorUserId: String, authorUsername: String, parentId: String? = null): Status {
        val draft = WarpnetTweet(
            id = "",
            createdAt = "",
            rootId = parentId ?: "",
            text = text,
            userId = authorUserId,
            username = authorUsername,
            parentId = parentId,
        )
        val raw = client.request(ProtocolIds.PRIVATE_POST_TWEET, newTweetAdapter.toJson(draft))
        val created = tweetAdapter.fromJson(raw)
            ?: throw IllegalStateException("postStatus returned empty body")
        return created.toStatus(author = runCatching { getUser(created.userId) }.getOrNull())
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

    suspend fun favouriteStatus(tweetId: String, userId: String): Long {
        val raw = client.request(
            ProtocolIds.PUBLIC_POST_LIKE,
            likeEventAdapter.toJson(LikeEvent(tweetId = tweetId, userId = userId)),
        )
        return likesCountAdapter.fromJson(raw)?.likesCount ?: 0
    }

    suspend fun unfavouriteStatus(tweetId: String, userId: String): Long {
        val raw = client.request(
            ProtocolIds.PUBLIC_POST_UNLIKE,
            likeEventAdapter.toJson(LikeEvent(tweetId = tweetId, userId = userId)),
        )
        return likesCountAdapter.fromJson(raw)?.likesCount ?: 0
    }

    // -----------------------------------------------------------------
    // Retweets
    // -----------------------------------------------------------------

    // Warpnet's NewRetweetEvent is domain.Tweet, so the payload is the
    // retweeter's copy of the tweet rather than a (tweet_id, user_id) pair.
    suspend fun reblogStatus(tweetId: String, retweeterId: String, retweeterUsername: String): Long {
        val payload = WarpnetTweet(
            id = tweetId,
            createdAt = "",
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

    suspend fun unreblogStatus(tweetId: String, retweeterId: String): Long {
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

    /** Caller's own notifications feed. Second element is the next-page cursor, empty when exhausted. */
    suspend fun getNotifications(userId: String, cursor: String = "", limit: Int = 40): Pair<List<Notification>, String> {
        val raw = client.request(
            ProtocolIds.PRIVATE_GET_NOTIFICATIONS,
            getNotifsAdapter.toJson(GetNotificationsEvent(userId = userId, cursor = cursor, limit = limit)),
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
     * caller's perspective. Synthesises a Mastodon [Relationship] because
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

    private suspend fun hydrateStatuses(tweets: List<WarpnetTweet>): List<Status> {
        if (tweets.isEmpty()) return emptyList()
        val cache = mutableMapOf<String, WarpnetUser>()
        return tweets.map { t -> t.toStatus(resolveUser(t.userId, cache)) }
    }

    private suspend fun resolveUser(userId: String, cache: MutableMap<String, WarpnetUser>): WarpnetUser? {
        if (userId.isBlank()) return null
        cache[userId]?.let { return it }
        return runCatching { getUser(userId) }.getOrNull()?.also { cache[userId] = it }
    }
}
