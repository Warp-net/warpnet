/* Copyright 2024 Tusky Contributors
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

package com.keylesspalace.tusky.components.notifications.requests.details

import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import android.os.Bundle
import android.util.Log
import android.view.View
import androidx.fragment.app.Fragment
import androidx.fragment.app.activityViewModels
import androidx.lifecycle.lifecycleScope
import androidx.paging.LoadState
import androidx.recyclerview.widget.DividerItemDecoration
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.SimpleItemAnimator
import at.connyduck.calladapter.networkresult.onFailure
import at.connyduck.sparkbutton.compose.SparkButtonState
import com.google.android.material.snackbar.BaseTransientBottomBar.LENGTH_LONG
import com.google.android.material.snackbar.Snackbar
import com.keylesspalace.tusky.BottomSheetActivity
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.components.compose.ComposeActivity
import com.keylesspalace.tusky.components.instanceinfo.InstanceInfoRepository
import com.keylesspalace.tusky.components.notifications.NotificationActionListener
import com.keylesspalace.tusky.components.notifications.NotificationsPagingAdapter
import com.keylesspalace.tusky.databinding.FragmentNotificationRequestDetailsBinding
import com.keylesspalace.tusky.db.AccountManager
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.interfaces.AccountActionListener
import com.keylesspalace.tusky.interfaces.LoadMoreActionListener
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.settings.PrefKeys
import com.keylesspalace.tusky.util.CardViewMode
import com.keylesspalace.tusky.util.StatusDisplayOptions
import com.keylesspalace.tusky.util.getErrorString
import com.keylesspalace.tusky.util.hide
import com.keylesspalace.tusky.util.openLink
import com.keylesspalace.tusky.util.reply
import com.keylesspalace.tusky.util.report
import com.keylesspalace.tusky.util.show
import com.keylesspalace.tusky.util.startActivityWithSlideInAnimation
import com.keylesspalace.tusky.util.viewAccount
import com.keylesspalace.tusky.util.viewBinding
import com.keylesspalace.tusky.util.viewMedia
import com.keylesspalace.tusky.util.viewTag
import com.keylesspalace.tusky.util.viewThread
import com.keylesspalace.tusky.util.visible
import com.keylesspalace.tusky.view.ConfirmationBottomSheet.Companion.confirmFavourite
import com.keylesspalace.tusky.view.ConfirmationBottomSheet.Companion.confirmReblog
import com.keylesspalace.tusky.viewdata.AttachmentViewData
import com.keylesspalace.tusky.viewdata.NotificationViewData
import com.keylesspalace.tusky.viewdata.StatusViewData
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch

@AndroidEntryPoint
class NotificationRequestDetailsFragment :
    Fragment(R.layout.fragment_notification_request_details),
    StatusActionListener,
    LoadMoreActionListener<NotificationViewData.LoadMore>,
    NotificationActionListener,
    AccountActionListener {

    @Inject
    lateinit var preferences: SharedPreferences

    @Inject
    lateinit var accountManager: AccountManager

    @Inject
    lateinit var instanceInfoRepository: InstanceInfoRepository

    private val viewModel: NotificationRequestDetailsViewModel by activityViewModels()

    private val binding by viewBinding(FragmentNotificationRequestDetailsBinding::bind)

    private var adapter: NotificationsPagingAdapter? = null

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)

        setupAdapter().let { adapter ->
            this.adapter = adapter
            setupRecyclerView(adapter)

            lifecycleScope.launch {
                viewModel.pager.collectLatest { pagingData ->
                    adapter.submitData(pagingData)
                }
            }
        }

        lifecycleScope.launch {
            viewModel.error.collect { error ->
                Snackbar.make(
                    binding.root,
                    error.getErrorString(requireContext()),
                    LENGTH_LONG
                ).show()
            }
        }

        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.startComposing.collect { composeOptions ->
                val intent = ComposeActivity.newIntent(requireContext(), composeOptions)
                requireContext().startActivityWithSlideInAnimation(intent)
            }
        }
    }

    private fun setupRecyclerView(adapter: NotificationsPagingAdapter) {
        binding.recyclerView.adapter = adapter
        binding.recyclerView.setHasFixedSize(true)
        binding.recyclerView.layoutManager = LinearLayoutManager(requireContext())
        binding.recyclerView.addItemDecoration(
            DividerItemDecoration(requireContext(), DividerItemDecoration.VERTICAL)
        )
        (binding.recyclerView.itemAnimator as SimpleItemAnimator).supportsChangeAnimations = false
    }

    private fun setupAdapter(): NotificationsPagingAdapter {
        val activeAccount = accountManager.activeAccount!!
        val statusDisplayOptions = StatusDisplayOptions(
            animateAvatars = preferences.getBoolean(PrefKeys.ANIMATE_GIF_AVATARS, false),
            mediaPreviewEnabled = activeAccount.mediaPreviewEnabled,
            useAbsoluteTime = preferences.getBoolean(PrefKeys.ABSOLUTE_TIME_VIEW, false),
            showBotOverlay = preferences.getBoolean(PrefKeys.SHOW_BOT_OVERLAY, true),
            useBlurhash = preferences.getBoolean(PrefKeys.USE_BLURHASH, true),
            cardViewMode = if (preferences.getBoolean(PrefKeys.SHOW_CARDS_IN_TIMELINES, false)) {
                CardViewMode.INDENTED
            } else {
                CardViewMode.NONE
            },
            hideStats = preferences.getBoolean(PrefKeys.WELLBEING_HIDE_STATS_POSTS, false),
            animateEmojis = preferences.getBoolean(PrefKeys.ANIMATE_CUSTOM_EMOJIS, false),
            showStatsInline = preferences.getBoolean(PrefKeys.SHOW_STATS_INLINE, false),
            showSensitiveMedia = activeAccount.alwaysShowSensitiveMedia,
            openSpoiler = activeAccount.alwaysOpenSpoiler
        )

        return NotificationsPagingAdapter(
            statusDisplayOptions = statusDisplayOptions,
            statusListener = this,
            loadMoreListener = this,
            notificationActionListener = this,
            accountActionListener = this,
            instanceName = activeAccount.domain,
            accountManager = accountManager
        ).apply {
            addLoadStateListener { loadState ->
                binding.progressBar.visible(
                    loadState.refresh == LoadState.Loading && itemCount == 0
                )

                if (loadState.refresh is LoadState.Error) {
                    binding.recyclerView.hide()
                    binding.statusView.show()
                    val errorState = loadState.refresh as LoadState.Error
                    binding.statusView.setup(errorState.error) { retry() }
                    Log.w(TAG, "error loading notifications for user ${viewModel.accountId}", errorState.error)
                } else {
                    binding.recyclerView.show()
                    binding.statusView.hide()
                }
            }
        }
    }

    override fun onLoadMore(loadMore: NotificationViewData.LoadMore) {
        // not relevant here
    }

    override fun onReblog(
        viewData: StatusViewData.Concrete,
        reblog: Boolean,
        visibility: Status.Visibility?,
        state: SparkButtonState?
    ) {
        if (reblog && visibility == null) {
            confirmReblog(preferences) { visibility ->
                viewModel.reblog(viewData.id, reblog, visibility)
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
        viewModel.changeExpanded(expanded, viewData)
    }

    override fun onContentHiddenChange(viewData: StatusViewData.Concrete, isShowing: Boolean) {
        viewModel.changeContentShowing(isShowing, viewData)
    }

    override fun onContentCollapsedChange(viewData: StatusViewData.Concrete, isCollapsed: Boolean) {
        viewModel.changeContentCollapsed(isCollapsed, viewData)
    }

    override fun onVoteInPoll(viewData: StatusViewData.Concrete, pollId: String, choices: List<Int>) {
        viewModel.voteInPoll(viewData.id, pollId, choices)
    }

    override fun onShowPollResults(viewData: StatusViewData.Concrete) {
        viewModel.showPollResults(viewData)
    }

    override fun changeFilter(viewData: StatusViewData.Concrete, filtered: Boolean) {
        viewModel.changeFilter(filtered, viewData)
    }

    override fun onTranslate(viewData: StatusViewData.Concrete) {
        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.translate(viewData)
                .onFailure {
                    Snackbar.make(
                        requireView(),
                        getString(R.string.ui_error_translate, it.message),
                        LENGTH_LONG
                    ).show()
                }
        }
    }

    override fun onUntranslate(viewData: StatusViewData.Concrete) {
        viewModel.untranslate(viewData)
    }

    override fun onBlock(block: Boolean, accountId: String, position: Int) {
        viewModel.block(accountId)
    }

    override fun onMute(mute: Boolean, accountId: String, position: Int, notifications: Boolean) {
        viewModel.mute(accountId, notifications, null)
    }

    override fun onBlock(accountId: String) {
        viewModel.block(accountId)
    }

    override fun onMute(accountId: String, hideNotifications: Boolean, duration: Int?) {
        viewModel.mute(accountId, hideNotifications, duration)
    }

    override fun onMuteConversation(viewData: StatusViewData.Concrete, mute: Boolean) {
        viewModel.muteConversation(viewData.id, mute)
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
        requireContext().viewMedia(attachmentIndex, AttachmentViewData.list(viewData))
    }

    override fun onViewThread(viewData: StatusViewData.Concrete) {
        requireContext().viewThread(viewData)
    }

    override fun onEdit(viewData: StatusViewData.Concrete) {
        viewModel.editStatus(viewData.status)
    }

    override fun onReply(viewData: StatusViewData.Concrete) {
        requireContext().reply(viewData, accountManager.activeAccount!!)
    }

    override fun onReport(viewData: StatusViewData.Concrete) {
        requireContext().report(viewData)
    }

    override fun onViewTag(tag: String) {
        requireContext().viewTag(tag)
    }

    override fun onViewAccount(accountId: String) {
        requireContext().viewAccount(accountId)
    }

    override fun onViewUrl(url: String) {
        (requireActivity() as BottomSheetActivity).viewUrl(url)
    }

    override fun onViewReport(reportId: String) {
        requireContext().openLink(
            "https://${accountManager.activeAccount!!.domain}/admin/reports/$reportId"
        )
    }

    override fun onRespondToFollowRequest(accept: Boolean, accountIdRequestingFollow: String, position: Int) {
        val notification = adapter?.peek(position) ?: return
        viewModel.respondToFollowRequest(accept, accountId = accountIdRequestingFollow, notification = notification)
    }

    override fun onShowQuote(viewData: StatusViewData.Concrete) {
        viewModel.showQuote(viewData)
    }

    override fun removeQuote(viewData: StatusViewData.Concrete) {
        viewModel.removeQuote(viewData.status)
    }

    override fun onDestroyView() {
        adapter = null
        super.onDestroyView()
    }

    companion object {
        private const val TAG = "NotificationRequestsDetailsFragment"
        private const val EXTRA_ACCOUNT_ID = "accountId"
        fun newIntent(accountId: String, context: Context) = Intent(context, NotificationRequestDetailsActivity::class.java).apply {
            putExtra(EXTRA_ACCOUNT_ID, accountId)
        }
    }
}
