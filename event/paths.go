/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU Affero General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU Affero General Public License for more details.

 You should have received a copy of the GNU Affero General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package event

const (
	// admin
	PRIVATE_POST_PAIR          = "/private/post/admin/pair/0.0.0"
	PUBLIC_POST_NODE_CHALLENGE = "/public/post/admin/challenge/0.0.0"
	PRIVATE_GET_STATS          = "/private/get/admin/stats/0.0.0"
	// moderation
	PUBLIC_POST_MODERATION_RESULT = "/public/post/moderate/result/0.0.0"
	// application
	PRIVATE_DELETE_CHAT       = "/private/delete/chat/0.0.0"
	PRIVATE_DELETE_MESSAGE    = "/private/delete/message/0.0.0"
	PRIVATE_DELETE_TWEET      = "/private/delete/tweet/0.0.0"
	PRIVATE_GET_CHAT          = "/private/get/chat/0.0.0"
	PRIVATE_GET_CHATS         = "/private/get/chats/0.0.0"
	PRIVATE_GET_NOTIFICATION       = "/private/get/notification/0.0.0"
	PRIVATE_GET_NOTIFICATIONS      = "/private/get/notifications/0.0.0"
	PRIVATE_POST_NOTIFICATION_READ = "/private/post/notification/read/0.0.0"
	PRIVATE_GET_MESSAGE       = "/private/get/message/0.0.0"
	PRIVATE_GET_MESSAGES      = "/private/get/messages/0.0.0"
	PRIVATE_GET_TIMELINE      = "/private/get/timeline/0.0.0"
	PRIVATE_POST_LOGIN        = "/private/post/login/0.0.0"
	PRIVATE_POST_LOGOUT       = "/private/post/logout/0.0.0"
	PRIVATE_POST_TWEET        = "/private/post/tweet/0.0.0"
	PRIVATE_POST_USER         = "/private/post/user/0.0.0"
	PUBLIC_DELETE_REPLY       = "/public/delete/reply/0.0.0"
	PUBLIC_GET_FOLLOWINGS     = "/public/get/followings/0.0.0"
	PUBLIC_GET_FOLLOWERS      = "/public/get/followers/0.0.0"
	PUBLIC_GET_INFO           = "/public/get/info/0.0.0"
	PUBLIC_GET_REPLIES        = "/public/get/replies/0.0.0"
	PUBLIC_GET_REPLY          = "/public/get/reply/0.0.0"
	PUBLIC_GET_TWEET          = "/public/get/tweet/0.0.0"
	PUBLIC_GET_TWEET_STATS    = "/public/get/tweetstats/0.0.0"
	PUBLIC_GET_TWEETS         = "/public/get/tweets/0.0.0"
	PUBLIC_GET_USER           = "/public/get/user/0.0.0"
	PUBLIC_GET_USERS          = "/public/get/users/0.0.0"
	PUBLIC_GET_WHOTOFOLLOW    = "/public/get/whotofollow/0.0.0"
	PUBLIC_POST_CHAT          = "/public/post/chat/0.0.0"
	PUBLIC_POST_FOLLOW        = "/public/post/follow/0.0.0"
	PUBLIC_POST_LIKE          = "/public/post/like/0.0.0"
	PUBLIC_POST_MESSAGE       = "/public/post/message/0.0.0"
	PUBLIC_POST_REPLY         = "/public/post/reply/0.0.0"
	PUBLIC_POST_RETWEET       = "/public/post/retweet/0.0.0"
	PUBLIC_POST_IS_FOLLOWING  = "/public/post/isfollowing/0.0.0"
	PUBLIC_POST_IS_FOLLOWER   = "/public/post/isfollower/0.0.0"
	PUBLIC_POST_UNFOLLOW      = "/public/post/unfollow/0.0.0"
	PUBLIC_POST_UNLIKE        = "/public/post/unlike/0.0.0"
	PUBLIC_POST_UNRETWEET     = "/public/post/unretweet/0.0.0"
	PUBLIC_POST_VIEW          = "/public/post/view/0.0.0"
	PRIVATE_POST_UPLOAD_IMAGE = "/private/post/image/0.0.0"
	PUBLIC_GET_IMAGE          = "/public/get/image/0.0.0"
	PRIVATE_POST_BOOKMARK     = "/private/post/bookmark/0.0.0"
	PRIVATE_POST_UNBOOKMARK   = "/private/post/unbookmark/0.0.0"
	PRIVATE_GET_BOOKMARKS     = "/private/get/bookmarks/0.0.0"
	PUBLIC_POST_PIN           = "/public/post/pin/0.0.0"
	PUBLIC_POST_UNPIN         = "/public/post/unpin/0.0.0"

	PRIVATE_POST_BLOCK              = "/private/post/block/0.0.0"
	PRIVATE_POST_UNBLOCK            = "/private/post/unblock/0.0.0"
	PRIVATE_GET_BLOCKS              = "/private/get/blocks/0.0.0"
	PRIVATE_POST_MUTE               = "/private/post/mute/0.0.0"
	PRIVATE_POST_UNMUTE             = "/private/post/unmute/0.0.0"
	PRIVATE_GET_MUTES               = "/private/get/mutes/0.0.0"

	PUBLIC_GET_TWEET_LIKERS     = "/public/get/tweet/likers/0.0.0"
	PUBLIC_GET_TWEET_RETWEETERS = "/public/get/tweet/retweeters/0.0.0"

	PRIVATE_POST_SUBSCRIBE_USER   = "/private/post/subscribe/user/0.0.0"
	PRIVATE_POST_UNSUBSCRIBE_USER = "/private/post/unsubscribe/user/0.0.0"

	PRIVATE_POST_MEDIA_META = "/private/post/media/meta/0.0.0"
	PRIVATE_GET_MEDIA       = "/private/get/media/0.0.0"

	PUBLIC_GET_USERS_SEARCH = "/public/get/users/search/0.0.0"

	PRIVATE_POST_TWEET_EDIT = "/private/post/tweet/edit/0.0.0"

	PUBLIC_POST_QUOTE   = "/public/post/quote/0.0.0"
	PUBLIC_GET_QUOTING  = "/public/get/quoting/0.0.0"
	PUBLIC_DELETE_QUOTE = "/public/delete/quote/0.0.0"

	PRIVATE_GET_FOLLOW_REQUESTS           = "/private/get/follow/requests/0.0.0"
	PRIVATE_POST_FOLLOW_REQUEST_AUTHORIZE = "/private/post/follow/request/authorize/0.0.0"
	PRIVATE_POST_FOLLOW_REQUEST_REJECT    = "/private/post/follow/request/reject/0.0.0"

	PRIVATE_GET_FILTER             = "/private/get/filter/0.0.0"
	PRIVATE_GET_FILTERS            = "/private/get/filters/0.0.0"
	PRIVATE_POST_FILTER            = "/private/post/filter/0.0.0"
	PRIVATE_POST_FILTER_UPDATE     = "/private/post/filter/update/0.0.0"
	PRIVATE_DELETE_FILTER          = "/private/delete/filter/0.0.0"
	PRIVATE_POST_FILTER_KEYWORD    = "/private/post/filter/keyword/0.0.0"
	PRIVATE_POST_FILTER_KEYWORD_UPDATE = "/private/post/filter/keyword/update/0.0.0"
	PRIVATE_DELETE_FILTER_KEYWORD  = "/private/delete/filter/keyword/0.0.0"
)
