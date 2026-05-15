/* Copyright 2024 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.notifications.requests.details

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
import site.warpnet.warpdroid.BottomSheetActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.compose.ComposeActivity
import site.warpnet.warpdroid.components.instanceinfo.InstanceInfoRepository
import site.warpnet.warpdroid.components.notifications.NotificationActionListener
import site.warpnet.warpdroid.components.notifications.NotificationsPagingAdapter
import site.warpnet.warpdroid.databinding.FragmentNotificationRequestDetailsBinding
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.interfaces.AccountActionListener
import site.warpnet.warpdroid.interfaces.LoadMoreActionListener
import site.warpnet.warpdroid.interfaces.TweetActionListener
import site.warpnet.warpdroid.settings.PrefKeys
import site.warpnet.warpdroid.util.CardViewMode
import site.warpnet.warpdroid.util.TweetDisplayOptions
import site.warpnet.warpdroid.util.getErrorString
import site.warpnet.warpdroid.util.hide
import site.warpnet.warpdroid.util.openLink
import site.warpnet.warpdroid.util.reply
import site.warpnet.warpdroid.util.report
import site.warpnet.warpdroid.util.show
import site.warpnet.warpdroid.util.startActivityWithSlideInAnimation
import site.warpnet.warpdroid.util.viewAccount
import site.warpnet.warpdroid.util.viewBinding
import site.warpnet.warpdroid.util.viewMedia
import site.warpnet.warpdroid.util.viewTag
import site.warpnet.warpdroid.util.viewThread
import site.warpnet.warpdroid.util.visible
import site.warpnet.warpdroid.view.ConfirmationBottomSheet.Companion.confirmLike
import site.warpnet.warpdroid.view.ConfirmationBottomSheet.Companion.confirmRetweet
import site.warpnet.warpdroid.viewdata.AttachmentViewData
import site.warpnet.warpdroid.viewdata.NotificationViewData
import site.warpnet.warpdroid.viewdata.TweetViewData
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch

@AndroidEntryPoint
class NotificationRequestDetailsFragment :
    Fragment(R.layout.fragment_notification_request_details),
    TweetActionListener,
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
        val statusDisplayOptions = TweetDisplayOptions(
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
            showStatsInline = preferences.getBoolean(PrefKeys.SHOW_STATS_INLINE, true),
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
                    binding.tweetView.show()
                    val errorState = loadState.refresh as LoadState.Error
                    binding.tweetView.setup(errorState.error) { retry() }
                    Log.w(TAG, "error loading notifications for user ${viewModel.accountId}", errorState.error)
                } else {
                    binding.recyclerView.show()
                    binding.tweetView.hide()
                }
            }
        }
    }

    override fun onLoadMore(loadMore: NotificationViewData.LoadMore) {
        // not relevant here
    }

    override fun onRetweet(
        viewData: TweetViewData.Concrete,
        retweet: Boolean,
        visibility: Tweet.Visibility?,
        state: SparkButtonState?
    ) {
        if (retweet && visibility == null) {
            confirmRetweet(preferences) { visibility ->
                viewModel.retweet(viewData.id, retweet, visibility)
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
        viewModel.bookmark(viewData.id, viewData.accountId, bookmark)
    }

    override fun onExpandedChange(viewData: TweetViewData.Concrete, expanded: Boolean) {
        viewModel.changeExpanded(expanded, viewData)
    }

    override fun onContentHiddenChange(viewData: TweetViewData.Concrete, isShowing: Boolean) {
        viewModel.changeContentShowing(isShowing, viewData)
    }

    override fun onContentCollapsedChange(viewData: TweetViewData.Concrete, isCollapsed: Boolean) {
        viewModel.changeContentCollapsed(isCollapsed, viewData)
    }

    override fun onVoteInPoll(viewData: TweetViewData.Concrete, pollId: String, choices: List<Int>) {
        viewModel.voteInPoll(viewData.id, pollId, choices)
    }

    override fun onShowPollResults(viewData: TweetViewData.Concrete) {
        viewModel.showPollResults(viewData)
    }

    override fun changeFilter(viewData: TweetViewData.Concrete, filtered: Boolean) {
        viewModel.changeFilter(filtered, viewData)
    }

    override fun onTranslate(viewData: TweetViewData.Concrete) {
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

    override fun onUntranslate(viewData: TweetViewData.Concrete) {
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

    override fun onMuteConversation(viewData: TweetViewData.Concrete, mute: Boolean) {
        viewModel.muteConversation(viewData.id, mute)
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
        requireContext().viewMedia(attachmentIndex, AttachmentViewData.list(viewData))
    }

    override fun onViewThread(viewData: TweetViewData.Concrete) {
        requireContext().viewThread(viewData)
    }

    override fun onEdit(viewData: TweetViewData.Concrete) {
        viewModel.editStatus(viewData.status)
    }

    override fun onReply(viewData: TweetViewData.Concrete) {
        requireContext().reply(viewData, accountManager.activeAccount!!)
    }

    override fun onReport(viewData: TweetViewData.Concrete) {
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

    override fun onShowQuote(viewData: TweetViewData.Concrete) {
        viewModel.showQuote(viewData)
    }

    override fun removeQuote(viewData: TweetViewData.Concrete) {
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
