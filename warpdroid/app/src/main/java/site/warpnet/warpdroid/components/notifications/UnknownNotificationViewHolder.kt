/*
 * Copyright 2023 Warpdroid Contributors
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
 * see <http://www.gnu.org/licenses>.
 */

package site.warpnet.warpdroid.components.notifications

import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ItemUnknownNotificationBinding
import site.warpnet.warpdroid.util.TweetDisplayOptions
import site.warpnet.warpdroid.viewdata.NotificationViewData

internal class UnknownNotificationViewHolder(
    private val binding: ItemUnknownNotificationBinding,
) : NotificationsViewHolder, RecyclerView.ViewHolder(binding.root) {

    override fun bind(
        viewData: NotificationViewData.Concrete,
        payloads: List<*>,
        statusDisplayOptions: TweetDisplayOptions
    ) {
        binding.unknownNotificationType.text = viewData.type.name

        binding.root.setOnClickListener {
            MaterialAlertDialogBuilder(binding.root.context)
                .setMessage(R.string.unknown_notification_type_explanation)
                .setPositiveButton(android.R.string.ok, null)
                .show()
        }
    }
}
