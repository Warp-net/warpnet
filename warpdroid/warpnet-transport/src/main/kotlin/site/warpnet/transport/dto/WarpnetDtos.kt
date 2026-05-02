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
 * that the [com.keylesspalace.tusky.warpnet.WarpnetRepository] calls are
 * modelled here; adding new protocols means adding the DTO next to the
 * existing ones.
 */

// -----------------------------------------------------------------------------
// Domain objects (from warpnet/domain/warpnet.go)
// -----------------------------------------------------------------------------

@JsonClass(generateAdapter = true)
data class WarpnetTweet(
    val id: String,
    @Json(name = "created_at") val createdAt: String,
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

@JsonClass(generateAdapter = true)
data class LikesCountResponse(
    @Json(name = "likes_count") val likesCount: Long = 0,
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
