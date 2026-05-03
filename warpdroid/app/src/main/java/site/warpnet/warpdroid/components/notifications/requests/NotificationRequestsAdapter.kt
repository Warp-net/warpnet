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

package site.warpnet.warpdroid.components.notifications.requests

import android.view.LayoutInflater
import android.view.ViewGroup
import androidx.annotation.OptIn
import androidx.paging.PagingDataAdapter
import androidx.recyclerview.widget.DiffUtil
import com.google.android.material.badge.ExperimentalBadgeUtils
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ItemNotificationRequestBinding
import site.warpnet.warpdroid.entity.NotificationRequest
import site.warpnet.warpdroid.util.BindingHolder
import site.warpnet.warpdroid.util.emojify
import site.warpnet.warpdroid.util.loadAvatar
import java.text.NumberFormat

class NotificationRequestsAdapter(
    private val onAcceptRequest: (notificationRequestId: String) -> Unit,
    private val onDismissRequest: (notificationRequestId: String) -> Unit,
    private val onOpenDetails: (notificationRequest: NotificationRequest) -> Unit,
    private val animateAvatar: Boolean,
    private val animateEmojis: Boolean,
) : PagingDataAdapter<NotificationRequest, BindingHolder<ItemNotificationRequestBinding>>(NOTIFICATION_REQUEST_COMPARATOR) {

    private val numberFormat: NumberFormat = NumberFormat.getNumberInstance()

    override fun onCreateViewHolder(
        parent: ViewGroup,
        viewType: Int
    ): BindingHolder<ItemNotificationRequestBinding> {
        val binding = ItemNotificationRequestBinding.inflate(
            LayoutInflater.from(parent.context),
            parent,
            false
        )
        return BindingHolder(binding)
    }

    @OptIn(ExperimentalBadgeUtils::class)
    override fun onBindViewHolder(holder: BindingHolder<ItemNotificationRequestBinding>, position: Int) {
        getItem(position)?.let { notificationRequest ->
            val binding = holder.binding
            val context = binding.root.context
            val account = notificationRequest.account

            val avatarRadius = context.resources.getDimensionPixelSize(R.dimen.avatar_radius_48dp)
            loadAvatar(account.avatar, binding.notificationRequestAvatar, avatarRadius, animateAvatar)

            binding.notificationRequestBadge.text = numberFormat.format(notificationRequest.notificationsCount)

            val emojifiedName = account.name.emojify(
                account.emojis,
                binding.notificationRequestDisplayName,
                animateEmojis
            )
            binding.notificationRequestDisplayName.text = emojifiedName
            val formattedUsername = context.getString(R.string.post_username_format, account.username)
            binding.notificationRequestUsername.text = formattedUsername

            binding.notificationRequestAccept.setOnClickListener {
                onAcceptRequest(notificationRequest.id)
            }
            binding.notificationRequestDismiss.setOnClickListener {
                onDismissRequest(notificationRequest.id)
            }
            binding.root.setOnClickListener {
                onOpenDetails(notificationRequest)
            }
        }
    }

    companion object {
        val NOTIFICATION_REQUEST_COMPARATOR = object : DiffUtil.ItemCallback<NotificationRequest>() {
            override fun areItemsTheSame(oldItem: NotificationRequest, newItem: NotificationRequest): Boolean =
                oldItem.id == newItem.id
            override fun areContentsTheSame(oldItem: NotificationRequest, newItem: NotificationRequest): Boolean =
                oldItem == newItem
        }
    }
}
