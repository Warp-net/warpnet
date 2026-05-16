/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.warpnet

import site.warpnet.warpdroid.entity.Account
import site.warpnet.warpdroid.entity.Conversation
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.Relationship
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.entity.TimelineAccount
import site.warpnet.warpdroid.entity.notificationTypeFromString
import java.util.Date
import site.warpnet.transport.dto.WarpnetChat
import site.warpnet.transport.dto.WarpnetNotification
import site.warpnet.transport.dto.WarpnetTweet
import site.warpnet.transport.dto.WarpnetUser

/**
 * Maps Warpnet wire DTOs onto Warpdroid's pre-existing Warpnet-shaped entities.
 *
 * Most Warpnet concepts have no direct Warpnet equivalent (rich HTML content,
 * media attachment metadata, visibility enums, filter matches, emojis beyond
 * what [Tweet] requires). Where the translation is lossy, this mapper picks
 * the safest default — public visibility, no attachments, empty emoji list —
 * so the rest of the UI keeps working without special-casing the bridged
 * account. See [FAKE_BASE_URL] for how remote-origin URLs are synthesised.
 */
object WarpnetMapper {

    /** Warpnet UIs rely on URLs being present; Warpnet speaks in peer IDs. */
    const val FAKE_BASE_URL: String = "https://warpnet.local"

    fun WarpnetUser.toAccount(): Account = Account(
        id = id,
        // Warpnet has no instance-local handle; the canonical
        // peer-derived user_id is what the desktop frontend prints after
        // the @ sign, so mirror that here for parity.
        localUsername = id,
        username = id,
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
        localUsername = id,
        username = id,
        displayName = username,
        url = "$FAKE_BASE_URL/users/$id",
        avatar = avatarKey.orEmpty(),
        staticAvatar = avatarKey.orEmpty(),
        note = bio,
    )

    fun WarpnetTweet.toTweet(author: WarpnetUser?): Tweet {
        val account = author?.toTimelineAccount() ?: stubTimelineAccount(userId, username)
        return Tweet(
            id = id,
            url = "$FAKE_BASE_URL/tweets/$id",
            account = account,
            inReplyToId = parentId,
            inReplyToAccountId = null,
            retweet = null,
            content = text,
            createdAt = parseDate(createdAt.orEmpty()),
            editedAt = updatedAt?.let(::parseDate),
            emojis = emptyList(),
            retweetsCount = 0,
            likesCount = 0,
            repliesCount = 0,
            viewsCount = 0,
            retweeted = false,
            liked = false,
            bookmarked = false,
            sensitive = false,
            spoilerText = "",
            visibility = Tweet.Visibility.PUBLIC,
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
            showingRetweets = true,
        )

    fun WarpnetNotification.toNotification(author: WarpnetUser): Notification = Notification(
        id = id,
        type = notificationTypeFromString(type),
        account = author.toTimelineAccount(),
        status = null,
    )

    /**
     * Surface a Warpnet 1:1 chat as a [Conversation]. The Warpnet wire
     * carries only the latest message text and the two participants, so
     * [Conversation.lastStatus] is left null; the UI shows the chat row
     * as a single-participant thread with the most recent timestamp
     * from [WarpnetChat.updatedAt].
     */
    fun chatToConversation(chat: WarpnetChat, otherAccount: TimelineAccount): Conversation = Conversation(
        id = chat.id,
        accounts = listOf(otherAccount),
        lastStatus = null,
        unread = false,
    )

    private fun parseDate(raw: String): Date =
        if (raw.isEmpty()) Date(0) else runCatching { Date.from(java.time.Instant.parse(raw)) }.getOrElse { Date(0) }

    private fun stubTimelineAccount(userId: String, username: String): TimelineAccount = TimelineAccount(
        id = userId,
        localUsername = userId,
        username = userId,
        displayName = username,
        url = "$FAKE_BASE_URL/users/$userId",
        avatar = "",
        staticAvatar = "",
        note = "",
    )
}
