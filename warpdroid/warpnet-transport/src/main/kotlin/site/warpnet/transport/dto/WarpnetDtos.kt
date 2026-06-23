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
    // Parent tweet author's id; set on replies so the fat node can forward
    // the reply to the node hosting the parent (see domain.Tweet).
    @Json(name = "parent_user_id") val parentUserId: String? = null,
    @Json(name = "retweeted_by") val retweetedBy: String? = null,
    @Json(name = "root_id") val rootId: String = "",
    val text: String,
    @Json(name = "user_id") val userId: String,
    val username: String,
    @Json(name = "image_keys") val imageKeys: List<String>? = null,
    val network: String = "",
    val pinned: Boolean = false,
    @Json(name = "quoted_tweet_id") val quotedTweetId: String? = null,
    @Json(name = "quoted_user_id") val quotedUserId: String? = null,
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
    // Locked = "manually-approve followers"; gates the follow-request UI.
    val locked: Boolean = false,
)

// Wire shape mirrors domain.Notification on the fat node. The actor is
// already embedded in [text] ("Alice liked your tweet"); the wire does not
// carry a separate from_user_id or tweet_id.
@JsonClass(generateAdapter = true)
data class WarpnetNotification(
    val id: String,
    @Json(name = "created_at") val createdAt: String,
    val type: String,
    val text: String = "",
    @Json(name = "user_id") val userId: String = "",
    @Json(name = "is_read") val isRead: Boolean = false,
)

// -----------------------------------------------------------------------------
// Request payloads (event.go)
// -----------------------------------------------------------------------------

@JsonClass(generateAdapter = true)
data class GetUserEvent(
    @Json(name = "user_id") val userId: String,
    // Optional hint naming the node to resolve the user from when it is
    // unknown locally (the node that produced the list/timeline the user
    // came from). Mirrors the Vue client's getProfile(id, nodeId). The
    // server treats an empty/missing node_id as "no hint", so the default
    // is safe to always send.
    @Json(name = "node_id") val nodeId: String = "",
)

// Two shapes share this event, dispatched on rootId server-side:
//   - timeline/profile: userId set, rootId empty.
//   - thread replies: rootId set; parentId selects the subtree (empty means
//     the replies hanging off rootId); rootUserId lets the node forward to the
//     root author's home node. Replies come back as a flat TweetsResponse.
@JsonClass(generateAdapter = true)
data class GetAllTweetsEvent(
    @Json(name = "user_id") val userId: String = "",
    @Json(name = "root_id") val rootId: String = "",
    @Json(name = "parent_id") val parentId: String = "",
    @Json(name = "root_user_id") val rootUserId: String = "",
    val cursor: String = "",
    val limit: Int = 40,
)

@JsonClass(generateAdapter = true)
data class GetTweetEvent(
    @Json(name = "tweet_id") val tweetId: String,
    @Json(name = "user_id") val userId: String,
)

@JsonClass(generateAdapter = true)
data class NewFollowEvent(
    @Json(name = "follower_id") val followerId: String,
    @Json(name = "following_id") val followeeId: String,
)

@JsonClass(generateAdapter = true)
data class NewUnfollowEvent(
    @Json(name = "follower_id") val followerId: String,
    @Json(name = "following_id") val followeeId: String,
)

@JsonClass(generateAdapter = true)
data class LikeEvent(
    @Json(name = "tweet_id") val tweetId: String,
    @Json(name = "user_id") val userId: String,
    @Json(name = "owner_id") val ownerId: String,
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
    val cursor: String = "",
    val limit: Int = 40,
)

@JsonClass(generateAdapter = true)
data class GetNotificationEvent(
    @Json(name = "notification_id") val notificationId: String,
)

@JsonClass(generateAdapter = true)
data class MarkNotificationReadEvent(
    @Json(name = "notification_id") val notificationId: String,
)

@JsonClass(generateAdapter = true)
data class BookmarkEvent(
    @Json(name = "user_id") val userId: String,
    @Json(name = "tweet_id") val tweetId: String,
    @Json(name = "owner_user_id") val ownerUserId: String,
)

@JsonClass(generateAdapter = true)
data class UnbookmarkEvent(
    @Json(name = "user_id") val userId: String,
    @Json(name = "tweet_id") val tweetId: String,
)

@JsonClass(generateAdapter = true)
data class GetBookmarksEvent(
    @Json(name = "user_id") val userId: String,
    val cursor: String = "",
    val limit: Int = 40,
)

@JsonClass(generateAdapter = true)
data class BookmarkItem(
    @Json(name = "user_id") val userId: String,
    @Json(name = "tweet_id") val tweetId: String,
    @Json(name = "owner_user_id") val ownerUserId: String,
    @Json(name = "created_at") val createdAt: String = "",
)

@JsonClass(generateAdapter = true)
data class GetBookmarksResponse(
    val items: List<BookmarkItem> = emptyList(),
    val cursor: String = "",
)

@JsonClass(generateAdapter = true)
data class PinTweetEvent(
    @Json(name = "user_id") val userId: String,
    @Json(name = "tweet_id") val tweetId: String,
)

@JsonClass(generateAdapter = true)
data class BlockEvent(
    @Json(name = "blocker_id") val blockerId: String,
    @Json(name = "blockee_id") val blockeeId: String,
)

@JsonClass(generateAdapter = true)
data class MuteEvent(
    @Json(name = "muter_id") val muterId: String,
    @Json(name = "mutee_id") val muteeId: String,
)

@JsonClass(generateAdapter = true)
data class GetBlocksEvent(
    @Json(name = "user_id") val userId: String,
    val cursor: String = "",
    val limit: Int = 40,
)

@JsonClass(generateAdapter = true)
data class GetBlocksResponse(
    val ids: List<String> = emptyList(),
    val cursor: String = "",
)

@JsonClass(generateAdapter = true)
data class GetTweetLikersEvent(
    @Json(name = "tweet_id") val tweetId: String,
    @Json(name = "owner_user_id") val ownerUserId: String,
    val cursor: String = "",
    val limit: Int = 40,
)

@JsonClass(generateAdapter = true)
data class SubscribeUserEvent(
    @Json(name = "self_id") val selfId: String,
    @Json(name = "target_id") val targetId: String,
)

@JsonClass(generateAdapter = true)
data class UpdateMediaMetaEvent(
    @Json(name = "user_id") val userId: String,
    val key: String,
    val description: String = "",
    @Json(name = "focus_x") val focusX: Float = 0f,
    @Json(name = "focus_y") val focusY: Float = 0f,
)

@JsonClass(generateAdapter = true)
data class GetMediaEvent(
    @Json(name = "user_id") val userId: String,
    val key: String,
)

@JsonClass(generateAdapter = true)
data class GetMediaResponse(
    val key: String = "",
    val description: String = "",
    @Json(name = "focus_x") val focusX: Float = 0f,
    @Json(name = "focus_y") val focusY: Float = 0f,
)

@JsonClass(generateAdapter = true)
data class SearchUsersEvent(
    val query: String,
    val cursor: String = "",
    val limit: Int = 40,
)

@JsonClass(generateAdapter = true)
data class EditTweetEvent(
    @Json(name = "tweet_id") val tweetId: String,
    @Json(name = "user_id") val userId: String,
    val text: String,
)
@JsonClass(generateAdapter = true)
data class GetFollowRequestsEvent(
    @Json(name = "user_id") val userId: String,
    val cursor: String = "",
    val limit: Int = 40,
)

@JsonClass(generateAdapter = true)
data class GetFollowRequestsResponse(
    @Json(name = "follower_ids") val followerIds: List<String> = emptyList(),
    val cursor: String = "",
)

@JsonClass(generateAdapter = true)
data class FollowRequestActionEvent(
    @Json(name = "user_id") val userId: String,
    @Json(name = "follower_id") val followerId: String,
)

@JsonClass(generateAdapter = true)
data class WarpnetFilterKeyword(
    val id: String = "",
    val keyword: String = "",
    @Json(name = "whole_word") val wholeWord: Boolean = false,
)

@JsonClass(generateAdapter = true)
data class WarpnetFilter(
    val id: String = "",
    @Json(name = "user_id") val userId: String = "",
    val title: String = "",
    val context: List<String> = emptyList(),
    val action: String = "warn",
    @Json(name = "expires_at") val expiresAt: String? = null,
    val keywords: List<WarpnetFilterKeyword> = emptyList(),
)

@JsonClass(generateAdapter = true)
data class GetFilterEvent(
    @Json(name = "user_id") val userId: String,
    @Json(name = "filter_id") val filterId: String,
)

@JsonClass(generateAdapter = true)
data class GetFiltersEvent(
    @Json(name = "user_id") val userId: String,
    val cursor: String = "",
    val limit: Int = 40,
)

@JsonClass(generateAdapter = true)
data class GetFiltersResponse(
    val filters: List<WarpnetFilter> = emptyList(),
    val cursor: String = "",
)

@JsonClass(generateAdapter = true)
data class DeleteFilterEvent(
    @Json(name = "user_id") val userId: String,
    @Json(name = "filter_id") val filterId: String,
)

@JsonClass(generateAdapter = true)
data class AddFilterKeywordEvent(
    @Json(name = "user_id") val userId: String,
    @Json(name = "filter_id") val filterId: String,
    val keyword: String,
    @Json(name = "whole_word") val wholeWord: Boolean = false,
)

@JsonClass(generateAdapter = true)
data class UpdateFilterKeywordEvent(
    @Json(name = "user_id") val userId: String,
    @Json(name = "keyword_id") val keywordId: String,
    val keyword: String = "",
    @Json(name = "whole_word") val wholeWord: Boolean = false,
)

@JsonClass(generateAdapter = true)
data class DeleteFilterKeywordEvent(
    @Json(name = "user_id") val userId: String,
    @Json(name = "keyword_id") val keywordId: String,
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

// Wire shape mirrors event.TweetStatsResponse. The wire does NOT carry
// "tweets_count" — that field only exists on user-level stats, not per
// tweet, so declaring it here just defaulted the value to 0 forever.
@JsonClass(generateAdapter = true)
data class TweetStatsResponse(
    @Json(name = "tweet_id") val tweetId: String = "",
    @Json(name = "retweets_count") val retweetsCount: Long = 0,
    @Json(name = "likes_count") val likesCount: Long = 0,
    @Json(name = "replies_count") val repliesCount: Long = 0,
    @Json(name = "views_count") val viewsCount: Long = 0,
)

@JsonClass(generateAdapter = true)
data class GetChatsEvent(
    @Json(name = "user_id") val userId: String,
    val limit: Int = 40,
    val cursor: String = "",
)

@JsonClass(generateAdapter = true)
data class GetChatsResponse(
    val chats: List<WarpnetChat> = emptyList(),
    val cursor: String = "",
)

@JsonClass(generateAdapter = true)
data class WarpnetChat(
    val id: String = "",
    @Json(name = "owner_id") val ownerId: String = "",
    @Json(name = "other_user_id") val otherUserId: String = "",
    @Json(name = "last_message") val lastMessage: String = "",
    @Json(name = "created_at") val createdAt: String = "",
    @Json(name = "updated_at") val updatedAt: String = "",
)

// Wire shape mirrors event.NewChatEvent. chat_id is omitted so the fat node
// composes the deterministic 1:1 chat id; the response is a WarpnetChat.
@JsonClass(generateAdapter = true)
data class NewChatEvent(
    @Json(name = "owner_id") val ownerId: String,
    @Json(name = "other_user_id") val otherUserId: String,
)

@JsonClass(generateAdapter = true)
data class GetChatEvent(
    @Json(name = "user_id") val userId: String,
    @Json(name = "chat_id") val chatId: String,
)

@JsonClass(generateAdapter = true)
data class DeleteChatEvent(
    @Json(name = "user_id") val userId: String,
    @Json(name = "chat_id") val chatId: String,
)

@JsonClass(generateAdapter = true)
data class GetImageEvent(
    @Json(name = "user_id") val userId: String,
    val key: String,
)

// Wire shape mirrors event.GetImageResponse. The fat node returns the
// image as a single string "<mime>,<base64>" — the Vue front-end shoves
// the same string directly into an <img src="…"> data-URL slot. Warpdroid
// splits and base64-decodes the bytes before handing them to Glide.
@JsonClass(generateAdapter = true)
data class GetImageResponse(
    val file: String = "",
)

// Wire shape mirrors event.GetAllMessagesEvent on the fat node.
@JsonClass(generateAdapter = true)
data class GetMessagesEvent(
    @Json(name = "owner_id") val ownerId: String,
    @Json(name = "chat_id") val chatId: String,
    val limit: Int = 40,
    val cursor: String = "",
)

// Wire shape mirrors event.ChatMessagesResponse.
@JsonClass(generateAdapter = true)
data class GetMessagesResponse(
    @Json(name = "chat_id") val chatId: String = "",
    val cursor: String = "",
    val messages: List<WarpnetMessage> = emptyList(),
)

// Wire shape mirrors domain.ChatMessage. Used to PARSE responses; sending must
// use NewMessageEvent instead, since an empty created_at ("") fails to
// unmarshal into the node's time.Time field.
@JsonClass(generateAdapter = true)
data class WarpnetMessage(
    val id: String = "",
    @Json(name = "chat_id") val chatId: String = "",
    @Json(name = "sender_id") val senderId: String = "",
    @Json(name = "receiver_id") val receiverId: String = "",
    val text: String = "",
    @Json(name = "created_at") val createdAt: String = "",
    val status: String = "",
)

// Send-only body for PUBLIC_POST_MESSAGE: just the fields the node reads. The
// server assigns id/created_at/status, so we must not send created_at="" — the
// node parses it into time.Time and rejects the empty string.
@JsonClass(generateAdapter = true)
data class NewMessageEvent(
    @Json(name = "chat_id") val chatId: String,
    @Json(name = "sender_id") val senderId: String,
    @Json(name = "receiver_id") val receiverId: String,
    val text: String,
)

// Wire shape mirrors event.DeleteMessageEvent = GetMessageEvent.
@JsonClass(generateAdapter = true)
data class DeleteMessageEvent(
    @Json(name = "chat_id") val chatId: String,
    val id: String,
)

// Wire shape mirrors event.ReportEvent. `type` uses the same numeric
// ModerationObjectType enum the fat node defines; the backend
// currently accepts only 0 (user) and 1 (tweet) — reply (2) and image
// (3) reports are validated out server-side. `reason` is a free-form
// string capped at 256 chars by the backend (the Android UI offers a
// fixed set of presets but any non-empty short string is accepted).
// `object_id` must be the tweet id when type == 1; for user reports
// it is omitted (wire sends "").
@JsonClass(generateAdapter = true)
data class WarpnetReportEvent(
    val type: Int,
    @Json(name = "object_id") val objectId: String = "",
    @Json(name = "target_user_id") val targetUserId: String,
    @Json(name = "target_node_id") val targetNodeId: String,
    val reason: String,
)
