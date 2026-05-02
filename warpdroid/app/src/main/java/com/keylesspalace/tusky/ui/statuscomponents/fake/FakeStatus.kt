/* Copyright 2025 Tusky Contributors
 *
 * This file is a part of Tusky.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Tusky is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Tusky; if not,
 * see <http://www.gnu.org/licenses>. */

package com.keylesspalace.tusky.ui.statuscomponents.fake

import at.connyduck.sparkbutton.compose.SparkButtonState
import com.keylesspalace.tusky.entity.Attachment
import com.keylesspalace.tusky.entity.HashTag
import com.keylesspalace.tusky.entity.Poll
import com.keylesspalace.tusky.entity.PollOption
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.entity.TimelineAccount
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.viewdata.StatusViewData
import java.util.Date

fun fakeStatusViewData(
    account: TimelineAccount = fakeTimelineAccount,
    content: String = "<p>This is a status </p><p><a href=\"https://mastodon.social/tags/nokings\" class=\"mention hashtag\" rel=\"tag\">#<span>hashtag</span></a></p>",
    attachments: List<Attachment> = fourAttachments,
    mentions: List<Status.Mention> = emptyList(),
    tags: List<HashTag> = listOf(HashTag("hashtag", "https://example.org/tags/hashtag")),
    poll: Poll = pollWithFourOptions
) = StatusViewData.Concrete(
    status = Status(
        id = "456",
        url = "https://example.org/456",
        account = account,
        inReplyToId = null,
        inReplyToAccountId = null,
        reblog = null,
        content = content,
        createdAt = Date(),
        editedAt = null,
        emojis = emptyList(),
        reblogsCount = 2,
        favouritesCount = 3,
        repliesCount = 4,
        quotesCount = 1,
        reblogged = false,
        favourited = false,
        bookmarked = false,
        sensitive = false,
        spoilerText = "",
        visibility = Status.Visibility.PUBLIC,
        attachments = attachments,
        mentions = mentions,
        tags = tags,
        application = Status.Application(
            name = "Tusky",
            website = "https://tusky.app"
        ),
        pinned = false,
        muted = false,
        poll = poll,
        card = null,
        language = "en",
        filtered = null,
        quote = null
    ),
    isExpanded = true,
    isShowingContent = true,
    isCollapsed = false,
    filterActive = false,
    quote = null
)

val fakeTimelineAccount = TimelineAccount(
    id = "01HAWDAG8VXPW2D6PZFA3MQ3FH",
    localUsername = "connyduck",
    username = "connyduck@fosspri.de",
    displayName = "Conny Duck Test",
    bot = true,
    url = "https://goblin.technology/@connyduck",
    avatar = "https://goblin.technology/assets/default_avatars/GoToSocial_icon4.webp",
    staticAvatar = "https://goblin.technology/assets/default_avatars/GoToSocial_icon4.webp",
    note = "",
    emojis = emptyList()
)

val fourAttachments = listOf(
    Attachment(
        id = "1",
        url = "https://example.com/1",
        previewUrl = "https://example.com/preview/1",
        type = Attachment.Type.IMAGE,
        description = "description 1",
        blurhash = "U8EC~$^REK-:~Tt6Rjxv9FWYxbRlr{s=oxMz"
    ),
    Attachment(
        id = "2",
        url = "https://example.com/2",
        previewUrl = null,
        meta = Attachment.MetaData(
            original = Attachment.Size(
                duration = 123f
            )
        ),
        type = Attachment.Type.AUDIO,
        description = "description 2",
        blurhash = "U~Kd[4p#_%mN%MkQv~j1fQfQfQfQ%MkQv~j1"
    ),
    Attachment(
        id = "3",
        url = "https://example.com/3",
        previewUrl = "https://example.com/preview/3",
        type = Attachment.Type.IMAGE,
        meta = Attachment.MetaData(
            original = Attachment.Size(
                width = 3527,
                height = 2351,
                aspect = 1.5002127f
            )
        ),
        description = null,
        blurhash = $$"UDEfoh0L.3xupB%Kt6oIxmxZRWR$t2t6R-n*"
    ),
    Attachment(
        id = "4",
        url = "https://example.com/4",
        previewUrl = "https://example.com/preview/4",
        type = Attachment.Type.IMAGE,
        meta = Attachment.MetaData(
            original = Attachment.Size(
                width = 2352,
                height = 3527,
                aspect = 0.6668557f
            )
        ),
        description = "description 4",
        blurhash = "UBB#%BxsI:%GxtWFj?WE0gf%-UIuNZV[tMbY"
    )
)

val pollWithFourOptions = Poll(
    id = "1",
    expired = true,
    expiresAt = Date(1761416541000),
    multiple = false,
    votesCount = 120,
    options = listOf(
        PollOption(
            title = "Option 1",
            votesCount = 12
        ),
        PollOption(
            title = "Option 2 with a super long title to see how the interface handles it",
            votesCount = 60
        ),
        PollOption(
            title = "Option 3",
            votesCount = 100
        ),
        PollOption(
            title = "Option 4",
            votesCount = 3
        ),
    ),
    ownVotes = listOf(2)
)

val noopListener = object : StatusActionListener {
    override fun onReblog(viewData: StatusViewData.Concrete, reblog: Boolean, visibility: Status.Visibility?, state: SparkButtonState?) {}
    override fun onFavourite(viewData: StatusViewData.Concrete, favourite: Boolean, state: SparkButtonState?) { }
    override fun onBookmark(viewData: StatusViewData.Concrete, bookmark: Boolean) { }
    override fun onViewMedia(viewData: StatusViewData.Concrete, attachmentIndex: Int) { }
    override fun onViewThread(viewData: StatusViewData.Concrete) { }
    override fun onExpandedChange(viewData: StatusViewData.Concrete, expanded: Boolean) { }
    override fun onContentHiddenChange(viewData: StatusViewData.Concrete, isShowing: Boolean) { }
    override fun onContentCollapsedChange(viewData: StatusViewData.Concrete, isCollapsed: Boolean) { }
    override fun onVoteInPoll(viewData: StatusViewData.Concrete, pollId: String, choices: List<Int>) { }
    override fun onShowPollResults(viewData: StatusViewData.Concrete) { }
    override fun changeFilter(viewData: StatusViewData.Concrete, filtered: Boolean) { }
    override fun onTranslate(viewData: StatusViewData.Concrete) { }
    override fun onUntranslate(viewData: StatusViewData.Concrete) { }
    override fun onBlock(accountId: String) { }
    override fun onMute(accountId: String, hideNotifications: Boolean, duration: Int?) { }
    override fun onMuteConversation(viewData: StatusViewData.Concrete, mute: Boolean) { }
    override fun onEdit(viewData: StatusViewData.Concrete) { }
    override fun onDelete(viewData: StatusViewData.Concrete) { }
    override fun onRedraft(viewData: StatusViewData.Concrete) { }
    override fun onPin(viewData: StatusViewData.Concrete, pin: Boolean) { }
    override fun onViewTag(tag: String) { }
    override fun onViewAccount(accountId: String) { }
    override fun onViewUrl(url: String) { }
    override fun onReply(viewData: StatusViewData.Concrete) { }
    override fun onReport(viewData: StatusViewData.Concrete) { }
    override fun onShowQuote(viewData: StatusViewData.Concrete) { }
    override fun removeQuote(viewData: StatusViewData.Concrete) { }
}
