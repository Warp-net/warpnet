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

package site.warpnet.warpdroid.components.notifications

import android.content.Intent
import androidx.core.net.toUri
import androidx.recyclerview.widget.RecyclerView
import site.warpnet.warpdroid.databinding.ItemModerationWarningNotificationBinding
import site.warpnet.warpdroid.util.StatusDisplayOptions
import site.warpnet.warpdroid.viewdata.NotificationViewData

class ModerationWarningViewHolder(
    private val binding: ItemModerationWarningNotificationBinding,
    private val instanceDomain: String
) : RecyclerView.ViewHolder(binding.root), NotificationsViewHolder {

    override fun bind(
        viewData: NotificationViewData.Concrete,
        payloads: List<*>,
        statusDisplayOptions: StatusDisplayOptions
    ) {
        if (payloads.isNotEmpty()) {
            return
        }
        val warning = viewData.moderationWarning!!

        binding.moderationWarningDescription.setText(warning.action.text)

        binding.root.setOnClickListener {
            val intent = Intent(Intent.ACTION_VIEW, "https://$instanceDomain/disputes/strikes/${warning.id}".toUri())
            binding.root.context.startActivity(intent)
        }
    }
}
