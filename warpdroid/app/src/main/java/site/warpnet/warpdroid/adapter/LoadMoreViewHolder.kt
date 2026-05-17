/* Copyright 2021 Warpdroid Contributors
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

import androidx.recyclerview.widget.RecyclerView
import site.warpnet.warpdroid.databinding.ItemLoadMoreBinding
import site.warpnet.warpdroid.interfaces.LoadMoreActionListener
import site.warpnet.warpdroid.util.hide
import site.warpnet.warpdroid.util.visible
import site.warpnet.warpdroid.viewdata.LoadMoreViewData

/**
 * Placeholder for missing parts in timelines.
 *
 * Displays a "Load more" button to load the gap, or a
 * circular progress bar if the missing page is being loaded.
 */
class LoadMoreViewHolder<LM : LoadMoreViewData>(
    private val binding: ItemLoadMoreBinding,
    private val listener: LoadMoreActionListener<LM>
) : RecyclerView.ViewHolder(binding.root) {

    fun setup(viewData: LM) {
        binding.loadMoreButton.visible(!viewData.isLoading)
        binding.loadMoreProgressBar.visible(viewData.isLoading)
        binding.loadMoreButton.setOnClickListener {
            binding.loadMoreButton.hide()
            binding.loadMoreProgressBar.show()
            listener.onLoadMore(viewData)
        }
    }
}
