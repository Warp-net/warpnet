/* Copyright 2025 Tusky Contributors
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

package com.keylesspalace.tusky.ui.statuscomponents

import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.TuskyButtonSize
import com.keylesspalace.tusky.ui.TuskyOutlinedButton
import com.keylesspalace.tusky.ui.TuskyTextButton
import com.keylesspalace.tusky.ui.preferences.LocalAccount
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.statuscomponents.text.emojify
import com.keylesspalace.tusky.ui.statuscomponents.text.mastodonHtmlText
import com.keylesspalace.tusky.ui.statuscomponents.text.toAnnotatedString
import com.keylesspalace.tusky.ui.statuscomponents.text.toInlineContent
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.util.localeNameForUntrustedISO639LangCode
import com.keylesspalace.tusky.viewdata.StatusViewData
import com.keylesspalace.tusky.viewdata.TranslationViewData

@Composable
fun ColumnScope.StatusContent(
    statusViewData: StatusViewData.Concrete,
    listener: StatusActionListener
) {
    val status = statusViewData.actionable

    var isExpanded by remember(statusViewData.isExpanded) { mutableStateOf(statusViewData.isExpanded) }

    val (content, trailingHashTags) = if (isExpanded || status.spoilerText.isEmpty()) {
        mastodonHtmlText(
            status = statusViewData,
            onMentionClick = { accountId -> listener.onViewAccount(accountId) },
            onHashtagClick = { tag -> listener.onViewTag(tag) },
            onUrlClick = { url -> listener.onViewUrl(url) },
            splitOffTrailingHashtags = true
        )
    } else {
        status.mentions.toAnnotatedString(
            onMentionClick = { accountId -> listener.onViewAccount(accountId) }
        ) to emptyList()
    }

    statusViewData.translation?.let { translation ->
        when (translation) {
            TranslationViewData.Loading -> {
                Text(
                    text = stringResource(R.string.label_translating),
                    style = LocalPreferences.current.statusTextStyles.small,
                    color = tuskyColors.tertiaryTextColor,
                    modifier = Modifier.padding(top = 6.dp)
                )
            }

            is TranslationViewData.Loaded -> {
                val langName = localeNameForUntrustedISO639LangCode(translation.data.detectedSourceLanguage)

                Text(
                    text = stringResource(R.string.label_translated, langName, translation.data.provider),
                    style = LocalPreferences.current.statusTextStyles.small,
                    color = tuskyColors.tertiaryTextColor,
                    modifier = Modifier.padding(top = 6.dp)
                )

                TuskyTextButton(
                    text = stringResource(R.string.action_show_original),
                    size = TuskyButtonSize.Small,
                    onClick = {
                        listener.onUntranslate(statusViewData)
                    }
                )
            }
        }
    }

    if (status.spoilerText.isNotEmpty()) {
        val spoilerText = statusViewData.translation?.data?.spoilerText ?: status.spoilerText
        val spoilerDescription = stringResource(R.string.description_post_cw, spoilerText)
        val contentHiddenDescription = stringResource(R.string.content_hidden_description)
        Text(
            text = spoilerText.emojify(status.emojis),
            color = tuskyColors.primaryTextColor,
            style = LocalPreferences.current.statusTextStyles.medium,
            inlineContent = status.emojis.toInlineContent(),
            modifier = Modifier
                .padding(top = 6.dp)
                .semantics {
                    contentDescription = spoilerDescription
                }
        )
        TuskyOutlinedButton(
            text = if (isExpanded) {
                stringResource(R.string.post_content_warning_show_less)
            } else {
                stringResource(R.string.post_content_warning_show_more)
            },
            onClick = {
                isExpanded = !isExpanded
                listener.onExpandedChange(statusViewData, isExpanded)
            },
            size = TuskyButtonSize.Small,
            modifier = Modifier
                .widthIn(min = 150.dp)
                .padding(top = 6.dp)
                .clearAndSetSemantics {
                    if (!isExpanded) {
                        contentDescription = contentHiddenDescription
                    }
                }
        )
    }

    var isCollapsed by remember(statusViewData.isCollapsed) { mutableStateOf(statusViewData.isCollapsed) }

    StatusText(
        content = content,
        status = statusViewData,
        isCollapsed = isCollapsed,
        isExpanded = isExpanded,
        textColor = tuskyColors.primaryTextColor,
        onContentCollapsedChange = {
            isCollapsed = !isCollapsed
            listener.onContentCollapsedChange(statusViewData, isCollapsed)
        },
        modifier = Modifier
            .padding(top = 6.dp)
            .fillMaxWidth()
    )

    MediaAttachments(
        attachments = statusViewData.attachments,
        onOpenAttachment = { index -> listener.onViewMedia(statusViewData, index) },
        onMediaHiddenChanged = { listener.onContentHiddenChange(statusViewData, !statusViewData.isShowingContent) },
        sensitive = status.sensitive,
        isStatusExpanded = status.spoilerText.isEmpty() || isExpanded,
        showMedia = statusViewData.isShowingContent,
        downloadPreviews = LocalAccount.current?.mediaPreviewEnabled ?: true,
        showBlurhash = LocalPreferences.current.useBlurhash,
        filter = statusViewData.filter,
        modifier = Modifier.padding(top = 6.dp)
    )

    Poll(
        statusViewData = statusViewData,
        isExpanded = isExpanded,
        listener = listener
    )

    Quote(
        statusViewData = statusViewData,
        isExpanded = isExpanded,
        listener = listener,
        modifier = Modifier.padding(top = 6.dp)
    )

    LinkPreviewCard(
        statusViewData = statusViewData,
        isExpanded = isExpanded,
        listener = listener
    )

    if (trailingHashTags.isNotEmpty()) {
        Hashtags(
            hashtags = trailingHashTags,
            singleLine = !statusViewData.isDetailed,
            style = if (statusViewData.isDetailed) {
                LocalPreferences.current.statusTextStyles.large
            } else {
                LocalPreferences.current.statusTextStyles.medium
            },
            listener = listener,
            modifier = Modifier.padding(top = 6.dp)
        )
    }
}
