/* Copyright 2025 Warpdroid Contributors
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

import android.view.ViewGroup
import androidx.core.view.updateLayoutParams
import androidx.core.view.updatePaddingRelative
import androidx.recyclerview.widget.RecyclerView
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ItemPlaceholderBinding
import site.warpnet.warpdroid.util.visible

class PlaceholderViewHolder(
    binding: ItemPlaceholderBinding,
    mode: Mode,
) : RecyclerView.ViewHolder(binding.root) {
    init {
        val res = binding.root.context.resources
        binding.topPlaceholder.visible(mode != Mode.STATUS)
        binding.reblogButtonPlaceholder.visible(mode != Mode.CONVERSATION)
        if (mode == Mode.NOTIFICATION) {
            binding.topPlaceholder.updatePaddingRelative(
                start = res.getDimensionPixelSize(R.dimen.status_info_padding_large)
            )
        }
        if (mode == Mode.CONVERSATION) {
            binding.moreButtonPlaceHolder.updateLayoutParams<ViewGroup.MarginLayoutParams> {
                marginEnd = res.getDimensionPixelSize(R.dimen.conversation_placeholder_more_button_inset)
            }
        }
    }

    enum class Mode {
        STATUS,
        NOTIFICATION,
        CONVERSATION
    }
}
