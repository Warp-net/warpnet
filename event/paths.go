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
	PRIVATE_POST_NODE_VALIDATE         = "/private/post/admin/validate/0.0.0"
	PUBLIC_POST_NODE_VALIDATION_RESULT = "/public/post/admin/validate/result/0.0.0"
	PUBLIC_POST_NODE_CHALLENGE         = "/public/post/admin/challenge/0.0.0"
	PRIVATE_POST_PAIR                  = "/private/post/admin/pair/0.0.0"
	PRIVATE_GET_STATS                  = "/private/get/admin/stats/0.0.0"
	// moderation
	PRIVATE_POST_MODERATE         = "/private/post/moderate/0.0.0"
	PUBLIC_POST_MODERATION_RESULT = "/public/post/moderate/result/0.0.0"
	// application
	PRIVATE_DELETE_CHAT       = "/private/delete/chat/0.0.0"
	PRIVATE_DELETE_MESSAGE    = "/private/delete/message/0.0.0"
	PRIVATE_DELETE_TWEET      = "/private/delete/tweet/0.0.0"
	PRIVATE_GET_CHAT          = "/private/get/chat/0.0.0"
	PRIVATE_GET_CHATS         = "/private/get/chats/0.0.0"
	PRIVATE_GET_MESSAGE       = "/private/get/message/0.0.0"
	PRIVATE_GET_MESSAGES      = "/private/get/messages/0.0.0"
	PRIVATE_GET_TIMELINE      = "/private/get/timeline/0.0.0"
	PRIVATE_POST_LOGIN        = "/private/post/login/0.0.0"
	PRIVATE_POST_LOGOUT       = "/private/post/logout/0.0.0"
	PRIVATE_POST_TWEET        = "/private/post/tweet/0.0.0"
	PRIVATE_POST_USER         = "/private/post/user/0.0.0"
	PUBLIC_DELETE_REPLY       = "/public/delete/reply/0.0.0"
	PUBLIC_GET_FOLLOWEES      = "/public/get/followees/0.0.0"
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
	PUBLIC_POST_UNFOLLOW      = "/public/post/unfollow/0.0.0"
	PUBLIC_POST_UNLIKE        = "/public/post/unlike/0.0.0"
	PUBLIC_POST_UNRETWEET     = "/public/post/unretweet/0.0.0"
	PRIVATE_POST_UPLOAD_IMAGE = "/private/post/image/0.0.0"
	PUBLIC_GET_IMAGE          = "/public/get/image/0.0.0"
)
