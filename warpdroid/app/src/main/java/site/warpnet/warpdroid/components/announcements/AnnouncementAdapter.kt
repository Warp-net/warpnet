/* Copyright 2020 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.announcements

import android.annotation.SuppressLint
import android.graphics.drawable.Drawable
import android.os.Build
import android.text.SpannableString
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import androidx.core.view.size
import androidx.recyclerview.widget.RecyclerView
import com.bumptech.glide.Glide
import com.bumptech.glide.request.target.Target
import com.google.android.material.chip.Chip
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ItemAnnouncementBinding
import site.warpnet.warpdroid.entity.Announcement
import site.warpnet.warpdroid.interfaces.LinkListener
import site.warpnet.warpdroid.util.AbsoluteTimeFormatter
import site.warpnet.warpdroid.util.BindingHolder
import site.warpnet.warpdroid.util.EmojiSpan
import site.warpnet.warpdroid.util.clearEmojiTargets
import site.warpnet.warpdroid.util.emojify
import site.warpnet.warpdroid.util.parseAsWarpnetHtml
import site.warpnet.warpdroid.util.setClickableText
import site.warpnet.warpdroid.util.setEmojiTargets
import site.warpnet.warpdroid.util.visible

interface AnnouncementActionListener : LinkListener {
    fun openReactionPicker(announcementId: String, target: View)
    fun addReaction(announcementId: String, name: String)
    fun removeReaction(announcementId: String, name: String)
}

class AnnouncementAdapter(
    private var items: List<Announcement> = emptyList(),
    private val listener: AnnouncementActionListener,
    private val wellbeingEnabled: Boolean = false,
    private val animateEmojis: Boolean = false
) : RecyclerView.Adapter<BindingHolder<ItemAnnouncementBinding>>() {

    private val absoluteTimeFormatter = AbsoluteTimeFormatter()

    override fun onCreateViewHolder(
        parent: ViewGroup,
        viewType: Int
    ): BindingHolder<ItemAnnouncementBinding> {
        val binding = ItemAnnouncementBinding.inflate(
            LayoutInflater.from(parent.context),
            parent,
            false
        )
        return BindingHolder(binding)
    }

    @SuppressLint("SetTextI18n")
    override fun onBindViewHolder(holder: BindingHolder<ItemAnnouncementBinding>, position: Int) {
        val item = items[position]

        holder.binding.announcementDate.text = absoluteTimeFormatter.format(item.publishedAt, false)

        val text = holder.binding.text
        val chips = holder.binding.chipGroup
        val addReactionChip = holder.binding.addReactionChip

        val emojifiedText: CharSequence = item.content.parseAsWarpnetHtml().emojify(
            item.emojis,
            text,
            animateEmojis
        )

        setClickableText(text, emojifiedText, item.mentions, item.tags, listener)

        // If wellbeing mode is enabled, announcement badge counts should not be shown.
        if (wellbeingEnabled) {
            // Since reactions are not visible in wellbeing mode,
            // we shouldn't be able to add any ourselves.
            addReactionChip.visibility = View.GONE
            return
        }

        // hide button if announcement badge limit is already reached
        addReactionChip.visible(item.reactions.size < 8)

        val requestManager = Glide.with(chips)

        chips.clearEmojiTargets()
        val targets = ArrayList<Target<Drawable>>(item.reactions.size)

        item.reactions.forEachIndexed { i, reaction ->
            (
                chips.getChildAt(i)?.takeUnless { it.id == R.id.addReactionChip } as Chip?
                    ?: Chip(chips.context).apply {
                        isCheckable = true
                        checkedIcon = null
                        isCloseIconVisible = false
                        setChipBackgroundColorResource(R.color.selectable_chip_background)
                        chips.addView(this, i)
                    }
                )
                .apply {
                    if (reaction.url == null) {
                        this.text = "${reaction.name} ${reaction.count}"
                    } else {
                        // we set the EmojiSpan on a space, because otherwise the Chip won't have the right size
                        // https://github.com/tuskyapp/Tusky/issues/2308
                        val spannable = SpannableString("  ${reaction.count}")
                        val span = EmojiSpan(this)
                        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                            span.contentDescription = reaction.name
                        }
                        val target = span.createGlideTarget(this, animateEmojis)
                        spannable.setSpan(span, 0, 1, 0)
                        requestManager
                            .asDrawable()
                            .load(
                                if (animateEmojis) {
                                    reaction.url
                                } else {
                                    reaction.staticUrl
                                }
                            )
                            .into(target)
                        targets.add(target)
                        this.text = spannable
                    }

                    isChecked = reaction.me

                    setOnClickListener {
                        if (reaction.me) {
                            listener.removeReaction(item.id, reaction.name)
                        } else {
                            listener.addReaction(item.id, reaction.name)
                        }
                    }
                }
        }

        while (chips.size - 1 > item.reactions.size) {
            chips.removeViewAt(item.reactions.size)
        }

        // Store Glide targets for later cancellation
        chips.setEmojiTargets(targets)

        addReactionChip.setOnClickListener {
            listener.openReactionPicker(item.id, it)
        }
    }

    override fun onViewRecycled(holder: BindingHolder<ItemAnnouncementBinding>) {
        holder.binding.chipGroup.clearEmojiTargets()
    }

    override fun getItemCount() = items.size

    fun updateList(items: List<Announcement>) {
        this.items = items
        notifyDataSetChanged()
    }
}
