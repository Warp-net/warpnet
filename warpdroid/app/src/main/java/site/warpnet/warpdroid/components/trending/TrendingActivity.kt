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

package site.warpnet.warpdroid.components.trending

import android.content.Context
import android.content.Intent
import android.os.Bundle
import androidx.fragment.app.commit
import site.warpnet.warpdroid.BaseActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ActivityTrendingBinding
import site.warpnet.warpdroid.util.viewBinding
import dagger.hilt.android.AndroidEntryPoint

@AndroidEntryPoint
class TrendingActivity : BaseActivity() {

    private val binding: ActivityTrendingBinding by viewBinding(ActivityTrendingBinding::inflate)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(binding.root)

        setSupportActionBar(binding.includedToolbar.toolbar)

        supportActionBar?.run {
            setTitle(R.string.title_public_trending_hashtags)
            setDisplayHomeAsUpEnabled(true)
            setDisplayShowHomeEnabled(true)
        }

        if (supportFragmentManager.findFragmentById(R.id.fragmentContainer) == null) {
            supportFragmentManager.commit {
                val fragment = TrendingTagsFragment.newInstance()
                replace(R.id.fragmentContainer, fragment)
            }
        }
    }

    companion object {
        fun getIntent(context: Context) = Intent(context, TrendingActivity::class.java)
    }
}
