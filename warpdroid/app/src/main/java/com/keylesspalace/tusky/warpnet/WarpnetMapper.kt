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
import com.keylesspalace.tusky.entity.notificationTypeFromString
import java.util.Date
import site.warpnet.transport.dto.WarpnetNotification
import site.warpnet.transport.dto.WarpnetTweet
import site.warpnet.transport.dto.WarpnetUser

/**
 * Maps Warpnet wire DTOs onto Tusky's pre-existing Mastodon-shaped entities.
 *
 * Most Mastodon concepts have no direct Warpnet equivalent (rich HTML content,
 * media attachment metadata, visibility enums, filter matches, emojis beyond
 * what [Status] requires). Where the translation is lossy, this mapper picks
 * the safest default — public visibility, no attachments, empty emoji list —
 * so the rest of the UI keeps working without special-casing the bridged
 * account. See [FAKE_BASE_URL] for how remote-origin URLs are synthesised.
 */
object WarpnetMapper {

    /** Mastodon UIs rely on URLs being present; Warpnet speaks in peer IDs. */
    const val FAKE_BASE_URL: String = "https://warpnet.local"

    fun WarpnetUser.toAccount(): Account = Account(
        id = id,
        localUsername = username,
        username = username,
        displayName = username,
        createdAt = parseDate(createdAt),
        note = bio,
        url = "$FAKE_BASE_URL/users/$id",
        avatar = avatarKey.orEmpty(),
        header = backgroundImageKey,
        locked = false,
        followersCount = followersCount.toInt(),
        followingCount = followingsCount.toInt(),
        statusesCount = tweetsCount.toInt(),
    )

    fun WarpnetUser.toTimelineAccount(): TimelineAccount = TimelineAccount(
        id = id,
        localUsername = username,
        username = username,
        displayName = username,
        url = "$FAKE_BASE_URL/users/$id",
        avatar = avatarKey.orEmpty(),
        staticAvatar = avatarKey.orEmpty(),
        note = bio,
    )

    fun WarpnetTweet.toStatus(author: WarpnetUser?): Status {
        val account = author?.toTimelineAccount() ?: stubTimelineAccount(userId, username)
        return Status(
            id = id,
            url = "$FAKE_BASE_URL/tweets/$id",
            account = account,
            inReplyToId = parentId,
            inReplyToAccountId = null,
            reblog = null,
            content = text,
            createdAt = parseDate(createdAt),
            editedAt = updatedAt?.let(::parseDate),
            emojis = emptyList(),
            reblogsCount = 0,
            favouritesCount = 0,
            repliesCount = 0,
            reblogged = false,
            favourited = false,
            bookmarked = false,
            sensitive = false,
            spoilerText = "",
            visibility = Status.Visibility.PUBLIC,
            attachments = emptyList(),
            mentions = emptyList(),
            quote = null,
        )
    }

    /**
     * Synthesise a [Relationship] from two follow-probe booleans. Warpnet has
     * no notion of blocking / muting / pending requests so those all resolve
     * to `false`.
     */
    fun relationshipFrom(targetUserId: String, following: Boolean, followedBy: Boolean): Relationship =
        Relationship(
            id = targetUserId,
            following = following,
            followedBy = followedBy,
            blocking = false,
            muting = false,
            mutingNotifications = false,
            requested = false,
            showingReblogs = true,
            blockingDomain = false,
        )

    fun WarpnetNotification.toNotification(author: WarpnetUser): Notification = Notification(
        id = id,
        type = notificationTypeFromString(type),
        account = author.toTimelineAccount(),
        status = null,
    )

    private fun parseDate(raw: String): Date =
        if (raw.isEmpty()) Date(0) else runCatching { Date.from(java.time.Instant.parse(raw)) }.getOrElse { Date(0) }

    private fun stubTimelineAccount(userId: String, username: String): TimelineAccount = TimelineAccount(
        id = userId,
        localUsername = username,
        username = username,
        displayName = username,
        url = "$FAKE_BASE_URL/users/$userId",
        avatar = "",
        staticAvatar = "",
        note = "",
    )
}
