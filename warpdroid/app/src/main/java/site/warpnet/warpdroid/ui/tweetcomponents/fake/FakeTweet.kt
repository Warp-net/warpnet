/* Copyright 2025 Warpdroid Contributors
 *
 * This file is a part of Warpdroid.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Warpdroid is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Warpdroid; if not,
 * see <http://www.gnu.org/licenses>. */

package site.warpnet.warpdroid.ui.tweetcomponents.fake

import at.connyduck.sparkbutton.compose.SparkButtonState
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.entity.HashTag
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.entity.TimelineAccount
import site.warpnet.warpdroid.interfaces.TweetActionListener
import site.warpnet.warpdroid.viewdata.TweetViewData
import java.util.Date

fun fakeTweetViewData(
    account: TimelineAccount = fakeTimelineAccount,
    content: String = "<p>This is a status </p><p><a href=\"https://mastodon.social/tags/nokings\" class=\"mention hashtag\" rel=\"tag\">#<span>hashtag</span></a></p>",
    attachments: List<Attachment> = fourAttachments,
    mentions: List<Tweet.Mention> = emptyList(),
    tags: List<HashTag> = listOf(HashTag("hashtag", "https://example.org/tags/hashtag")),
) = TweetViewData.Concrete(
    status = Tweet(
        id = "456",
        url = "https://example.org/456",
        account = account,
        inReplyToId = null,
        inReplyToAccountId = null,
        retweet = null,
        content = content,
        createdAt = Date(),
        editedAt = null,
        emojis = emptyList(),
        retweetsCount = 2,
        likesCount = 3,
        repliesCount = 4,
        quotesCount = 1,
        retweeted = false,
        liked = false,
        bookmarked = false,
        sensitive = false,
        spoilerText = "",
        visibility = Tweet.Visibility.PUBLIC,
        attachments = attachments,
        mentions = mentions,
        tags = tags,
        application = Tweet.Application(
            name = "Warpdroid",
            website = "https://tusky.app"
        ),
        pinned = false,
        muted = false,
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

val noopListener = object : TweetActionListener {
    override fun onRetweet(viewData: TweetViewData.Concrete, retweet: Boolean, visibility: Tweet.Visibility?, state: SparkButtonState?) {}
    override fun onLike(viewData: TweetViewData.Concrete, like: Boolean, state: SparkButtonState?) { }
    override fun onBookmark(viewData: TweetViewData.Concrete, bookmark: Boolean) { }
    override fun onViewMedia(viewData: TweetViewData.Concrete, attachmentIndex: Int) { }
    override fun onViewThread(viewData: TweetViewData.Concrete) { }
    override fun onExpandedChange(viewData: TweetViewData.Concrete, expanded: Boolean) { }
    override fun onContentHiddenChange(viewData: TweetViewData.Concrete, isShowing: Boolean) { }
    override fun onContentCollapsedChange(viewData: TweetViewData.Concrete, isCollapsed: Boolean) { }
    override fun changeFilter(viewData: TweetViewData.Concrete, filtered: Boolean) { }
    override fun onBlock(accountId: String) { }
    override fun onMute(accountId: String, hideNotifications: Boolean, duration: Int?) { }
    override fun onMuteConversation(viewData: TweetViewData.Concrete, mute: Boolean) { }
    override fun onEdit(viewData: TweetViewData.Concrete) { }
    override fun onDelete(viewData: TweetViewData.Concrete) { }
    override fun onRedraft(viewData: TweetViewData.Concrete) { }
    override fun onPin(viewData: TweetViewData.Concrete, pin: Boolean) { }
    override fun onViewTag(tag: String) { }
    override fun onViewAccount(accountId: String) { }
    override fun onViewUrl(url: String) { }
    override fun onReply(viewData: TweetViewData.Concrete) { }
    override fun onReport(viewData: TweetViewData.Concrete) { }
    override fun onShowQuote(viewData: TweetViewData.Concrete) { }
    override fun removeQuote(viewData: TweetViewData.Concrete) { }
}
