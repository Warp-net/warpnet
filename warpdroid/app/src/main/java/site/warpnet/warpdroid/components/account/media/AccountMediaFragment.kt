/* Copyright 2022 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.account.media

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
import androidx.fragment.app.viewModels
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.paging.LoadState
import androidx.recyclerview.widget.GridLayoutManager
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.ViewMediaActivity
import site.warpnet.warpdroid.databinding.FragmentTimelineBinding
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.interfaces.ActionButtonActivity
import site.warpnet.warpdroid.interfaces.RefreshableFragment
import site.warpnet.warpdroid.settings.PrefKeys
import site.warpnet.warpdroid.util.ensureBottomPadding
import site.warpnet.warpdroid.util.hide
import site.warpnet.warpdroid.util.openLink
import site.warpnet.warpdroid.util.show
import site.warpnet.warpdroid.util.viewBinding
import site.warpnet.warpdroid.viewdata.AttachmentViewData
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch

/**
 * Fragment with multiple columns of media previews for the specified account.
 */
@AndroidEntryPoint
class AccountMediaFragment :
    Fragment(R.layout.fragment_timeline),
    RefreshableFragment,
    MenuProvider {

    @Inject
    lateinit var accountManager: AccountManager

    @Inject
    lateinit var preferences: SharedPreferences

    private val binding by viewBinding(FragmentTimelineBinding::bind)

    private val viewModel: AccountMediaViewModel by viewModels()

    private var adapter: AccountMediaGridAdapter? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        viewModel.accountId = arguments?.getString(ACCOUNT_ID_ARG)!!
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        requireActivity().addMenuProvider(this, viewLifecycleOwner, Lifecycle.State.RESUMED)

        val useBlurhash = preferences.getBoolean(PrefKeys.USE_BLURHASH, true)

        val hasFab = (activity as? ActionButtonActivity?)?.actionButton != null
        binding.recyclerView.ensureBottomPadding(fab = hasFab)

        val adapter = AccountMediaGridAdapter(
            useBlurhash = useBlurhash,
            context = view.context,
            onAttachmentClickListener = ::onAttachmentClick
        )
        this.adapter = adapter

        val columnCount = view.context.resources.getInteger(R.integer.profile_media_column_count)
        val imageSpacing = view.context.resources.getDimensionPixelSize(
            R.dimen.profile_media_spacing
        )

        binding.recyclerView.addItemDecoration(
            GridSpacingItemDecoration(columnCount, imageSpacing, 0)
        )

        binding.recyclerView.layoutManager = GridLayoutManager(view.context, columnCount)
        binding.recyclerView.adapter = adapter

        binding.swipeRefreshLayout.isEnabled = false
        binding.swipeRefreshLayout.setOnRefreshListener { refreshContent() }

        binding.tweetView.visibility = View.GONE

        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.media.collectLatest { pagingData ->
                adapter.submitData(pagingData)
            }
        }

        adapter.addLoadStateListener { loadState ->
            binding.tweetView.hide()
            binding.progressBar.hide()

            if (loadState.refresh != LoadState.Loading && loadState.source.refresh != LoadState.Loading) {
                binding.swipeRefreshLayout.isRefreshing = false
            }

            if (adapter.itemCount == 0) {
                when (loadState.refresh) {
                    is LoadState.NotLoading -> {
                        if (loadState.append is LoadState.NotLoading && loadState.source.refresh is LoadState.NotLoading) {
                            binding.tweetView.show()
                            binding.tweetView.setup(
                                R.drawable.elephant_friend_empty,
                                R.string.message_empty,
                                null
                            )
                        }
                    }
                    is LoadState.Error -> {
                        binding.tweetView.show()
                        binding.tweetView.setup((loadState.refresh as LoadState.Error).error)
                    }
                    is LoadState.Loading -> {
                        binding.progressBar.show()
                    }
                }
            }
        }
    }

    override fun onDestroyView() {
        // Clear the adapter to prevent leaking the View
        adapter = null
        super.onDestroyView()
    }

    override fun onCreateMenu(menu: Menu, menuInflater: MenuInflater) {
        menuInflater.inflate(R.menu.fragment_account_media, menu)
    }

    override fun onMenuItemSelected(menuItem: MenuItem): Boolean {
        return when (menuItem.itemId) {
            R.id.action_refresh -> {
                binding.swipeRefreshLayout.isRefreshing = true
                refreshContent()
                true
            }
            else -> false
        }
    }

    private fun onAttachmentClick(selected: AttachmentViewData, view: View) {
        if (!selected.isRevealed) {
            viewModel.revealAttachment(selected)
            return
        }
        val attachmentsFromSameStatus = viewModel.attachmentData.filter { attachmentViewData ->
            attachmentViewData.statusId == selected.statusId
        }
        val currentIndex = attachmentsFromSameStatus.indexOf(selected)

        when (selected.attachment.type) {
            Attachment.Type.IMAGE,
            Attachment.Type.GIFV,
            Attachment.Type.VIDEO,
            Attachment.Type.AUDIO -> {
                val intent = ViewMediaActivity.newIntent(
                    view.context,
                    attachmentsFromSameStatus,
                    currentIndex
                )
                if (activity != null) {
                    val url = selected.attachment.url
                    ViewCompat.setTransitionName(view, url)
                    val options = ActivityOptionsCompat.makeSceneTransitionAnimation(
                        requireActivity(),
                        view,
                        url
                    )
                    startActivity(intent, options.toBundle())
                } else {
                    startActivity(intent)
                }
            }
            Attachment.Type.UNKNOWN -> {
                context?.openLink(selected.attachment.unknownUrl)
            }
        }
    }

    override fun refreshContent() {
        adapter?.refresh()
    }

    companion object {

        fun newInstance(accountId: String): AccountMediaFragment {
            val fragment = AccountMediaFragment()
            val args = Bundle(1)
            args.putString(ACCOUNT_ID_ARG, accountId)
            fragment.arguments = args
            return fragment
        }

        private const val ACCOUNT_ID_ARG = "account_id"
    }
}
