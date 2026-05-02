/* Copyright 2017 Andrew Dawson
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
package com.keylesspalace.tusky.interfaces

import at.connyduck.sparkbutton.compose.SparkButtonState
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.viewdata.StatusViewData

interface StatusActionListener : LinkListener {

    /**
     * Reblog the post represented by [viewData]
     * @param reblog true to reblog, false to undo a reblog
     * @param visibility The visibility to use for the reblog, if the user has already chosen it, null otherwise
     * @param state Optional SparkButtonState for delayed animation
     */
    fun onReblog(
        viewData: StatusViewData.Concrete,
        reblog: Boolean,
        visibility: Status.Visibility?,
        state: SparkButtonState?
    )

    /**
     * Favourite the post represented by [viewData]
     * @param state Optional SparkButtonState to trigger delayed animation
     */
    fun onFavourite(viewData: StatusViewData.Concrete, favourite: Boolean, state: SparkButtonState?)

    fun onBookmark(viewData: StatusViewData.Concrete, bookmark: Boolean)

    fun onExpandedChange(viewData: StatusViewData.Concrete, expanded: Boolean)

    fun onContentHiddenChange(viewData: StatusViewData.Concrete, isShowing: Boolean)

    /**
     * Called when the status [android.widget.ToggleButton] responsible for collapsing long
     * status content is interacted with.
     *
     * @param viewData    The status that is being toggled
     * @param isCollapsed Whether the status content is shown in a collapsed state or fully.
     */
    fun onContentCollapsedChange(viewData: StatusViewData.Concrete, isCollapsed: Boolean)

    fun onVoteInPoll(viewData: StatusViewData.Concrete, pollId: String, choices: List<Int>)

    fun onShowPollResults(viewData: StatusViewData.Concrete)

    fun changeFilter(viewData: StatusViewData.Concrete, filtered: Boolean)

    fun onTranslate(viewData: StatusViewData.Concrete)

    fun onUntranslate(viewData: StatusViewData.Concrete)

    fun onBlock(accountId: String)

    fun onMute(accountId: String, hideNotifications: Boolean, duration: Int?)

    fun onMuteConversation(viewData: StatusViewData.Concrete, mute: Boolean)

    fun onDelete(viewData: StatusViewData.Concrete)

    fun onRedraft(viewData: StatusViewData.Concrete)

    fun onPin(viewData: StatusViewData.Concrete, pin: Boolean)

    fun onViewMedia(viewData: StatusViewData.Concrete, attachmentIndex: Int)

    fun onViewThread(viewData: StatusViewData.Concrete)

    fun onEdit(viewData: StatusViewData.Concrete)

    fun onReply(viewData: StatusViewData.Concrete)

    fun onReport(viewData: StatusViewData.Concrete)

    /**
     * Show a quote despite the author being blocked or muted.
     * @param viewData The parent status containing the quote.
     */
    fun onShowQuote(viewData: StatusViewData.Concrete)

    fun removeQuote(viewData: StatusViewData.Concrete)
}
