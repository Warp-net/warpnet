/* Copyright 2019 Conny Duck
 *
 * This file is a part of Tusky.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Tusky is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Tusky; if not,
 * see <http://www.gnu.org/licenses>. */

package com.keylesspalace.tusky.adapter

import android.view.LayoutInflater
import android.view.MotionEvent
import android.view.ViewGroup
import androidx.recyclerview.widget.RecyclerView
import androidx.viewbinding.ViewBinding
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.TabData
import com.keylesspalace.tusky.databinding.ItemTabPreferenceBinding
import com.keylesspalace.tusky.databinding.ItemTabPreferenceSmallBinding
import com.keylesspalace.tusky.util.BindingHolder
import com.keylesspalace.tusky.util.hide
import com.keylesspalace.tusky.util.setDrawableTint

interface ItemInteractionListener {
    fun onTabAdded(tab: TabData)
    fun onTabRemoved(position: Int)
    fun onStartDelete(viewHolder: RecyclerView.ViewHolder)
    fun onStartDrag(viewHolder: RecyclerView.ViewHolder)
    fun onActionChipClicked(tab: TabData, tabPosition: Int)
    fun onChipClicked(tab: TabData, tabPosition: Int, chipPosition: Int)
}

class TabAdapter(
    private var data: List<TabData>,
    private val small: Boolean,
    private val listener: ItemInteractionListener,
    private var removeButtonEnabled: Boolean = false
) : RecyclerView.Adapter<BindingHolder<ViewBinding>>() {

    fun updateData(newData: List<TabData>) {
        this.data = newData
        notifyDataSetChanged()
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): BindingHolder<ViewBinding> {
        val binding = if (small) {
            ItemTabPreferenceSmallBinding.inflate(
                LayoutInflater.from(parent.context),
                parent,
                false
            )
        } else {
            ItemTabPreferenceBinding.inflate(LayoutInflater.from(parent.context), parent, false)
        }
        return BindingHolder(binding)
    }

    override fun onBindViewHolder(holder: BindingHolder<ViewBinding>, position: Int) {
        val tab = data[position]

        if (small) {
            val binding = holder.binding as ItemTabPreferenceSmallBinding

            binding.textView.setText(tab.text)

            binding.textView.setCompoundDrawablesRelativeWithIntrinsicBounds(tab.icon, 0, 0, 0)

            binding.textView.setOnClickListener {
                listener.onTabAdded(tab)
            }
        } else {
            val binding = holder.binding as ItemTabPreferenceBinding

            binding.textView.setText(tab.text)

            binding.textView.setCompoundDrawablesRelativeWithIntrinsicBounds(tab.icon, 0, 0, 0)

            binding.imageView.setOnTouchListener { _, event ->
                if (event.action == MotionEvent.ACTION_DOWN) {
                    listener.onStartDrag(holder)
                    true
                } else {
                    false
                }
            }
            binding.removeButton.setOnClickListener {
                listener.onTabRemoved(holder.bindingAdapterPosition)
            }
            binding.removeButton.isEnabled = removeButtonEnabled
            setDrawableTint(
                holder.itemView.context,
                binding.removeButton.drawable,
                (if (removeButtonEnabled) android.R.attr.textColorTertiary else R.attr.textColorDisabled)
            )

            binding.chipGroup.hide()
        }
    }

    override fun getItemCount() = data.size

    fun setRemoveButtonVisible(enabled: Boolean) {
        if (removeButtonEnabled != enabled) {
            removeButtonEnabled = enabled
            notifyDataSetChanged()
        }
    }
}
