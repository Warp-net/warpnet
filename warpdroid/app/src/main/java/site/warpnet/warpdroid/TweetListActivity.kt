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
import android.view.View
import androidx.coordinatorlayout.widget.CoordinatorLayout
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat.Type.systemBars
import androidx.core.view.updateLayoutParams
import androidx.fragment.app.commit
import site.warpnet.warpdroid.components.timeline.TimelineFragment
import site.warpnet.warpdroid.components.timeline.viewmodel.TimelineViewModel.Kind
import site.warpnet.warpdroid.databinding.ActivityTweetlistBinding
import site.warpnet.warpdroid.util.viewBinding
import dagger.hilt.android.AndroidEntryPoint

/**
 * Generic single-screen container for a [TimelineFragment] when there is no
 * dedicated activity for the kind (home goes through MainActivity, profiles
 * through AccountActivity).
 *
 * Warpdroid supports a smaller set of kinds than upstream Tusky because
 * Warpnet has no hashtag / trending / list timelines (those would require
 * §5 Tier B backend work). Today only LIKES / BOOKMARKS / QUOTES flow
 * through here.
 */
@AndroidEntryPoint
class TweetListActivity : BottomSheetActivity() {

    private val binding by viewBinding(ActivityTweetlistBinding::inflate)
    private lateinit var kind: Kind
    private var statusId: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(binding.root)

        setSupportActionBar(binding.includedToolbar.toolbar)

        kind = Kind.valueOf(intent.getStringExtra(EXTRA_KIND)!!)
        statusId = intent.getStringExtra(EXTRA_STATUS_ID)

        val title = when (kind) {
            Kind.LIKES -> getString(R.string.title_likes)
            Kind.BOOKMARKS -> getString(R.string.title_bookmarks)
            Kind.QUOTES -> getString(R.string.title_quotes)
            else -> null
        }

        supportActionBar?.run {
            setTitle(title)
            setDisplayHomeAsUpEnabled(true)
            setDisplayShowHomeEnabled(true)
        }

        if (supportFragmentManager.findFragmentById(R.id.fragmentContainer) == null) {
            supportFragmentManager.commit {
                val fragment = when (kind) {
                    Kind.QUOTES -> TimelineFragment.newInstance(kind, statusId)
                    else -> TimelineFragment.newInstance(kind, null)
                }
                replace(R.id.fragmentContainer, fragment)
            }
        }

        // Compose-from-tag was the only consumer of the FAB. Hide it.
        binding.composeButton.visibility = View.GONE

        ViewCompat.setOnApplyWindowInsetsListener(binding.root) { _, insets ->
            val bars = insets.getInsets(systemBars())
            binding.root.updateLayoutParams<CoordinatorLayout.LayoutParams> {
                topMargin = bars.top
                bottomMargin = bars.bottom
            }
            insets
        }
    }

    companion object {
        private const val EXTRA_KIND = "kind"
        private const val EXTRA_STATUS_ID = "statusId"

        fun newLikesIntent(context: Context): Intent =
            Intent(context, TweetListActivity::class.java).putExtra(EXTRA_KIND, Kind.LIKES.name)

        fun newBookmarksIntent(context: Context): Intent =
            Intent(context, TweetListActivity::class.java).putExtra(EXTRA_KIND, Kind.BOOKMARKS.name)

        fun newQuotesIntent(context: Context, statusId: String): Intent =
            Intent(context, TweetListActivity::class.java)
                .putExtra(EXTRA_KIND, Kind.QUOTES.name)
                .putExtra(EXTRA_STATUS_ID, statusId)
    }
}
