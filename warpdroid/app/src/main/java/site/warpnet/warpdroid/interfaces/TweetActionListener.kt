/* Copyright 2017 Andrew Dawson
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
package site.warpnet.warpdroid.interfaces

import at.connyduck.sparkbutton.compose.SparkButtonState
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.viewdata.TweetViewData

interface TweetActionListener : LinkListener {

    /**
     * Retweet the post represented by [viewData]
     * @param retweet true to retweet, false to undo a retweet
     * @param visibility The visibility to use for the retweet, if the user has already chosen it, null otherwise
     * @param state Optional SparkButtonState for delayed animation
     */
    fun onRetweet(
        viewData: TweetViewData.Concrete,
        retweet: Boolean,
        visibility: Tweet.Visibility?,
        state: SparkButtonState?
    )

    /**
     * Like the post represented by [viewData]
     * @param state Optional SparkButtonState to trigger delayed animation
     */
    fun onLike(viewData: TweetViewData.Concrete, like: Boolean, state: SparkButtonState?)

    fun onBookmark(viewData: TweetViewData.Concrete, bookmark: Boolean)

    fun onExpandedChange(viewData: TweetViewData.Concrete, expanded: Boolean)

    fun onContentHiddenChange(viewData: TweetViewData.Concrete, isShowing: Boolean)

    /**
     * Called when the status [android.widget.ToggleButton] responsible for collapsing long
     * status content is interacted with.
     *
     * @param viewData    The status that is being toggled
     * @param isCollapsed Whether the status content is shown in a collapsed state or fully.
     */
    fun onContentCollapsedChange(viewData: TweetViewData.Concrete, isCollapsed: Boolean)

    fun onVoteInPoll(viewData: TweetViewData.Concrete, pollId: String, choices: List<Int>)

    fun onShowPollResults(viewData: TweetViewData.Concrete)

    fun changeFilter(viewData: TweetViewData.Concrete, filtered: Boolean)

    fun onTranslate(viewData: TweetViewData.Concrete)

    fun onUntranslate(viewData: TweetViewData.Concrete)

    fun onBlock(accountId: String)

    fun onMute(accountId: String, hideNotifications: Boolean, duration: Int?)

    fun onMuteConversation(viewData: TweetViewData.Concrete, mute: Boolean)

    fun onDelete(viewData: TweetViewData.Concrete)

    fun onRedraft(viewData: TweetViewData.Concrete)

    fun onPin(viewData: TweetViewData.Concrete, pin: Boolean)

    fun onViewMedia(viewData: TweetViewData.Concrete, attachmentIndex: Int)

    fun onViewThread(viewData: TweetViewData.Concrete)

    fun onEdit(viewData: TweetViewData.Concrete)

    fun onReply(viewData: TweetViewData.Concrete)

    fun onReport(viewData: TweetViewData.Concrete)

    /**
     * Show a quote despite the author being blocked or muted.
     * @param viewData The parent status containing the quote.
     */
    fun onShowQuote(viewData: TweetViewData.Concrete)

    fun removeQuote(viewData: TweetViewData.Concrete)
}
