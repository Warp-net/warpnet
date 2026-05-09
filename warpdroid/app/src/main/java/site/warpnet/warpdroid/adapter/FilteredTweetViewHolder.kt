/* Copyright 2024 Warpdroid Contributors
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
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.notifications.NotificationsViewHolder
import site.warpnet.warpdroid.databinding.ItemTweetFilteredBinding
import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.entity.FilterResult
import site.warpnet.warpdroid.interfaces.TweetActionListener
import site.warpnet.warpdroid.util.TweetDisplayOptions
import site.warpnet.warpdroid.viewdata.NotificationViewData
import site.warpnet.warpdroid.viewdata.TweetViewData

open class FilteredTweetViewHolder(
    private val binding: ItemTweetFilteredBinding,
    private val listener: TweetActionListener
) : RecyclerView.ViewHolder(binding.root) {

    fun bind(viewData: TweetViewData.Concrete) {
        val matchedFilterResult: FilterResult? = viewData.actionable.filtered.orEmpty().find { filterResult ->
            filterResult.filter.action == Filter.Action.WARN
        }

        val matchedFilterTitle = matchedFilterResult?.filter?.title.orEmpty()

        binding.tweetFilterLabel.text = itemView.context.getString(
            R.string.status_filter_placeholder_label_format,
            matchedFilterTitle
        )
        binding.tweetFilterShowAnyway.setOnClickListener {
            listener.changeFilter(viewData, false)
        }
    }
}

class FilteredNotificationViewHolder(
    binding: ItemTweetFilteredBinding,
    listener: TweetActionListener
) : FilteredTweetViewHolder(binding, listener), NotificationsViewHolder {

    override fun bind(
        viewData: NotificationViewData.Concrete,
        payloads: List<*>,
        statusDisplayOptions: TweetDisplayOptions
    ) {
        if (payloads.isEmpty()) {
            bind(viewData.statusViewData!!)
        }
    }
}
