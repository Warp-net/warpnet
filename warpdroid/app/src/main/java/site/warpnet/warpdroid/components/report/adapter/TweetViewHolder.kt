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

import android.text.Spanned
import android.text.TextUtils
import android.view.View
import androidx.recyclerview.widget.RecyclerView
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.report.model.TweetViewState
import site.warpnet.warpdroid.databinding.ItemReportTweetBinding
import site.warpnet.warpdroid.entity.Emoji
import site.warpnet.warpdroid.entity.HashTag
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.interfaces.LinkListener
import site.warpnet.warpdroid.util.AbsoluteTimeFormatter
import site.warpnet.warpdroid.util.TweetDisplayOptions
import site.warpnet.warpdroid.util.TweetViewHelper
import site.warpnet.warpdroid.util.TweetViewHelper.Companion.COLLAPSE_INPUT_FILTER
import site.warpnet.warpdroid.util.TweetViewHelper.Companion.NO_INPUT_FILTER
import site.warpnet.warpdroid.util.emojify
import site.warpnet.warpdroid.util.getRelativeTimeSpanString
import site.warpnet.warpdroid.util.hide
import site.warpnet.warpdroid.util.parseAsWarpnetHtml
import site.warpnet.warpdroid.util.setClickableMentions
import site.warpnet.warpdroid.util.setClickableText
import site.warpnet.warpdroid.util.shouldTrimStatus
import site.warpnet.warpdroid.util.show
import site.warpnet.warpdroid.viewdata.TweetViewData
import java.util.Date

class TweetViewHolder(
    private val binding: ItemReportTweetBinding,
    private val statusDisplayOptions: TweetDisplayOptions,
    private val viewState: TweetViewState,
    private val adapterHandler: AdapterHandler,
    private val getStatusForPosition: (Int) -> TweetViewData.Concrete?
) : RecyclerView.ViewHolder(binding.root) {

    private val mediaViewHeight = itemView.context.resources.getDimensionPixelSize(
        R.dimen.tweet_media_preview_height
    )
    private val statusViewHelper = TweetViewHelper(itemView)
    private val absoluteTimeFormatter = AbsoluteTimeFormatter()

    private val previewListener = object : TweetViewHelper.MediaPreviewListener {
        override fun onViewMedia(v: View?, idx: Int) {
            viewdata()?.let { viewdata ->
                adapterHandler.showMedia(v, viewdata, idx)
            }
        }

        override fun onContentHiddenChange(isShowing: Boolean) {
            viewdata()?.id?.let { id ->
                viewState.setMediaShow(id, isShowing)
            }
        }
    }

    init {
        binding.tweetSelection.setOnCheckedChangeListener { _, isChecked ->
            viewdata()?.let { viewdata ->
                adapterHandler.setStatusChecked(viewdata.status, isChecked)
            }
        }
        binding.tweetMediaPreviewContainer.clipToOutline = true
    }

    fun bind(viewData: TweetViewData.Concrete) {
        binding.tweetSelection.isChecked = adapterHandler.isStatusChecked(viewData.id)

        updateTextView()

        val sensitive = viewData.status.sensitive

        statusViewHelper.setMediasPreview(
            statusDisplayOptions,
            viewData.status.attachments,
            sensitive,
            previewListener,
            viewState.isMediaShow(viewData.id, viewData.status.sensitive),
            mediaViewHeight
        )

        setCreatedAt(viewData.status.createdAt)
    }

    private fun updateTextView() {
        viewdata()?.let { viewdata ->
            val content = viewdata.actionable.content.parseAsWarpnetHtml()
            setupCollapsedState(
                shouldTrimStatus(content),
                viewState.isCollapsed(viewdata.id, true),
                viewState.isContentShow(viewdata.id, viewdata.status.sensitive),
                viewdata.status.spoilerText
            )

            if (viewdata.status.spoilerText.isBlank()) {
                setTextVisible(
                    true,
                    content,
                    viewdata.status.mentions,
                    viewdata.status.tags,
                    viewdata.status.emojis,
                    adapterHandler
                )
                binding.tweetContentWarningButton.hide()
                binding.tweetContentWarningDescription.hide()
            } else {
                val emojiSpoiler = viewdata.status.spoilerText.emojify(
                    viewdata.status.emojis,
                    binding.tweetContentWarningDescription,
                    statusDisplayOptions.animateEmojis
                )
                binding.tweetContentWarningDescription.text = emojiSpoiler
                binding.tweetContentWarningDescription.show()
                binding.tweetContentWarningButton.show()
                setContentWarningButtonText(viewState.isContentShow(viewdata.id, true))
                binding.tweetContentWarningButton.setOnClickListener {
                    viewdata()?.let { viewdata ->
                        val contentShown = viewState.isContentShow(viewdata.id, true)
                        binding.tweetContentWarningDescription.invalidate()
                        viewState.setContentShow(viewdata.id, !contentShown)
                        setTextVisible(
                            !contentShown,
                            content,
                            viewdata.status.mentions,
                            viewdata.status.tags,
                            viewdata.status.emojis,
                            adapterHandler
                        )
                        setContentWarningButtonText(!contentShown)
                    }
                }
                setTextVisible(
                    viewState.isContentShow(viewdata.id, true),
                    content,
                    viewdata.status.mentions,
                    viewdata.status.tags,
                    viewdata.status.emojis,
                    adapterHandler
                )
            }
        }
    }

    private fun setContentWarningButtonText(contentShown: Boolean) {
        if (contentShown) {
            binding.tweetContentWarningButton.setText(R.string.post_content_warning_show_less)
        } else {
            binding.tweetContentWarningButton.setText(R.string.post_content_warning_show_more)
        }
    }

    private fun setTextVisible(
        expanded: Boolean,
        content: Spanned,
        mentions: List<Tweet.Mention>,
        tags: List<HashTag>?,
        emojis: List<Emoji>,
        listener: LinkListener
    ) {
        if (expanded) {
            val emojifiedText = content.emojify(
                emojis,
                binding.tweetContent,
                statusDisplayOptions.animateEmojis
            )
            setClickableText(binding.tweetContent, emojifiedText, mentions, tags, listener)
        } else {
            setClickableMentions(binding.tweetContent, mentions, listener)
        }
        if (binding.tweetContent.text.isNullOrBlank()) {
            binding.tweetContent.hide()
        } else {
            binding.tweetContent.show()
        }
    }

    private fun setCreatedAt(createdAt: Date?) {
        if (statusDisplayOptions.useAbsoluteTime) {
            binding.timestampInfo.text = absoluteTimeFormatter.format(createdAt)
        } else {
            binding.timestampInfo.text = if (createdAt != null) {
                val then = createdAt.time
                val now = System.currentTimeMillis()
                getRelativeTimeSpanString(binding.timestampInfo.context, then, now)
            } else {
                // unknown minutes~
                "?m"
            }
        }
    }

    private fun setupCollapsedState(
        collapsible: Boolean,
        collapsed: Boolean,
        expanded: Boolean,
        spoilerText: String
    ) {
        /* input filter for TextViews have to be set before text */
        if (collapsible && (expanded || TextUtils.isEmpty(spoilerText))) {
            binding.buttonToggleContent.setOnClickListener {
                viewdata()?.let { viewdata ->
                    viewState.setCollapsed(viewdata.id, !collapsed)
                    updateTextView()
                }
            }

            binding.buttonToggleContent.show()
            if (collapsed) {
                binding.buttonToggleContent.setText(R.string.post_content_show_more)
                binding.tweetContent.filters = COLLAPSE_INPUT_FILTER
            } else {
                binding.buttonToggleContent.setText(R.string.post_content_show_less)
                binding.tweetContent.filters = NO_INPUT_FILTER
            }
        } else {
            binding.buttonToggleContent.hide()
            binding.tweetContent.filters = NO_INPUT_FILTER
        }
    }

    private fun viewdata() = getStatusForPosition(bindingAdapterPosition)
}
