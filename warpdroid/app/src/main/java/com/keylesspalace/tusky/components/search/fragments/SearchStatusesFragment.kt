/* Copyright 2021 Tusky Contributors
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

package com.keylesspalace.tusky.components.search.fragments

import android.content.SharedPreferences
import android.os.Bundle
import android.view.View
import androidx.compose.foundation.lazy.LazyListScope
import androidx.lifecycle.lifecycleScope
import androidx.paging.PagingData
import androidx.paging.compose.LazyPagingItems
import at.connyduck.calladapter.networkresult.onFailure
import at.connyduck.sparkbutton.compose.SparkButtonState
import com.google.android.material.snackbar.Snackbar
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.components.compose.ComposeActivity
import com.keylesspalace.tusky.components.instanceinfo.InstanceInfo
import com.keylesspalace.tusky.db.entity.AccountEntity
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.statuscomponents.Status
import com.keylesspalace.tusky.util.reply
import com.keylesspalace.tusky.util.report
import com.keylesspalace.tusky.util.startActivityWithSlideInAnimation
import com.keylesspalace.tusky.util.viewMedia
import com.keylesspalace.tusky.view.ConfirmationBottomSheet.Companion.confirmFavourite
import com.keylesspalace.tusky.view.ConfirmationBottomSheet.Companion.confirmReblog
import com.keylesspalace.tusky.viewdata.AttachmentViewData
import com.keylesspalace.tusky.viewdata.StatusViewData
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.launch

@AndroidEntryPoint
class SearchStatusesFragment :
    SearchFragment<StatusViewData.Concrete>(),
    StatusActionListener {

    @Inject
    lateinit var preferences: SharedPreferences

    override val data: Flow<PagingData<StatusViewData.Concrete>>
        get() = viewModel.statusesFlow

    override fun LazyListScope.searchResult(
        result: LazyPagingItems<StatusViewData.Concrete>,
        instanceInfo: InstanceInfo,
        accounts: List<AccountEntity>
    ) {
        items(
            count = result.itemCount,
            // We cannot use ids as keys because in rare cases search result pages can include posts already found in previous pages.
            // key = result.itemKey { viewData -> viewData.id },
            itemContent = { index ->
                result[index]?.let { viewData ->
                    Status(
                        viewData,
                        this@SearchStatusesFragment,
                        translationEnabled = instanceInfo.translationEnabled,
                        accounts = accounts
                    )
                }
            }
        )
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)
        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.startComposing.collect { composeOptions ->
                val intent = ComposeActivity.newIntent(requireContext(), composeOptions)
                requireContext().startActivityWithSlideInAnimation(intent)
            }
        }
    }

    override fun onReblog(
        viewData: StatusViewData.Concrete,
        reblog: Boolean,
        visibility: Status.Visibility?,
        state: SparkButtonState?
    ) {
        if (reblog && visibility == null) {
            confirmReblog(preferences) { visibility ->
                viewModel.reblog(viewData.id, true, visibility)
                state?.animate()
            }
        } else {
            viewModel.reblog(viewData.id, reblog, visibility ?: Status.Visibility.PUBLIC)
            if (reblog) {
                state?.animate()
            }
        }
    }

    override fun onFavourite(
        viewData: StatusViewData.Concrete,
        favourite: Boolean,
        state: SparkButtonState?
    ) {
        if (favourite) {
            confirmFavourite(preferences) {
                viewModel.favorite(viewData.id, true)
                state?.animate()
            }
        } else {
            viewModel.favorite(viewData.id, false)
        }
    }

    override fun onBookmark(viewData: StatusViewData.Concrete, bookmark: Boolean) {
        viewModel.bookmark(viewData.id, bookmark)
    }

    override fun onExpandedChange(viewData: StatusViewData.Concrete, expanded: Boolean) {
        viewModel.expandedChange(viewData, expanded)
    }

    override fun onContentHiddenChange(viewData: StatusViewData.Concrete, isShowing: Boolean) {
        viewModel.contentHiddenChange(viewData, isShowing)
    }

    override fun onContentCollapsedChange(viewData: StatusViewData.Concrete, isCollapsed: Boolean) {
        viewModel.collapsedChange(viewData, isCollapsed)
    }

    override fun onVoteInPoll(viewData: StatusViewData.Concrete, pollId: String, choices: List<Int>) {
        viewModel.voteInPoll(viewData.id, pollId, choices)
    }

    override fun onShowPollResults(viewData: StatusViewData.Concrete) {
        viewModel.showPollResults(viewData)
    }

    override fun onTranslate(viewData: StatusViewData.Concrete) {
        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.translate(viewData)
                .onFailure {
                    Snackbar.make(
                        requireView(),
                        getString(R.string.ui_error_translate, it.message),
                        Snackbar.LENGTH_LONG
                    ).show()
                }
        }
    }

    override fun onUntranslate(viewData: StatusViewData.Concrete) {
        viewModel.untranslate(viewData)
    }

    override fun changeFilter(viewData: StatusViewData.Concrete, filtered: Boolean) {
        viewModel.changeFilter(filtered, viewData)
    }

    override fun onBlock(accountId: String) {
        viewModel.block(accountId)
    }

    override fun onMute(accountId: String, hideNotifications: Boolean, duration: Int?) {
        viewModel.mute(accountId, hideNotifications, duration)
    }

    override fun onMuteConversation(viewData: StatusViewData.Concrete, mute: Boolean) {
        viewModel.muteConversation(viewData.id, !viewData.status.muted)
    }

    override fun onDelete(viewData: StatusViewData.Concrete) {
        viewModel.delete(viewData.id)
    }

    override fun onRedraft(viewData: StatusViewData.Concrete) {
        viewModel.redraftStatus(viewData.status)
    }

    override fun onPin(viewData: StatusViewData.Concrete, pin: Boolean) {
        viewModel.pin(viewData.id, pin)
    }

    override fun onViewMedia(viewData: StatusViewData.Concrete, attachmentIndex: Int) {
        requireContext().viewMedia(
            attachmentIndex,
            AttachmentViewData.list(viewData),
        )
    }

    override fun onViewThread(viewData: StatusViewData.Concrete) {
        bottomSheetActivity?.viewThread(viewData.id, viewData.status.url)
    }

    override fun onEdit(viewData: StatusViewData.Concrete) {
        viewModel.editStatus(viewData.status)
    }

    override fun onReply(viewData: StatusViewData.Concrete) {
        requireContext().reply(viewData, viewModel.activeAccount!!)
    }

    override fun onReport(viewData: StatusViewData.Concrete) {
        requireContext().report(viewData)
    }

    override fun onShowQuote(viewData: StatusViewData.Concrete) {
        viewModel.showQuote(viewData)
    }

    override fun removeQuote(viewData: StatusViewData.Concrete) {
        viewModel.removeQuote(viewData.status)
    }

    companion object {
        fun newInstance() = SearchStatusesFragment()
    }
}
