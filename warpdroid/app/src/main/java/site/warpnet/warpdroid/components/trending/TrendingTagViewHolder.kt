/* Copyright 2023 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.trending

import androidx.recyclerview.widget.RecyclerView
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ItemTrendingCellBinding
import site.warpnet.warpdroid.util.formatNumber
import site.warpnet.warpdroid.viewdata.TrendingViewData
import java.text.NumberFormat

class TrendingTagViewHolder(
    private val binding: ItemTrendingCellBinding
) : RecyclerView.ViewHolder(binding.root) {

    private val numberFormat: NumberFormat = NumberFormat.getNumberInstance()

    fun setup(tagViewData: TrendingViewData.Tag, onViewTag: (String) -> Unit) {
        binding.tag.text = binding.root.context.getString(R.string.hashtag_format, tagViewData.name)

        binding.graph.maxTrendingValue = tagViewData.maxTrendingValue
        binding.graph.primaryLineData = tagViewData.usage
        binding.graph.secondaryLineData = tagViewData.accounts

        binding.totalUsage.text = formatNumber(tagViewData.usage.sum(), 1000)

        val totalAccounts = tagViewData.accounts.sum()
        binding.totalAccounts.text = formatNumber(totalAccounts, 1000)

        binding.currentUsage.text = numberFormat.format(tagViewData.usage.last())
        binding.currentAccounts.text = numberFormat.format(tagViewData.accounts.last())

        itemView.setOnClickListener {
            onViewTag(tagViewData.name)
        }

        itemView.contentDescription =
            itemView.context.getString(
                R.string.accessibility_talking_about_tag,
                totalAccounts,
                tagViewData.name
            )
    }
}
