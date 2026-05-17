package site.warpnet.warpdroid.adapter

import androidx.recyclerview.widget.RecyclerView
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ItemAccountBinding
import site.warpnet.warpdroid.entity.TimelineAccount
import site.warpnet.warpdroid.interfaces.AccountActionListener
import site.warpnet.warpdroid.interfaces.LinkListener
import site.warpnet.warpdroid.util.emojify
import site.warpnet.warpdroid.util.loadAvatar
import site.warpnet.warpdroid.util.visible

class AccountViewHolder(
    private val binding: ItemAccountBinding
) : RecyclerView.ViewHolder(binding.root) {
    private lateinit var accountId: String

    fun setupWithAccount(
        account: TimelineAccount,
        animateAvatar: Boolean,
        animateEmojis: Boolean,
        showBotOverlay: Boolean
    ) {
        accountId = account.id

        binding.accountUsername.text = binding.accountUsername.context.getString(
            R.string.post_username_format,
            account.username
        )

        val emojifiedName = account.name.emojify(
            account.emojis,
            binding.accountDisplayName,
            animateEmojis
        )
        binding.accountDisplayName.text = emojifiedName

        val avatarRadius = binding.accountAvatar.context.resources
            .getDimensionPixelSize(R.dimen.avatar_radius_48dp)
        loadAvatar(account.avatar, binding.accountAvatar, avatarRadius, animateAvatar)

        binding.accountBotBadge.visible(showBotOverlay && account.bot)
    }

    fun setupActionListener(listener: AccountActionListener) {
        itemView.setOnClickListener { listener.onViewAccount(accountId) }
    }

    fun setupLinkListener(listener: LinkListener) {
        itemView.setOnClickListener {
            listener.onViewAccount(
                accountId
            )
        }
    }
}
