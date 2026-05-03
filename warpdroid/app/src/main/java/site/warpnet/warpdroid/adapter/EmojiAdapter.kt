/* Copyright 2018 Conny Duck
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

package site.warpnet.warpdroid.adapter

import android.view.LayoutInflater
import android.view.ViewGroup
import androidx.appcompat.widget.TooltipCompat
import androidx.recyclerview.widget.RecyclerView
import com.bumptech.glide.Glide
import site.warpnet.warpdroid.databinding.ItemEmojiButtonBinding
import site.warpnet.warpdroid.entity.Emoji
import site.warpnet.warpdroid.util.BindingHolder
import java.util.Locale

class EmojiAdapter(
    emojiList: List<Emoji>,
    private val onEmojiSelectedListener: OnEmojiSelectedListener,
    private val animate: Boolean
) : RecyclerView.Adapter<BindingHolder<ItemEmojiButtonBinding>>() {

    private val emojiList: List<Emoji> = emojiList.filter { emoji -> emoji.visibleInPicker }
        .sortedBy { it.shortcode.lowercase(Locale.ROOT) }
        .sortedBy { it.category?.lowercase(Locale.ROOT) ?: "" }

    override fun getItemCount() = emojiList.size

    override fun onCreateViewHolder(
        parent: ViewGroup,
        viewType: Int
    ): BindingHolder<ItemEmojiButtonBinding> {
        val binding = ItemEmojiButtonBinding.inflate(
            LayoutInflater.from(parent.context),
            parent,
            false
        )
        return BindingHolder(binding)
    }

    override fun onBindViewHolder(holder: BindingHolder<ItemEmojiButtonBinding>, position: Int) {
        val emoji = emojiList[position]
        val emojiImageView = holder.binding.root

        if (animate) {
            Glide.with(emojiImageView)
                .load(emoji.url)
                .into(emojiImageView)
        } else {
            Glide.with(emojiImageView)
                .asBitmap()
                .load(emoji.url)
                .into(emojiImageView)
        }

        emojiImageView.setOnClickListener {
            onEmojiSelectedListener.onEmojiSelected(emoji.shortcode)
        }

        emojiImageView.contentDescription = emoji.shortcode
        TooltipCompat.setTooltipText(emojiImageView, emoji.shortcode)
    }
}

interface OnEmojiSelectedListener {
    fun onEmojiSelected(shortcode: String)
}
