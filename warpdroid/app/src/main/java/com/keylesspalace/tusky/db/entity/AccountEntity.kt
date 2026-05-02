/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package com.keylesspalace.tusky.db.entity

import com.keylesspalace.tusky.TabData
import com.keylesspalace.tusky.components.systemnotifications.NotificationChannelData
import com.keylesspalace.tusky.defaultTabs
import com.keylesspalace.tusky.entity.Emoji
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.settings.DefaultReplyVisibility
import com.keylesspalace.tusky.settings.QuotePolicy

/**
 * In-memory replacement for Tusky's Room-backed account record.
 *
 * Warpdroid has no login or per-account persistence — there is exactly one
 * stub account (see [com.keylesspalace.tusky.db.AccountManager]) which exists
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
    val notificationsReblogged: Boolean = true,
    val notificationsFavorited: Boolean = true,
    val notificationsPolls: Boolean = true,
    val notificationsSubscriptions: Boolean = true,
    val notificationsUpdates: Boolean = true,
    val notificationsAdmin: Boolean = true,
    val notificationsOther: Boolean = true,
    val notificationSound: Boolean = true,
    val notificationVibration: Boolean = true,
    val notificationLight: Boolean = true,
    val defaultPostPrivacy: Status.Visibility = Status.Visibility.PUBLIC,
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
    val isShowHomeBoosts: Boolean = true,
    val isShowHomeReplies: Boolean = true,
    val isShowHomeSelfBoosts: Boolean = true,
) {
    val identifier: String
        get() = "$domain:$accountId"

    val fullName: String
        get() = "@$username@$domain"

    fun isPushNotificationsEnabled(): Boolean = unifiedPushUrl.isNotEmpty()

    fun matchesPushSubscription(endpoint: String): Boolean = unifiedPushUrl == endpoint
}
