/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.warpnet

import site.warpnet.warpdroid.entity.AccountSource
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.entity.User
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.Poll
import site.warpnet.warpdroid.entity.PollOption
import site.warpnet.warpdroid.entity.Relationship
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.entity.TimelineUser
import site.warpnet.warpdroid.entity.notificationTypeFromString
import java.util.Date
import site.warpnet.transport.dto.WarpnetNotification
import site.warpnet.transport.dto.WarpnetPoll
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

    /** Warpnet UIs rely on URLs being present; Warpnet speaks in peer IDs.
     *  Use the canonical project domain so synthesised share URLs point
     *  at the real Warpnet web entry point. */
    const val FAKE_BASE_URL: String = "https://warpnet.site"

    fun WarpnetUser.toAccount(): User = User(
        id = id,
        // Warpnet has no instance-local handle; the canonical
        // peer-derived user_id is what the desktop frontend prints after
        // the @ sign, so mirror that here for parity.
        localUsername = id,
        username = id,
        displayName = username,
        createdAt = parseDate(createdAt),
        note = bio,
        // Warpnet bios are plain text, so mirror [note] into [source.note];
        // Edit Profile prefills its Bio field from source.note (the editable
        // plaintext), which would otherwise be null and silently blank it.
        source = AccountSource(note = bio),
        url = "$FAKE_BASE_URL/users/$id",
        avatar = warpnetImageUrl(id, avatarKey),
        header = warpnetImageUrl(id, backgroundImageKey),
        locked = locked,
        followersCount = followersCount.toInt(),
        followingCount = followingsCount.toInt(),
        statusesCount = tweetsCount.toInt(),
        network = network,
        nodeId = nodeId,
    )

    fun WarpnetUser.toTimelineUser(): TimelineUser = TimelineUser(
        id = id,
        localUsername = id,
        username = id,
        displayName = username,
        url = "$FAKE_BASE_URL/users/$id",
        avatar = warpnetImageUrl(id, avatarKey),
        staticAvatar = warpnetImageUrl(id, avatarKey),
        note = bio,
        network = network,
    )

    /**
     * Build the synthetic URL that [WarpnetAvatarLoader] recognises and
     * routes through `PUBLIC_GET_IMAGE`. Empty key → empty string, which
     * the existing `loadAvatar()` helpers treat as "no avatar" and fall
     * back to the default drawable without ever hitting Glide.
     */
    fun warpnetImageUrl(userId: String, key: String?): String =
        if (userId.isBlank() || key.isNullOrBlank()) ""
        else "warpnet://avatar/$userId/$key"

    /**
     * Video counterpart of [warpnetImageUrl]. Resolved by
     * [site.warpnet.warpdroid.util.WarpnetVideoDataSource] rather than
     * Glide, so it carries its own scheme prefix — a video blob is fetched
     * over `PUBLIC_GET_VIDEO`, not `PUBLIC_GET_IMAGE`.
     */
    fun warpnetVideoUrl(userId: String, key: String?): String =
        if (userId.isBlank() || key.isNullOrBlank()) ""
        else "warpnet://video/$userId/$key"

    /**
     * Warpnet stores attachments as content-addressed blob keys on the tweet
     * itself — up to four images plus one video — with no per-attachment
     * record, so the [Attachment] rows are synthesised here. The key doubles
     * as the attachment id: it is a content hash, so it is stable and unique
     * within the tweet.
     */
    private fun WarpnetTweet.toAttachments(): List<Attachment> {
        val images = imageKeys.orEmpty().filter { it.isNotBlank() }.map { key ->
            Attachment(
                id = key,
                url = warpnetImageUrl(userId, key),
                previewUrl = warpnetImageUrl(userId, key),
                type = Attachment.Type.IMAGE,
            )
        }
        val video = videoKey?.takeIf { it.isNotBlank() }?.let { key ->
            Attachment(
                id = key,
                url = warpnetVideoUrl(userId, key),
                // A Warpnet video carries no separate poster frame; the
                // player renders its own first frame once it opens.
                previewUrl = null,
                type = Attachment.Type.VIDEO,
            )
        }
        return if (video == null) images else images + video
    }

    /**
     * Map the poll carried by the tweet. Tallies are not on the wire here —
     * they are a separate read — so the counts start at zero and
     * [site.warpnet.warpdroid.warpnet.WarpnetRepository] folds in the real
     * ones before the tweet reaches the UI.
     */
    private fun WarpnetPoll.toPoll(): Poll = Poll(
        options = options.map { PollOption(title = it) },
        expiresAt = expiresAt.takeIf { it.isNotEmpty() }?.let(::parseDate),
    )

    fun WarpnetTweet.toTweet(author: WarpnetUser?): Tweet {
        val account = author?.toTimelineUser() ?: stubTimelineUser(userId, username)
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
            reactionsCount = 0,
            repliesCount = 0,
            viewsCount = 0,
            retweeted = false,
            reacted = false,
            bookmarked = false,
            sensitive = false,
            spoilerText = "",
            visibility = Tweet.Visibility.PUBLIC,
            attachments = toAttachments(),
            mentions = emptyList(),
            quote = null,
            poll = poll?.toPoll(),
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

    // The wire shape (domain.Notification) embeds the actor in [text]
    // ("Alice reacted your tweet") and exposes only the recipient's user_id,
    // so the UI gets a stub account; the visible content is text + type.
    fun WarpnetNotification.toNotification(): Notification = Notification(
        id = id,
        type = notificationTypeFromString(type),
        account = stubTimelineUser(userId, text),
        status = null,
    )

    private fun parseDate(raw: String): Date =
        if (raw.isEmpty()) Date(0) else runCatching { Date.from(java.time.Instant.parse(raw)) }.getOrElse { Date(0) }

    private fun stubTimelineUser(userId: String, username: String): TimelineUser = TimelineUser(
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
