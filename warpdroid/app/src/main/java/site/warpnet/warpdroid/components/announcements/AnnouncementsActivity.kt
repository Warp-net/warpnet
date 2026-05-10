/* Copyright 2020 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.announcements

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.view.Menu
import android.view.MenuInflater
import android.view.MenuItem
import android.view.View
import android.widget.PopupWindow
import androidx.activity.viewModels
import androidx.core.view.MenuProvider
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.DividerItemDecoration
import androidx.recyclerview.widget.LinearLayoutManager
import site.warpnet.warpdroid.BottomSheetActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.TweetListActivity
import site.warpnet.warpdroid.adapter.EmojiAdapter
import site.warpnet.warpdroid.adapter.OnEmojiSelectedListener
import site.warpnet.warpdroid.databinding.ActivityAnnouncementsBinding
import site.warpnet.warpdroid.settings.PrefKeys
import site.warpnet.warpdroid.util.Error
import site.warpnet.warpdroid.util.Loading
import site.warpnet.warpdroid.util.Success
import site.warpnet.warpdroid.util.ensureBottomPadding
import site.warpnet.warpdroid.util.hide
import site.warpnet.warpdroid.util.show
import site.warpnet.warpdroid.util.startActivityWithSlideInAnimation
import site.warpnet.warpdroid.util.unsafeLazy
import site.warpnet.warpdroid.util.viewBinding
import site.warpnet.warpdroid.view.EmojiPicker
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.launch

@AndroidEntryPoint
class AnnouncementsActivity :
    BottomSheetActivity(),
    AnnouncementActionListener,
    OnEmojiSelectedListener,
    MenuProvider {

    private val viewModel: AnnouncementsViewModel by viewModels()

    private val binding by viewBinding(ActivityAnnouncementsBinding::inflate)

    private lateinit var adapter: AnnouncementAdapter

    private val picker by unsafeLazy { EmojiPicker(this) }
    private val pickerDialog by unsafeLazy {
        PopupWindow(this)
            .apply {
                contentView = picker
                isFocusable = true
                setOnDismissListener {
                    currentAnnouncementId = null
                }
            }
    }
    private var currentAnnouncementId: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(binding.root)
        addMenuProvider(this)

        setSupportActionBar(binding.includedToolbar.toolbar)
        supportActionBar?.apply {
            title = getString(R.string.title_announcements)
            setDisplayHomeAsUpEnabled(true)
            setDisplayShowHomeEnabled(true)
        }

        binding.swipeRefreshLayout.setOnRefreshListener(this::refreshAnnouncements)

        binding.announcementsList.ensureBottomPadding()
        binding.announcementsList.setHasFixedSize(true)
        binding.announcementsList.layoutManager = LinearLayoutManager(this)
        val divider = DividerItemDecoration(this, DividerItemDecoration.VERTICAL)
        binding.announcementsList.addItemDecoration(divider)

        val wellbeingEnabled = preferences.getBoolean(PrefKeys.WELLBEING_HIDE_STATS_POSTS, false)
        val animateEmojis = preferences.getBoolean(PrefKeys.ANIMATE_CUSTOM_EMOJIS, false)

        adapter = AnnouncementAdapter(emptyList(), this, wellbeingEnabled, animateEmojis)

        binding.announcementsList.adapter = adapter

        lifecycleScope.launch {
            viewModel.announcements.collect {
                if (it == null) return@collect
                when (it) {
                    is Success -> {
                        binding.progressBar.hide()
                        binding.swipeRefreshLayout.isRefreshing = false
                        if (it.data.isNullOrEmpty()) {
                            binding.errorMessageView.setup(
                                R.drawable.elephant_friend_empty,
                                R.string.no_announcements
                            )
                            binding.errorMessageView.show()
                        } else {
                            binding.errorMessageView.hide()
                        }
                        adapter.updateList(it.data ?: listOf())
                    }
                    is Loading -> {
                        binding.errorMessageView.hide()
                    }
                    is Error -> {
                        binding.progressBar.hide()
                        binding.swipeRefreshLayout.isRefreshing = false
                        binding.errorMessageView.setup(
                            R.drawable.errorphant_error,
                            R.string.error_generic
                        ) {
                            refreshAnnouncements()
                        }
                        binding.errorMessageView.show()
                    }
                }
            }
        }

        lifecycleScope.launch {
            viewModel.emoji.collect {
                picker.adapter = EmojiAdapter(it, this@AnnouncementsActivity, animateEmojis)
            }
        }

        viewModel.load()
        binding.progressBar.show()
    }

    override fun onCreateMenu(menu: Menu, menuInflater: MenuInflater) {
        menuInflater.inflate(R.menu.activity_announcements, menu)
    }

    override fun onMenuItemSelected(menuItem: MenuItem): Boolean {
        return when (menuItem.itemId) {
            R.id.action_refresh -> {
                binding.swipeRefreshLayout.isRefreshing = true
                refreshAnnouncements()
                true
            }
            else -> false
        }
    }

    private fun refreshAnnouncements() {
        viewModel.load()
        binding.swipeRefreshLayout.isRefreshing = true
    }

    override fun openReactionPicker(announcementId: String, target: View) {
        currentAnnouncementId = announcementId
        pickerDialog.showAsDropDown(target)
    }

    override fun onEmojiSelected(shortcode: String) {
        viewModel.addReaction(currentAnnouncementId!!, shortcode)
        pickerDialog.dismiss()
    }

    override fun addReaction(announcementId: String, name: String) {
        viewModel.addReaction(announcementId, name)
    }

    override fun removeReaction(announcementId: String, name: String) {
        viewModel.removeReaction(announcementId, name)
    }

    override fun onViewTag(tag: String) {
        val intent = TweetListActivity.newHashtagIntent(this, tag)
        startActivityWithSlideInAnimation(intent)
    }

    override fun onViewAccount(accountId: String) {
        viewAccount(accountId)
    }

    override fun onViewUrl(url: String) {
        viewUrl(url)
    }

    companion object {
        fun newIntent(context: Context) = Intent(context, AnnouncementsActivity::class.java)
    }
}
