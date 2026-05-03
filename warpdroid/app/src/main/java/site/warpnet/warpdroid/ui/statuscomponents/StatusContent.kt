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

package site.warpnet.warpdroid.ui.statuscomponents

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
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.interfaces.StatusActionListener
import site.warpnet.warpdroid.ui.WarpdroidButtonSize
import site.warpnet.warpdroid.ui.WarpdroidOutlinedButton
import site.warpnet.warpdroid.ui.WarpdroidTextButton
import site.warpnet.warpdroid.ui.preferences.LocalAccount
import site.warpnet.warpdroid.ui.preferences.LocalPreferences
import site.warpnet.warpdroid.ui.statuscomponents.text.emojify
import site.warpnet.warpdroid.ui.statuscomponents.text.warpnetHtmlText
import site.warpnet.warpdroid.ui.statuscomponents.text.toAnnotatedString
import site.warpnet.warpdroid.ui.statuscomponents.text.toInlineContent
import site.warpnet.warpdroid.ui.warpdroidColors
import site.warpnet.warpdroid.util.localeNameForUntrustedISO639LangCode
import site.warpnet.warpdroid.viewdata.StatusViewData
import site.warpnet.warpdroid.viewdata.TranslationViewData

@Composable
fun ColumnScope.StatusContent(
    statusViewData: StatusViewData.Concrete,
    listener: StatusActionListener
) {
    val status = statusViewData.actionable

    var isExpanded by remember(statusViewData.isExpanded) { mutableStateOf(statusViewData.isExpanded) }

    val (content, trailingHashTags) = if (isExpanded || status.spoilerText.isEmpty()) {
        warpnetHtmlText(
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
                    color = warpdroidColors.tertiaryTextColor,
                    modifier = Modifier.padding(top = 6.dp)
                )
            }

            is TranslationViewData.Loaded -> {
                val langName = localeNameForUntrustedISO639LangCode(translation.data.detectedSourceLanguage)

                Text(
                    text = stringResource(R.string.label_translated, langName, translation.data.provider),
                    style = LocalPreferences.current.statusTextStyles.small,
                    color = warpdroidColors.tertiaryTextColor,
                    modifier = Modifier.padding(top = 6.dp)
                )

                WarpdroidTextButton(
                    text = stringResource(R.string.action_show_original),
                    size = WarpdroidButtonSize.Small,
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
            color = warpdroidColors.primaryTextColor,
            style = LocalPreferences.current.statusTextStyles.medium,
            inlineContent = status.emojis.toInlineContent(),
            modifier = Modifier
                .padding(top = 6.dp)
                .semantics {
                    contentDescription = spoilerDescription
                }
        )
        WarpdroidOutlinedButton(
            text = if (isExpanded) {
                stringResource(R.string.post_content_warning_show_less)
            } else {
                stringResource(R.string.post_content_warning_show_more)
            },
            onClick = {
                isExpanded = !isExpanded
                listener.onExpandedChange(statusViewData, isExpanded)
            },
            size = WarpdroidButtonSize.Small,
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
        textColor = warpdroidColors.primaryTextColor,
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
