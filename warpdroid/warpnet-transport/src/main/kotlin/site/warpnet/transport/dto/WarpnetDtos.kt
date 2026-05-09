/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport.dto

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Wire DTOs for the handful of Warpnet protocol IDs that Warpdroid actually
 * speaks. Shapes mirror `warpnet/domain/warpnet.go` and
 * `warpnet/event/event.go` — see `docs/warpnet-protocol.md` for the full
 * catalogue. Only the endpoints used by [site.warpnet.transport.ProtocolIds]
 * that the [site.warpnet.warpdroid.warpnet.WarpnetRepository] calls are
 * modelled here; adding new protocols means adding the DTO next to the
 * existing ones.
 */

// -----------------------------------------------------------------------------
// Domain objects (from warpnet/domain/warpnet.go)
// -----------------------------------------------------------------------------

@JsonClass(generateAdapter = true)
data class WarpnetTweet(
    val id: String,
    // Pass null when posting a draft tweet so the backend stamps the
    // creation time itself (database/tweet-repo.go:152). Sending the
    // empty string instead fails json-iterator's time.Time decode in
    // the /private/post/tweet handler before the zero-value fallback
    // runs, so the post silently disappears with the server logging
    // an unmarshal error.
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "parent_id") val parentId: String? = null,
    @Json(name = "retweeted_by") val retweetedBy: String? = null,
    @Json(name = "root_id") val rootId: String = "",
    val text: String,
    @Json(name = "user_id") val userId: String,
    val username: String,
    @Json(name = "image_keys") val imageKeys: List<String>? = null,
    val network: String = "",
)

@JsonClass(generateAdapter = true)
data class WarpnetUser(
    val id: String,
    val username: String,
    val bio: String = "",
    val birthdate: String = "",
    @Json(name = "avatar_key") val avatarKey: String? = null,
    @Json(name = "background_image_key") val backgroundImageKey: String = "",
    @Json(name = "created_at") val createdAt: String = "",
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "followings_count") val followingsCount: Long = 0,
    @Json(name = "followers_count") val followersCount: Long = 0,
    @Json(name = "tweets_count") val tweetsCount: Long = 0,
    @Json(name = "isOffline") val isOffline: Boolean = false,
    @Json(name = "node_id") val nodeId: String = "",
    val network: String = "",
    val website: String? = null,
)

@JsonClass(generateAdapter = true)
data class WarpnetNotification(
    val id: String,
    @Json(name = "created_at") val createdAt: String,
    val type: String,
    @Json(name = "from_user_id") val fromUserId: String,
    @Json(name = "tweet_id") val tweetId: String? = null,
    val text: String? = null,
)

// -----------------------------------------------------------------------------
// Request payloads (event.go)
// -----------------------------------------------------------------------------

@JsonClass(generateAdapter = true)
data class GetUserEvent(@Json(name = "user_id") val userId: String)

@JsonClass(generateAdapter = true)
data class GetAllTweetsEvent(
    @Json(name = "user_id") val userId: String,
    val cursor: String = "",
    val limit: Int = 40,
)

@JsonClass(generateAdapter = true)
data class GetTweetEvent(
    @Json(name = "tweet_id") val tweetId: String,
    @Json(name = "user_id") val userId: String,
)

@JsonClass(generateAdapter = true)
data class GetAllRepliesEvent(
    @Json(name = "root_id") val rootId: String,
    @Json(name = "parent_id") val parentId: String = "",
    val cursor: String = "",
    val limit: Int = 40,
)

@JsonClass(generateAdapter = true)
data class NewFollowEvent(
    @Json(name = "follower") val followerId: String,
    @Json(name = "followee") val followeeId: String,
)

@JsonClass(generateAdapter = true)
data class NewUnfollowEvent(
    @Json(name = "follower") val followerId: String,
    @Json(name = "followee") val followeeId: String,
)

@JsonClass(generateAdapter = true)
data class LikeEvent(
    @Json(name = "tweet_id") val tweetId: String,
    @Json(name = "user_id") val userId: String,
)

/**
 * Records a tweet view on the author's node.
 *
 * - [tweetId] — id of the tweet being viewed.
 * - [userId]  — the tweet *author's* id (kept on the wire as `user_id` to
 *               match warpnet's `event.ViewEvent` shape; the literal
 *               package lives in the Go node, not in this module, so
 *               the link is intentionally plain text).
 * - [viewerId] — the local pairing user's id; the backend skips
 *                self-views and dedupes per (tweetId, viewerId) within
 *                a 30-minute window.
 */
@JsonClass(generateAdapter = true)
data class ViewEvent(
    @Json(name = "tweet_id") val tweetId: String,
    @Json(name = "user_id") val userId: String,
    @Json(name = "viewer_id") val viewerId: String,
)

@JsonClass(generateAdapter = true)
data class GetTweetStatsEvent(
    @Json(name = "tweet_id") val tweetId: String,
    @Json(name = "user_id") val userId: String,
)

@JsonClass(generateAdapter = true)
data class DeleteTweetEvent(
    @Json(name = "tweet_id") val tweetId: String,
    @Json(name = "user_id") val userId: String,
)

@JsonClass(generateAdapter = true)
data class UnretweetEvent(
    @Json(name = "tweet_id") val tweetId: String,
    @Json(name = "retweeter_id") val retweeterId: String,
)

@JsonClass(generateAdapter = true)
data class GetNotificationsEvent(
    @Json(name = "user_id") val userId: String,
    val cursor: String = "",
    val limit: Int = 40,
)

@JsonClass(generateAdapter = true)
data class GetFollowersEvent(
    @Json(name = "user_id") val userId: String,
    val cursor: String = "",
    val limit: Int = 40,
)

@JsonClass(generateAdapter = true)
data class GetFollowingsEvent(
    @Json(name = "user_id") val userId: String,
    val cursor: String = "",
    val limit: Int = 40,
)

/**
 * Probe event for PUBLIC_POST_IS_FOLLOWING / PUBLIC_POST_IS_FOLLOWER.
 *
 * The server answers from the caller's perspective: "am I following
 * `user_id`?" / "does `user_id` follow me?" — so the single field is
 * the *other* party's id.
 */
@JsonClass(generateAdapter = true)
data class GetIsFollowingEvent(
    @Json(name = "user_id") val userId: String,
)

@JsonClass(generateAdapter = true)
data class GetAllUsersEvent(
    @Json(name = "user_id") val userId: String,
    val cursor: String = "",
    val limit: Int = 40,
)

// -----------------------------------------------------------------------------
// Response payloads
// -----------------------------------------------------------------------------

@JsonClass(generateAdapter = true)
data class TweetsResponse(
    val tweets: List<WarpnetTweet> = emptyList(),
    val cursor: String = "",
)

@JsonClass(generateAdapter = true)
data class RepliesResponse(
    val replies: List<WarpnetTweet> = emptyList(),
    val cursor: String = "",
)

@JsonClass(generateAdapter = true)
data class UsersResponse(
    val users: List<WarpnetUser> = emptyList(),
    val cursor: String = "",
)

/**
 * Standalone counter response returned by `PUBLIC_POST_LIKE` /
 * `PUBLIC_POST_UNLIKE` / `PUBLIC_POST_RETWEET` / `PUBLIC_POST_UNRETWEET`.
 *
 * Wire format is `{"count": N}` (warpnet's `event.LikesCountResponse`
 * marshals its single `Count` field as `count`). The previous
 * `@Json(name = "likes_count")` here always parsed to 0 — that name
 * lives on the inner `likes_count` field of [TweetStatsResponse],
 * not on this standalone response.
 */
@JsonClass(generateAdapter = true)
data class LikesCountResponse(
    @Json(name = "count") val likesCount: Long = 0,
)

/**
 * Standalone counter response returned by `PUBLIC_POST_VIEW`. Same
 * `{"count": N}` wire shape as [LikesCountResponse]; we keep a
 * dedicated type so call sites can read `count` without confusing
 * it with the like/retweet counter.
 */
@JsonClass(generateAdapter = true)
data class ViewsCountResponse(
    @Json(name = "count") val count: Long = 0,
)

@JsonClass(generateAdapter = true)
data class GetNotificationsResponse(
    val notifications: List<WarpnetNotification> = emptyList(),
    val cursor: String = "",
    @Json(name = "unread_count") val unreadCount: Long = 0,
)

@JsonClass(generateAdapter = true)
data class FollowersResponse(
    val followers: List<String> = emptyList(),
    val cursor: String = "",
    @Json(name = "following_id") val followingId: String = "",
)

@JsonClass(generateAdapter = true)
data class FollowingsResponse(
    val followings: List<String> = emptyList(),
    val cursor: String = "",
    @Json(name = "follower_id") val followerId: String = "",
)

@JsonClass(generateAdapter = true)
data class IsFollowingResponse(
    @Json(name = "is_following") val isFollowing: Boolean = false,
)

@JsonClass(generateAdapter = true)
data class IsFollowerResponse(
    @Json(name = "is_follower") val isFollower: Boolean = false,
)

@JsonClass(generateAdapter = true)
data class TweetStatsResponse(
    @Json(name = "tweet_id") val tweetId: String = "",
    @Json(name = "tweets_count") val tweetsCount: Long = 0,
    @Json(name = "retweets_count") val retweetsCount: Long = 0,
    @Json(name = "likes_count") val likesCount: Long = 0,
    @Json(name = "replies_count") val repliesCount: Long = 0,
    @Json(name = "views_count") val viewsCount: Long = 0,
)
