/* Copyright 2019 Joel Pyska
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

package site.warpnet.warpdroid.components.report.fragments

import android.content.SharedPreferences
import android.os.Bundle
import android.view.Menu
import android.view.MenuInflater
import android.view.MenuItem
import android.view.View
import androidx.core.app.ActivityOptionsCompat
import androidx.core.view.MenuProvider
import androidx.core.view.ViewCompat
import androidx.fragment.app.Fragment
import androidx.fragment.app.activityViewModels
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.paging.LoadState
import androidx.recyclerview.widget.DividerItemDecoration
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.SimpleItemAnimator
import androidx.swiperefreshlayout.widget.SwipeRefreshLayout.OnRefreshListener
import com.google.android.material.snackbar.Snackbar
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.TweetListActivity
import site.warpnet.warpdroid.ViewMediaActivity
import site.warpnet.warpdroid.components.account.AccountActivity
import site.warpnet.warpdroid.components.report.ReportViewModel
import site.warpnet.warpdroid.components.report.adapter.AdapterHandler
import site.warpnet.warpdroid.components.report.adapter.TweetsAdapter
import site.warpnet.warpdroid.databinding.FragmentReportTweetsBinding
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.settings.PrefKeys
import site.warpnet.warpdroid.util.CardViewMode
import site.warpnet.warpdroid.util.TweetDisplayOptions
import site.warpnet.warpdroid.util.viewBinding
import site.warpnet.warpdroid.util.visible
import site.warpnet.warpdroid.viewdata.AttachmentViewData
import site.warpnet.warpdroid.viewdata.TweetViewData
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch

@AndroidEntryPoint
class ReportTweetsFragment :
    Fragment(R.layout.fragment_report_tweets),
    OnRefreshListener,
    MenuProvider,
    AdapterHandler {

    @Inject
    lateinit var accountManager: AccountManager

    @Inject
    lateinit var preferences: SharedPreferences

    private val viewModel: ReportViewModel by activityViewModels()

    private val binding by viewBinding(FragmentReportTweetsBinding::bind)

    private var adapter: TweetsAdapter? = null

    private var snackbarErrorRetry: Snackbar? = null

    override fun showMedia(v: View?, status: TweetViewData.Concrete, idx: Int) {
        when (status.attachments[idx].type) {
            Attachment.Type.GIFV, Attachment.Type.VIDEO, Attachment.Type.IMAGE, Attachment.Type.AUDIO -> {
                val attachments = AttachmentViewData.list(status)
                val intent = ViewMediaActivity.newIntent(requireContext(), attachments, idx)
                if (v != null) {
                    val url = status.attachments[idx].url
                    ViewCompat.setTransitionName(v, url)
                    val options = ActivityOptionsCompat.makeSceneTransitionAnimation(
                        requireActivity(),
                        v,
                        url
                    )
                    startActivity(intent, options.toBundle())
                } else {
                    startActivity(intent)
                }
            }

            Attachment.Type.UNKNOWN -> {
            }
        }
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        requireActivity().addMenuProvider(this, viewLifecycleOwner, Lifecycle.State.RESUMED)
        handleClicks()
        initStatusesView()
        binding.swipeRefreshLayout.setOnRefreshListener(this)
    }

    override fun onDestroyView() {
        // Clear the adapter to prevent leaking the View
        adapter = null
        snackbarErrorRetry = null
        super.onDestroyView()
    }

    override fun onCreateMenu(menu: Menu, menuInflater: MenuInflater) {
        menuInflater.inflate(R.menu.fragment_report_tweets, menu)
    }

    override fun onMenuItemSelected(menuItem: MenuItem): Boolean {
        return when (menuItem.itemId) {
            R.id.action_refresh -> {
                binding.swipeRefreshLayout.isRefreshing = true
                onRefresh()
                true
            }

            else -> false
        }
    }

    override fun onRefresh() {
        snackbarErrorRetry?.dismiss()
        snackbarErrorRetry = null
        adapter?.refresh()
    }

    private fun initStatusesView() {
        val statusDisplayOptions = TweetDisplayOptions(
            animateAvatars = false,
            mediaPreviewEnabled = accountManager.activeAccount?.mediaPreviewEnabled ?: true,
            useAbsoluteTime = preferences.getBoolean(PrefKeys.ABSOLUTE_TIME_VIEW, false),
            showBotOverlay = false,
            useBlurhash = preferences.getBoolean(PrefKeys.USE_BLURHASH, true),
            cardViewMode = CardViewMode.NONE,
            hideStats = preferences.getBoolean(PrefKeys.WELLBEING_HIDE_STATS_POSTS, false),
            animateEmojis = preferences.getBoolean(PrefKeys.ANIMATE_CUSTOM_EMOJIS, false),
            showStatsInline = preferences.getBoolean(PrefKeys.SHOW_STATS_INLINE, true),
            showSensitiveMedia = accountManager.activeAccount!!.alwaysShowSensitiveMedia,
            openSpoiler = accountManager.activeAccount!!.alwaysOpenSpoiler
        )

        val adapter = TweetsAdapter(statusDisplayOptions, viewModel.statusViewState, this)
        this.adapter = adapter

        binding.recyclerView.addItemDecoration(
            DividerItemDecoration(requireContext(), DividerItemDecoration.VERTICAL)
        )
        binding.recyclerView.layoutManager = LinearLayoutManager(requireContext())
        binding.recyclerView.adapter = adapter
        (binding.recyclerView.itemAnimator as SimpleItemAnimator).supportsChangeAnimations = false

        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.statusesFlow.collectLatest { pagingData ->
                adapter.submitData(pagingData)
            }
        }

        adapter.addLoadStateListener { loadState ->
            if (loadState.refresh is LoadState.Error ||
                loadState.append is LoadState.Error ||
                loadState.prepend is LoadState.Error
            ) {
                showError(adapter)
            }

            binding.progressBarBottom.visible(loadState.append == LoadState.Loading)
            binding.progressBarTop.visible(loadState.prepend == LoadState.Loading)
            binding.progressBarLoading.visible(
                loadState.refresh == LoadState.Loading && !binding.swipeRefreshLayout.isRefreshing
            )

            if (loadState.refresh != LoadState.Loading) {
                binding.swipeRefreshLayout.isRefreshing = false
            }
        }
    }

    private fun showError(adapter: TweetsAdapter) {
        if (snackbarErrorRetry?.isShown != true) {
            snackbarErrorRetry =
                Snackbar.make(binding.swipeRefreshLayout, R.string.failed_fetch_posts, Snackbar.LENGTH_INDEFINITE)
                    .setAction(R.string.action_retry) {
                        adapter.retry()
                    }.also {
                        it.show()
                    }
        }
    }

    private fun handleClicks() {
        binding.buttonBack.setOnClickListener {
            viewModel.backFrom(ReportViewModel.Screen.Statuses)
        }

        binding.buttonContinue.setOnClickListener {
            viewModel.forwardFrom(ReportViewModel.Screen.Statuses)
        }
    }

    override fun setStatusChecked(status: Tweet, isChecked: Boolean) {
        viewModel.setStatusChecked(status, isChecked)
    }

    override fun isStatusChecked(id: String): Boolean {
        return viewModel.isStatusChecked(id)
    }

    override fun onViewAccount(accountId: String) = startActivity(
        AccountActivity.newIntent(requireContext(), accountId)
    )

    override fun onViewTag(tag: String) = startActivity(
        TweetListActivity.newHashtagIntent(requireContext(), tag)
    )

    override fun onViewUrl(url: String) = viewModel.checkClickedUrl(url)

    companion object {
        fun newInstance() = ReportTweetsFragment()
    }
}
