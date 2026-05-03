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

package site.warpnet.warpdroid.components.notifications

import androidx.recyclerview.widget.RecyclerView
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ItemFollowBinding
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.interfaces.AccountActionListener
import site.warpnet.warpdroid.interfaces.LinkListener
import site.warpnet.warpdroid.util.StatusDisplayOptions
import site.warpnet.warpdroid.util.emojify
import site.warpnet.warpdroid.util.hide
import site.warpnet.warpdroid.util.loadAvatar
import site.warpnet.warpdroid.util.parseAsWarpnetHtml
import site.warpnet.warpdroid.util.setClickableText
import site.warpnet.warpdroid.util.show
import site.warpnet.warpdroid.util.unicodeWrap
import site.warpnet.warpdroid.util.visible
import site.warpnet.warpdroid.viewdata.NotificationViewData

class FollowViewHolder(
    private val binding: ItemFollowBinding,
    private val listener: AccountActionListener,
    private val linkListener: LinkListener
) : RecyclerView.ViewHolder(binding.root), NotificationsViewHolder {

    override fun bind(
        viewData: NotificationViewData.Concrete,
        payloads: List<*>,
        statusDisplayOptions: StatusDisplayOptions
    ) {
        if (payloads.isNotEmpty()) {
            return
        }
        val context = itemView.context
        val account = viewData.account
        val messageTemplate = context.getString(
            if (viewData.type == Notification.Type.SignUp) R.string.notification_sign_up_format else R.string.notification_follow_format
        )
        val wrappedDisplayName = account.name.unicodeWrap()

        binding.notificationText.text = messageTemplate.format(wrappedDisplayName)
            .emojify(account.emojis, binding.notificationText, statusDisplayOptions.animateEmojis)

        binding.notificationUsername.text = context.getString(R.string.post_username_format, viewData.account.username)

        val emojifiedDisplayName = wrappedDisplayName.emojify(
            account.emojis,
            binding.notificationDisplayName,
            statusDisplayOptions.animateEmojis
        )
        binding.notificationDisplayName.text = emojifiedDisplayName

        if (account.note.isEmpty()) {
            binding.accountNote.hide()
        } else {
            binding.accountNote.show()

            val emojifiedNote = account.note.parseAsWarpnetHtml()
                .emojify(account.emojis, binding.accountNote, statusDisplayOptions.animateEmojis)
            setClickableText(binding.accountNote, emojifiedNote, emptyList(), null, linkListener)
        }

        val avatarRadius = context.resources
            .getDimensionPixelSize(R.dimen.avatar_radius_42dp)
        loadAvatar(
            account.avatar,
            binding.notificationAvatar,
            avatarRadius,
            statusDisplayOptions.animateAvatars
        )

        binding.avatarBadge.visible(statusDisplayOptions.showBotOverlay && account.bot)

        itemView.setOnClickListener { listener.onViewAccount(account.id) }
        binding.accountNote.setOnClickListener { listener.onViewAccount(account.id) }
    }
}
