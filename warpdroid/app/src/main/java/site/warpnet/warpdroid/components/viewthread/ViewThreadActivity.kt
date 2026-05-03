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

package site.warpnet.warpdroid.components.viewthread

import android.content.Context
import android.content.Intent
import android.os.Bundle
import androidx.fragment.app.commit
import site.warpnet.warpdroid.BottomSheetActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ActivityViewThreadBinding
import site.warpnet.warpdroid.util.viewBinding
import dagger.hilt.android.AndroidEntryPoint

@AndroidEntryPoint
class ViewThreadActivity : BottomSheetActivity() {

    private val binding by viewBinding(ActivityViewThreadBinding::inflate)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(binding.root)
        setSupportActionBar(binding.includedToolbar.toolbar)
        supportActionBar?.run {
            setDisplayHomeAsUpEnabled(true)
            setDisplayShowHomeEnabled(true)
            setDisplayShowTitleEnabled(true)
        }

        setTitle(R.string.title_view_thread)

        val id = intent.getStringExtra(ID_EXTRA)!!
        val url = intent.getStringExtra(URL_EXTRA)!!
        val fragment =
            supportFragmentManager.findFragmentByTag(FRAGMENT_TAG + id) as ViewThreadFragment?
                ?: ViewThreadFragment.newInstance(id, url)

        supportFragmentManager.commit {
            replace(R.id.fragment_container, fragment, FRAGMENT_TAG + id)
        }
    }

    companion object {

        fun newIntent(context: Context, id: String, url: String): Intent {
            val intent = Intent(context, ViewThreadActivity::class.java)
            intent.putExtra(ID_EXTRA, id)
            intent.putExtra(URL_EXTRA, url)
            return intent
        }

        private const val ID_EXTRA = "id"
        private const val URL_EXTRA = "url"
        private const val FRAGMENT_TAG = "ViewThreadFragment_"
    }
}
