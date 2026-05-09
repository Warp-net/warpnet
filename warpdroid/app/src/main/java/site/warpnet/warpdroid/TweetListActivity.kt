/* Copyright 2019 Warpdroid Contributors
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
 * see <https://www.gnu.org/licenses>. */

package site.warpnet.warpdroid

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.util.Log
import android.view.Menu
import android.view.MenuItem
import androidx.coordinatorlayout.widget.CoordinatorLayout
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat.Type.systemBars
import androidx.core.view.isVisible
import androidx.core.view.updateLayoutParams
import androidx.fragment.app.commit
import androidx.lifecycle.lifecycleScope
import at.connyduck.calladapter.networkresult.fold
import com.google.android.material.snackbar.Snackbar
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.FilterUpdatedEvent
import site.warpnet.warpdroid.components.compose.ComposeActivity
import site.warpnet.warpdroid.components.filters.EditFilterActivity
import site.warpnet.warpdroid.components.filters.FilterExpiration
import site.warpnet.warpdroid.components.filters.FiltersActivity
import site.warpnet.warpdroid.components.timeline.TimelineFragment
import site.warpnet.warpdroid.components.timeline.viewmodel.TimelineViewModel.Kind
import site.warpnet.warpdroid.databinding.ActivityTweetlistBinding
import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.util.startActivityWithSlideInAnimation
import site.warpnet.warpdroid.util.viewBinding
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.launch

@AndroidEntryPoint
class TweetListActivity : BottomSheetActivity() {

    @Inject
    lateinit var eventHub: EventHub

    private val binding: ActivityTweetlistBinding by viewBinding(
        ActivityTweetlistBinding::inflate
    )
    private lateinit var kind: Kind
    private var hashtag: String? = null
    private var statusId: String? = null
    private var followTagItem: MenuItem? = null
    private var unfollowTagItem: MenuItem? = null
    private var muteTagItem: MenuItem? = null
    private var unmuteTagItem: MenuItem? = null

    /** The filter muting hashtag, null if unknown or hashtag is not filtered */
    private var mutedFilter: Filter? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        Log.d("TweetListActivity", "onCreate")
        super.onCreate(savedInstanceState)
        setContentView(binding.root)

        setSupportActionBar(binding.includedToolbar.toolbar)

        kind = Kind.valueOf(intent.getStringExtra(EXTRA_KIND)!!)
        val listId = intent.getStringExtra(EXTRA_LIST_ID)
        hashtag = intent.getStringExtra(EXTRA_HASHTAG)
        statusId = intent.getStringExtra(EXTRA_STATUS_ID)

        val title = when (kind) {
            Kind.LIKES -> getString(R.string.title_likes)
            Kind.BOOKMARKS -> getString(R.string.title_bookmarks)
            Kind.TAG -> getString(R.string.hashtag_format, hashtag)
            Kind.PUBLIC_TRENDING_STATUSES -> getString(R.string.title_public_trending_statuses)
            Kind.QUOTES -> getString(R.string.title_quotes)
            else -> intent.getStringExtra(EXTRA_LIST_TITLE)
        }

        supportActionBar?.run {
            setTitle(title)
            setDisplayHomeAsUpEnabled(true)
            setDisplayShowHomeEnabled(true)
        }

        if (supportFragmentManager.findFragmentById(R.id.fragmentContainer) == null) {
            supportFragmentManager.commit {
                val fragment = when (kind) {
                    Kind.TAG -> TimelineFragment.newHashtagInstance(listOf(hashtag!!))
                    Kind.QUOTES -> TimelineFragment.newInstance(kind, statusId)
                    else -> TimelineFragment.newInstance(kind, listId)
                }
                replace(R.id.fragmentContainer, fragment)
            }
        }

        binding.composeButton.isVisible = kind == Kind.TAG
        binding.composeButton.setOnClickListener {
            postToTag()
        }
        val fabMargin = resources.getDimensionPixelSize(R.dimen.fabMargin)
        ViewCompat.setOnApplyWindowInsetsListener(binding.fragmentContainer) { _, insets ->
            val systemBarsInsets = insets.getInsets(systemBars())
            val bottomInsets = systemBarsInsets.bottom
            binding.composeButton.updateLayoutParams<CoordinatorLayout.LayoutParams> {
                bottomMargin = fabMargin + bottomInsets
            }
            insets
        }
    }

    override fun onCreateOptionsMenu(menu: Menu): Boolean {
        val tag = hashtag
        if (kind == Kind.TAG && tag != null) {
            lifecycleScope.launch {
                warpnetApi.tag(tag).fold(
                    { tagEntity ->
                        menuInflater.inflate(R.menu.view_hashtag_toolbar, menu)
                        followTagItem = menu.findItem(R.id.action_follow_hashtag)
                        unfollowTagItem = menu.findItem(R.id.action_unfollow_hashtag)
                        muteTagItem = menu.findItem(R.id.action_mute_hashtag)
                        unmuteTagItem = menu.findItem(R.id.action_unmute_hashtag)
                        followTagItem?.isVisible = tagEntity.following == false
                        unfollowTagItem?.isVisible = tagEntity.following == true
                        followTagItem?.setOnMenuItemClickListener { followTag() }
                        unfollowTagItem?.setOnMenuItemClickListener { unfollowTag() }
                        muteTagItem?.setOnMenuItemClickListener { muteTag() }
                        unmuteTagItem?.setOnMenuItemClickListener { unmuteTag() }
                        updateMuteTagMenuItems()
                    },
                    {
                        Log.w(TAG, "Failed to query tag #$tag", it)
                    }
                )
            }
        }

        return super.onCreateOptionsMenu(menu)
    }

    private fun postToTag() {
        val options = ComposeActivity.ComposeOptions(
            content = "#$hashtag",
            kind = ComposeActivity.ComposeKind.NEW
        )
        val intent = ComposeActivity.newIntent(this, options)
        startActivity(intent)
    }

    private fun followTag(): Boolean {
        val tag = hashtag
        if (tag != null) {
            lifecycleScope.launch {
                warpnetApi.followTag(tag).fold(
                    {
                        followTagItem?.isVisible = false
                        unfollowTagItem?.isVisible = true

                        Snackbar.make(
                            binding.root,
                            getString(R.string.following_hashtag_success_format, tag),
                            Snackbar.LENGTH_SHORT
                        ).show()
                    },
                    {
                        Snackbar.make(
                            binding.root,
                            getString(R.string.error_following_hashtag_format, tag),
                            Snackbar.LENGTH_SHORT
                        ).show()
                        Log.e(TAG, "Failed to follow #$tag", it)
                    }
                )
            }
        }

        return true
    }

    private fun unfollowTag(): Boolean {
        val tag = hashtag
        if (tag != null) {
            lifecycleScope.launch {
                warpnetApi.unfollowTag(tag).fold(
                    {
                        followTagItem?.isVisible = true
                        unfollowTagItem?.isVisible = false

                        Snackbar.make(
                            binding.root,
                            getString(R.string.unfollowing_hashtag_success_format, tag),
                            Snackbar.LENGTH_SHORT
                        ).show()
                    },
                    {
                        Snackbar.make(
                            binding.root,
                            getString(R.string.error_unfollowing_hashtag_format, tag),
                            Snackbar.LENGTH_SHORT
                        ).show()
                        Log.e(TAG, "Failed to unfollow #$tag", it)
                    }
                )
            }
        }

        return true
    }

    /**
     * Determine if the current hashtag is muted, and update the UI state accordingly.
     */
    private fun updateMuteTagMenuItems() {
        val tag = hashtag ?: return
        val hashedTag = "#$tag"

        muteTagItem?.isVisible = true
        muteTagItem?.isEnabled = false
        unmuteTagItem?.isVisible = false

        lifecycleScope.launch {
            warpnetApi.getFilters().fold(
                { filters ->
                    mutedFilter = filters.firstOrNull { filter ->
                        // TODO shouldn't this be an exact match (only one keyword; exactly the hashtag)?
                        filter.context.contains(Filter.Kind.HOME) && filter.title == hashedTag
                    }
                    updateTagMuteState(mutedFilter != null)
                },
                { throwable ->
                    Log.e(TAG, "Error getting filters: $throwable")
                }
            )
        }
    }

    private fun updateTagMuteState(muted: Boolean) {
        if (muted) {
            muteTagItem?.isVisible = false
            muteTagItem?.isEnabled = false
            unmuteTagItem?.isVisible = true
        } else {
            unmuteTagItem?.isVisible = false
            muteTagItem?.isEnabled = true
            muteTagItem?.isVisible = true
        }
    }

    private fun muteTag(): Boolean {
        val tag = hashtag ?: return true

        lifecycleScope.launch {
            var filterCreateSuccess = false
            val hashedTag = "#$tag"

            warpnetApi.createFilter(
                title = "#$tag",
                context = listOf(Filter.Kind.HOME),
                filterAction = Filter.Action.WARN,
                expiresIn = FilterExpiration.never
            ).fold(
                { filter ->
                    if (warpnetApi.addFilterKeyword(
                            filterId = filter.id,
                            keyword = hashedTag,
                            wholeWord = true
                        ).isSuccess
                    ) {
                        // must be requested again; otherwise does not contain the keyword (but server does)
                        mutedFilter = warpnetApi.getFilter(filter.id).getOrNull()

                        eventHub.dispatch(FilterUpdatedEvent(filter.context))
                        filterCreateSuccess = true
                    } else {
                        Snackbar.make(
                            binding.root,
                            getString(R.string.error_muting_hashtag_format, tag),
                            Snackbar.LENGTH_SHORT
                        ).show()
                        Log.e(TAG, "Failed to mute #$tag")
                    }
                },
                { throwable ->
                    Snackbar.make(
                        binding.root,
                        getString(R.string.error_muting_hashtag_format, tag),
                        Snackbar.LENGTH_SHORT
                    ).show()
                    Log.e(TAG, "Failed to mute #$tag", throwable)
                }
            )

            if (filterCreateSuccess) {
                updateTagMuteState(true)
                Snackbar.make(
                    binding.root,
                    getString(R.string.muting_hashtag_success_format, tag),
                    Snackbar.LENGTH_LONG
                ).apply {
                    setAction(R.string.action_view_filter) {
                        val intent = if (mutedFilter != null) {
                            Intent(this@TweetListActivity, EditFilterActivity::class.java).apply {
                                putExtra(EditFilterActivity.FILTER_TO_EDIT, mutedFilter)
                            }
                        } else {
                            Intent(this@TweetListActivity, FiltersActivity::class.java)
                        }

                        startActivityWithSlideInAnimation(intent)
                    }
                    show()
                }
            }
        }

        return true
    }

    private fun unmuteTag(): Boolean {
        lifecycleScope.launch {
            val tag = hashtag
            val result = if (mutedFilter != null) {
                val filter = mutedFilter!!
                if (filter.context.size > 1) {
                    // This filter exists in multiple contexts, just remove the home context
                    warpnetApi.updateFilter(
                        id = filter.id,
                        context = filter.context.filterNot { it == Filter.Kind.HOME }
                    )
                } else {
                    warpnetApi.deleteFilter(filter.id)
                }
            } else {
                null
            }

            result?.fold(
                {
                    updateTagMuteState(false)
                    eventHub.dispatch(FilterUpdatedEvent(listOf(Filter.Kind.HOME)))
                    mutedFilter = null

                    Snackbar.make(
                        binding.root,
                        getString(R.string.unmuting_hashtag_success_format, tag),
                        Snackbar.LENGTH_SHORT
                    ).show()
                },
                { throwable ->
                    Snackbar.make(
                        binding.root,
                        getString(R.string.error_unmuting_hashtag_format, tag),
                        Snackbar.LENGTH_SHORT
                    ).show()
                    Log.e(TAG, "Failed to unmute #$tag", throwable)
                }
            )
        }

        return true
    }

    companion object {

        private const val EXTRA_KIND = "kind"
        private const val EXTRA_LIST_ID = "id"
        private const val EXTRA_LIST_TITLE = "title"
        private const val EXTRA_HASHTAG = "tag"
        private const val EXTRA_STATUS_ID = "status"
        const val TAG = "TweetListActivity"

        fun newLikesIntent(context: Context) =
            Intent(context, TweetListActivity::class.java).apply {
                putExtra(EXTRA_KIND, Kind.LIKES.name)
            }

        fun newBookmarksIntent(context: Context) =
            Intent(context, TweetListActivity::class.java).apply {
                putExtra(EXTRA_KIND, Kind.BOOKMARKS.name)
            }

        fun newListIntent(context: Context, listId: String, listTitle: String) =
            Intent(context, TweetListActivity::class.java).apply {
                putExtra(EXTRA_KIND, Kind.LIST.name)
                putExtra(EXTRA_LIST_ID, listId)
                putExtra(EXTRA_LIST_TITLE, listTitle)
            }

        fun newHashtagIntent(context: Context, hashtag: String) =
            Intent(context, TweetListActivity::class.java).apply {
                putExtra(EXTRA_KIND, Kind.TAG.name)
                putExtra(EXTRA_HASHTAG, hashtag)
            }

        fun newTrendingIntent(context: Context) =
            Intent(context, TweetListActivity::class.java).apply {
                putExtra(EXTRA_KIND, Kind.PUBLIC_TRENDING_STATUSES.name)
            }

        fun newQuotesIntent(context: Context, statusId: String) =
            Intent(context, TweetListActivity::class.java).apply {
                putExtra(EXTRA_KIND, Kind.QUOTES.name)
                putExtra(EXTRA_STATUS_ID, statusId)
            }
    }
}
