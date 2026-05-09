/* Copyright 2019 Joel Pyska
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

package site.warpnet.warpdroid.components.report.adapter

import android.view.LayoutInflater
import android.view.ViewGroup
import androidx.paging.PagingDataAdapter
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.RecyclerView
import site.warpnet.warpdroid.components.report.model.TweetViewState
import site.warpnet.warpdroid.databinding.ItemReportTweetBinding
import site.warpnet.warpdroid.util.TweetDisplayOptions
import site.warpnet.warpdroid.viewdata.TweetViewData

class StatusesAdapter(
    private val statusDisplayOptions: TweetDisplayOptions,
    private val statusViewState: TweetViewState,
    private val adapterHandler: AdapterHandler
) : PagingDataAdapter<TweetViewData.Concrete, TweetViewHolder>(STATUS_COMPARATOR) {

    private val statusForPosition: (Int) -> TweetViewData.Concrete? = { position: Int ->
        if (position != RecyclerView.NO_POSITION) getItem(position) else null
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): TweetViewHolder {
        val binding = ItemReportTweetBinding.inflate(
            LayoutInflater.from(parent.context),
            parent,
            false
        )
        return TweetViewHolder(
            binding,
            statusDisplayOptions,
            statusViewState,
            adapterHandler,
            statusForPosition
        )
    }

    override fun onBindViewHolder(holder: TweetViewHolder, position: Int) {
        getItem(position)?.let { status ->
            holder.bind(status)
        }
    }

    companion object {
        val STATUS_COMPARATOR = object : DiffUtil.ItemCallback<TweetViewData.Concrete>() {
            override fun areContentsTheSame(
                oldItem: TweetViewData.Concrete,
                newItem: TweetViewData.Concrete
            ): Boolean = oldItem == newItem

            override fun areItemsTheSame(
                oldItem: TweetViewData.Concrete,
                newItem: TweetViewData.Concrete
            ): Boolean = oldItem.id == newItem.id
        }
    }
}
