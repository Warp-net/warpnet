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

package site.warpnet.warpdroid.components.compose

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.BaseAdapter
import android.widget.Filter
import android.widget.Filterable
import androidx.annotation.WorkerThread
import com.bumptech.glide.Glide
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ItemAutocompleteAccountBinding
import site.warpnet.warpdroid.databinding.ItemAutocompleteEmojiBinding
import site.warpnet.warpdroid.databinding.ItemAutocompleteHashtagBinding
import site.warpnet.warpdroid.entity.Emoji
import site.warpnet.warpdroid.entity.TimelineAccount
import site.warpnet.warpdroid.util.loadAvatar
import site.warpnet.warpdroid.util.visible

class ComposeAutoCompleteAdapter(
    private val autocompletionProvider: AutocompletionProvider,
    private val animateAvatar: Boolean,
    @Suppress("UNUSED_PARAMETER") animateEmojis: Boolean,
    private val showBotBadge: Boolean,
    // if true, @ # : are returned in the result, otherwise only the raw value
    private val withDecoration: Boolean = true,
) : BaseAdapter(), Filterable {

    private var resultList: List<AutocompleteResult> = emptyList()

    override fun getCount() = resultList.size

    override fun getItem(index: Int): AutocompleteResult {
        return resultList[index]
    }

    override fun getItemId(position: Int): Long {
        return position.toLong()
    }

    override fun getFilter() = object : Filter() {

        override fun convertResultToString(resultValue: Any): CharSequence {
            return when (resultValue) {
                is AutocompleteResult.AccountResult -> if (withDecoration) "@${resultValue.account.username}" else resultValue.account.username
                is AutocompleteResult.HashtagResult -> if (withDecoration) "#${resultValue.hashtag}" else resultValue.hashtag
                is AutocompleteResult.EmojiResult -> if (withDecoration) ":${resultValue.emoji.shortcode}:" else resultValue.emoji.shortcode
                else -> ""
            }
        }

        @WorkerThread
        override fun performFiltering(constraint: CharSequence?): FilterResults {
            val filterResults = FilterResults()
            if (constraint != null) {
                val results = autocompletionProvider.search(constraint.toString())
                filterResults.values = results
                filterResults.count = results.size
            }
            return filterResults
        }

        @Suppress("UNCHECKED_CAST")
        override fun publishResults(constraint: CharSequence?, results: FilterResults) {
            if (results.count > 0) {
                resultList = results.values as List<AutocompleteResult>
                notifyDataSetChanged()
            } else {
                notifyDataSetInvalidated()
            }
        }
    }

    override fun getView(position: Int, convertView: View?, parent: ViewGroup): View {
        val itemViewType = getItemViewType(position)
        val context = parent.context

        val view: View = convertView ?: run {
            val layoutInflater = LayoutInflater.from(context)
            val binding = when (itemViewType) {
                ACCOUNT_VIEW_TYPE -> ItemAutocompleteAccountBinding.inflate(layoutInflater)
                HASHTAG_VIEW_TYPE -> ItemAutocompleteHashtagBinding.inflate(layoutInflater)
                EMOJI_VIEW_TYPE -> ItemAutocompleteEmojiBinding.inflate(layoutInflater)
                else -> throw AssertionError("unknown view type")
            }
            binding.root.tag = binding
            binding.root
        }

        when (val binding = view.tag) {
            is ItemAutocompleteAccountBinding -> {
                val accountResult = getItem(position) as AutocompleteResult.AccountResult
                val account = accountResult.account
                binding.username.text = context.getString(R.string.post_username_format, account.username)
                binding.displayName.text = account.name
                val avatarRadius = context.resources.getDimensionPixelSize(
                    R.dimen.avatar_radius_42dp
                )
                loadAvatar(
                    account.avatar,
                    binding.avatar,
                    avatarRadius,
                    animateAvatar
                )
                binding.avatarBadge.visible(showBotBadge && account.bot)
            }
            is ItemAutocompleteHashtagBinding -> {
                val result = getItem(position) as AutocompleteResult.HashtagResult
                binding.root.text = context.getString(R.string.hashtag_format, result.hashtag)
            }
            is ItemAutocompleteEmojiBinding -> {
                val emojiResult = getItem(position) as AutocompleteResult.EmojiResult
                val (shortcode, url) = emojiResult.emoji
                binding.shortcode.text = context.getString(R.string.emoji_shortcode_format, shortcode)
                Glide.with(binding.preview)
                    .load(url)
                    .into(binding.preview)
            }
        }
        return view
    }

    override fun getViewTypeCount() = 3

    override fun getItemViewType(position: Int): Int {
        return when (getItem(position)) {
            is AutocompleteResult.AccountResult -> ACCOUNT_VIEW_TYPE
            is AutocompleteResult.HashtagResult -> HASHTAG_VIEW_TYPE
            is AutocompleteResult.EmojiResult -> EMOJI_VIEW_TYPE
        }
    }

    sealed interface AutocompleteResult {
        class AccountResult(val account: TimelineAccount) : AutocompleteResult

        class HashtagResult(val hashtag: String) : AutocompleteResult

        class EmojiResult(val emoji: Emoji) : AutocompleteResult
    }

    interface AutocompletionProvider {
        fun search(token: String): List<AutocompleteResult>
    }

    companion object {
        private const val ACCOUNT_VIEW_TYPE = 0
        private const val HASHTAG_VIEW_TYPE = 1
        private const val EMOJI_VIEW_TYPE = 2
    }
}
