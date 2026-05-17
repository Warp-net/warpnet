/* Copyright 2017 Andrew Dawson
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

package site.warpnet.warpdroid.components.accountlist.adapter

import android.view.LayoutInflater
import android.view.ViewGroup
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ItemBlockedUserBinding
import site.warpnet.warpdroid.interfaces.AccountActionListener
import site.warpnet.warpdroid.util.BindingHolder
import site.warpnet.warpdroid.util.emojify
import site.warpnet.warpdroid.util.loadAvatar
import site.warpnet.warpdroid.util.visible

/** Displays a list of blocked accounts. */
class BlocksAdapter(
    accountActionListener: AccountActionListener,
    animateAvatar: Boolean,
    animateEmojis: Boolean,
    showBotOverlay: Boolean
) : AccountAdapter<BindingHolder<ItemBlockedUserBinding>>(
    accountActionListener = accountActionListener,
    animateAvatar = animateAvatar,
    animateEmojis = animateEmojis,
    showBotOverlay = showBotOverlay
) {

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): BindingHolder<ItemBlockedUserBinding> {
        return BindingHolder(
            ItemBlockedUserBinding.inflate(LayoutInflater.from(parent.context), parent, false)
        )
    }

    override fun onBindViewHolder(viewHolder: BindingHolder<ItemBlockedUserBinding>, position: Int) {
        getItem(position)?.let { viewData ->
            val account = viewData.account
            val binding = viewHolder.binding
            val context = binding.root.context

            val emojifiedName = account.name.emojify(
                account.emojis,
                binding.blockedUserDisplayName,
                animateEmojis
            )
            binding.blockedUserDisplayName.text = emojifiedName
            val formattedUsername = context.getString(R.string.post_username_format, account.username)
            binding.blockedUserUsername.text = formattedUsername

            val avatarRadius = context.resources.getDimensionPixelSize(R.dimen.avatar_radius_48dp)
            loadAvatar(account.avatar, binding.blockedUserAvatar, avatarRadius, animateAvatar)

            binding.blockedUserBotBadge.visible(showBotOverlay && account.bot)

            binding.blockedUserUnblock.setOnClickListener {
                accountActionListener.onBlock(false, account.id, position)
            }
            binding.root.setOnClickListener {
                accountActionListener.onViewAccount(account.id)
            }
        }
    }
}
