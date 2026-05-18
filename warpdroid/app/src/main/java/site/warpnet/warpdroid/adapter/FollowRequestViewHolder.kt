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

import android.graphics.Typeface
import android.text.SpannableString
import android.text.Spanned
import android.text.style.StyleSpan
import androidx.recyclerview.widget.RecyclerView
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.notifications.NotificationsViewHolder
import site.warpnet.warpdroid.databinding.ItemFollowRequestBinding
import site.warpnet.warpdroid.entity.TimelineAccount
import site.warpnet.warpdroid.interfaces.AccountActionListener
import site.warpnet.warpdroid.interfaces.LinkListener
import site.warpnet.warpdroid.util.TweetDisplayOptions
import site.warpnet.warpdroid.util.hide
import site.warpnet.warpdroid.util.loadAvatar
import site.warpnet.warpdroid.util.parseAsWarpnetHtml
import site.warpnet.warpdroid.util.setClickableText
import site.warpnet.warpdroid.util.show
import site.warpnet.warpdroid.util.unicodeWrap
import site.warpnet.warpdroid.util.visible
import site.warpnet.warpdroid.viewdata.NotificationViewData

class FollowRequestViewHolder(
    private val binding: ItemFollowRequestBinding,
    private val accountListener: AccountActionListener,
    private val linkListener: LinkListener,
    private val showHeader: Boolean
) : RecyclerView.ViewHolder(binding.root), NotificationsViewHolder {

    override fun bind(
        viewData: NotificationViewData.Concrete,
        payloads: List<*>,
        statusDisplayOptions: TweetDisplayOptions
    ) {
        if (payloads.isNotEmpty()) {
            return
        }
        setupWithAccount(
            viewData.account,
            statusDisplayOptions.animateAvatars,
            statusDisplayOptions.animateEmojis,
            statusDisplayOptions.showBotOverlay
        )
        setupActionListener(accountListener, viewData.account.id)
    }

    fun setupWithAccount(
        account: TimelineAccount,
        animateAvatar: Boolean,
        @Suppress("UNUSED_PARAMETER") animateEmojis: Boolean,
        showBotOverlay: Boolean
    ) {
        val wrappedName = account.name.unicodeWrap()
        binding.displayNameTextView.text = wrappedName
        if (showHeader) {
            val wholeMessage: String = itemView.context.getString(
                R.string.notification_follow_request_format,
                wrappedName
            )
            binding.notificationTextView.text = SpannableString(wholeMessage).apply {
                setSpan(StyleSpan(Typeface.BOLD), 0, wrappedName.length, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
            }
        }
        binding.notificationTextView.visible(showHeader)
        val formattedUsername = itemView.context.getString(
            R.string.post_username_format,
            account.username
        )
        binding.usernameTextView.text = formattedUsername
        if (account.note.isEmpty()) {
            binding.accountNote.hide()
        } else {
            binding.accountNote.show()

            val parsedNote = account.note.parseAsWarpnetHtml()
            setClickableText(binding.accountNote, parsedNote, emptyList(), null, linkListener)
        }
        val avatarRadius = binding.avatar.context.resources.getDimensionPixelSize(
            R.dimen.avatar_radius_48dp
        )
        loadAvatar(account.avatar, binding.avatar, avatarRadius, animateAvatar)
        binding.avatarBadge.visible(showBotOverlay && account.bot)
    }

    fun setupActionListener(listener: AccountActionListener, accountId: String) {
        binding.acceptButton.setOnClickListener {
            val position = bindingAdapterPosition
            if (position != RecyclerView.NO_POSITION) {
                listener.onRespondToFollowRequest(true, accountId, position)
            }
        }
        binding.rejectButton.setOnClickListener {
            val position = bindingAdapterPosition
            if (position != RecyclerView.NO_POSITION) {
                listener.onRespondToFollowRequest(false, accountId, position)
            }
        }
        itemView.setOnClickListener { listener.onViewAccount(accountId) }
        binding.accountNote.setOnClickListener { listener.onViewAccount(accountId) }
    }
}
