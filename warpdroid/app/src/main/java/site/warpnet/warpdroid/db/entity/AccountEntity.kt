/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.db.entity

import site.warpnet.warpdroid.TabData
import site.warpnet.warpdroid.components.systemnotifications.NotificationChannelData
import site.warpnet.warpdroid.defaultTabs
import site.warpnet.warpdroid.entity.Emoji
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.settings.DefaultReplyVisibility
import site.warpnet.warpdroid.settings.QuotePolicy

/**
 * In-memory replacement for Warpdroid's Room-backed account record.
 *
 * Warpdroid has no login or per-account persistence — there is exactly one
 * stub account (see [site.warpnet.warpdroid.db.AccountManager]) which exists
 * for the lifetime of the process. The field surface is preserved so
 * downstream call sites (UI preferences, status display options, etc.) keep
 * compiling without being rewritten.
 */
data class AccountEntity(
    val id: Long,
    val domain: String,
    val accessToken: String,
    val clientId: String?,
    val clientSecret: String?,
    val isActive: Boolean,
    val accountId: String = "",
    val username: String = "",
    val displayName: String = "",
    val profilePictureUrl: String = "",
    val staticProfilePictureUrl: String = "",
    val profileHeaderUrl: String = "",
    val notificationsEnabled: Boolean = true,
    val notificationsMentioned: Boolean = true,
    val notificationsFollowed: Boolean = true,
    val notificationsFollowRequested: Boolean = true,
    val notificationsRetweeted: Boolean = true,
    val notificationsLiked: Boolean = true,
    val notificationsPolls: Boolean = true,
    val notificationsSubscriptions: Boolean = true,
    val notificationsUpdates: Boolean = true,
    val notificationsAdmin: Boolean = true,
    val notificationsOther: Boolean = true,
    val notificationSound: Boolean = true,
    val notificationVibration: Boolean = true,
    val notificationLight: Boolean = true,
    val defaultPostPrivacy: Tweet.Visibility = Tweet.Visibility.PUBLIC,
    val defaultReplyPrivacy: DefaultReplyVisibility = DefaultReplyVisibility.MATCH_DEFAULT_POST_VISIBILITY,
    val defaultMediaSensitivity: Boolean = false,
    val defaultPostLanguage: String = "",
    val defaultQuotePolicy: QuotePolicy = QuotePolicy.FOLLOWERS,
    val alwaysShowSensitiveMedia: Boolean = false,
    val alwaysOpenSpoiler: Boolean = false,
    val mediaPreviewEnabled: Boolean = true,
    val lastNotificationId: String = "0",
    val notificationMarkerId: String = "0",
    val emojis: List<Emoji> = emptyList(),
    val tabPreferences: List<TabData> = defaultTabs(),
    val notificationsFilter: Set<NotificationChannelData> = emptySet(),
    val oauthScopes: String = "",
    val unifiedPushUrl: String = "",
    val firstVisibleHomeTimelineItemIndex: Int = 0,
    val firstVisibleHomeTimelineItemOffset: Int = 0,
    val locked: Boolean = false,
    val hasDirectMessageBadge: Boolean = false,
    val isShowHomeRetweets: Boolean = true,
    val isShowHomeReplies: Boolean = true,
    val isShowHomeSelfRetweets: Boolean = true,
) {
    val identifier: String
        get() = "$domain:$accountId"

    val fullName: String
        get() = "@$username@$domain"
}
