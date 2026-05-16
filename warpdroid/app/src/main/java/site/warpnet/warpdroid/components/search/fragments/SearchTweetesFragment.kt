/* Copyright 2021 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.search.fragments

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
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.compose.ComposeActivity
import site.warpnet.warpdroid.components.instanceinfo.InstanceInfo
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.interfaces.TweetActionListener
import site.warpnet.warpdroid.ui.tweetcomponents.TweetCard
import site.warpnet.warpdroid.util.reply
import site.warpnet.warpdroid.util.report
import site.warpnet.warpdroid.util.startActivityWithSlideInAnimation
import site.warpnet.warpdroid.util.viewMedia
import site.warpnet.warpdroid.view.ConfirmationBottomSheet.Companion.confirmLike
import site.warpnet.warpdroid.view.ConfirmationBottomSheet.Companion.confirmRetweet
import site.warpnet.warpdroid.viewdata.AttachmentViewData
import site.warpnet.warpdroid.viewdata.TweetViewData
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.launch

@AndroidEntryPoint
class SearchStatusesFragment :
    SearchFragment<TweetViewData.Concrete>(),
    TweetActionListener {

    @Inject
    lateinit var preferences: SharedPreferences

    override val data: Flow<PagingData<TweetViewData.Concrete>>
        get() = viewModel.statusesFlow

    override fun LazyListScope.searchResult(
        result: LazyPagingItems<TweetViewData.Concrete>,
        instanceInfo: InstanceInfo,
        accounts: List<AccountEntity>
    ) {
        items(
            count = result.itemCount,
            // We cannot use ids as keys because in rare cases search result pages can include posts already found in previous pages.
            // key = result.itemKey { viewData -> viewData.id },
            itemContent = { index ->
                result[index]?.let { viewData ->
                    TweetCard(
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

    override fun onRetweet(
        viewData: TweetViewData.Concrete,
        retweet: Boolean,
        visibility: Tweet.Visibility?,
        state: SparkButtonState?
    ) {
        if (retweet && visibility == null) {
            confirmRetweet(preferences) { visibility ->
                viewModel.retweet(viewData.id, true, visibility)
                state?.animate()
            }
        } else {
            viewModel.retweet(viewData.id, retweet, visibility ?: Tweet.Visibility.PUBLIC)
            if (retweet) {
                state?.animate()
            }
        }
    }

    override fun onLike(
        viewData: TweetViewData.Concrete,
        like: Boolean,
        state: SparkButtonState?
    ) {
        if (like) {
            confirmLike(preferences) {
                viewModel.like(viewData.id, viewData.accountId, true)
                state?.animate()
            }
        } else {
            viewModel.like(viewData.id, viewData.accountId, false)
        }
    }

    override fun onBookmark(viewData: TweetViewData.Concrete, bookmark: Boolean) {
        viewModel.bookmark(viewData.id, viewData.status.account.id, bookmark)
    }

    override fun onExpandedChange(viewData: TweetViewData.Concrete, expanded: Boolean) {
        viewModel.expandedChange(viewData, expanded)
    }

    override fun onContentHiddenChange(viewData: TweetViewData.Concrete, isShowing: Boolean) {
        viewModel.contentHiddenChange(viewData, isShowing)
    }

    override fun onContentCollapsedChange(viewData: TweetViewData.Concrete, isCollapsed: Boolean) {
        viewModel.collapsedChange(viewData, isCollapsed)
    }

    override fun onTranslate(viewData: TweetViewData.Concrete) {
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

    override fun onUntranslate(viewData: TweetViewData.Concrete) {
        viewModel.untranslate(viewData)
    }

    override fun changeFilter(viewData: TweetViewData.Concrete, filtered: Boolean) {
        viewModel.changeFilter(filtered, viewData)
    }

    override fun onBlock(accountId: String) {
        viewModel.block(accountId)
    }

    override fun onMute(accountId: String, hideNotifications: Boolean, duration: Int?) {
        viewModel.mute(accountId, hideNotifications, duration)
    }

    override fun onMuteConversation(viewData: TweetViewData.Concrete, mute: Boolean) {
        viewModel.muteConversation(viewData.id, !viewData.status.muted)
    }

    override fun onDelete(viewData: TweetViewData.Concrete) {
        viewModel.delete(viewData.id)
    }

    override fun onRedraft(viewData: TweetViewData.Concrete) {
        viewModel.redraftStatus(viewData.status)
    }

    override fun onPin(viewData: TweetViewData.Concrete, pin: Boolean) {
        viewModel.pin(viewData.id, pin)
    }

    override fun onViewMedia(viewData: TweetViewData.Concrete, attachmentIndex: Int) {
        requireContext().viewMedia(
            attachmentIndex,
            AttachmentViewData.list(viewData),
        )
    }

    override fun onViewThread(viewData: TweetViewData.Concrete) {
        bottomSheetActivity?.viewThread(viewData.id, viewData.status.url, viewData.status.account.id)
    }

    override fun onEdit(viewData: TweetViewData.Concrete) {
        viewModel.editStatus(viewData.status)
    }

    override fun onReply(viewData: TweetViewData.Concrete) {
        requireContext().reply(viewData, viewModel.activeAccount!!)
    }

    override fun onReport(viewData: TweetViewData.Concrete) {
        requireContext().report(viewData)
    }

    override fun onShowQuote(viewData: TweetViewData.Concrete) {
        viewModel.showQuote(viewData)
    }

    override fun removeQuote(viewData: TweetViewData.Concrete) {
        viewModel.removeQuote(viewData.status)
    }

    companion object {
        fun newInstance() = SearchStatusesFragment()
    }
}
