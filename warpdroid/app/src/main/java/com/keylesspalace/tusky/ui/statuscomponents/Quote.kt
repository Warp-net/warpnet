/* Copyright 2026 Tusky Contributors
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

import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.hideFromAccessibility
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.entity.Filter
import com.keylesspalace.tusky.entity.Quote
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.preferences.textStyle
import com.keylesspalace.tusky.ui.statuscomponents.text.emojify
import com.keylesspalace.tusky.ui.statuscomponents.text.toInlineContent
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.ui.tuskyDefaultCornerShape
import com.keylesspalace.tusky.ui.tuskyDefaultRadius
import com.keylesspalace.tusky.util.getDomain
import com.keylesspalace.tusky.viewdata.StatusViewData

@Composable
fun Quote(
    statusViewData: StatusViewData.Concrete,
    isExpanded: Boolean,
    listener: StatusActionListener,
    modifier: Modifier = Modifier
) {
    val quote = statusViewData.quote

    if (quote == null || statusViewData.status.spoilerText.isNotEmpty() && !isExpanded) {
        return
    }

    when (quote.state) {
        Quote.State.PENDING -> {
            QuotePlaceholder(
                stringResource(R.string.quote_pending),
                modifier = modifier
            )
        }

        Quote.State.REVOKED -> {
            QuotePlaceholder(
                stringResource(R.string.quote_revoked),
                modifier = modifier
            )
        }

        Quote.State.REJECTED, Quote.State.DELETED, Quote.State.UNAUTHORIZED -> {
            QuotePlaceholder(
                stringResource(R.string.quote_unavailable),
                modifier = modifier
            )
        }

        Quote.State.ACCEPTED -> {
            QuoteContent(
                quotedStatusViewData = quote.quotedStatusViewData,
                listener = listener,
                modifier = modifier
            )
        }

        Quote.State.BLOCKED_ACCOUNT -> {
            if (quote.quoteShown || quote.quotedStatusViewData == null) {
                QuoteContent(
                    quotedStatusViewData = quote.quotedStatusViewData,
                    listener = listener,
                    modifier = modifier
                )
            } else {
                QuoteHidden(
                    text = stringResource(R.string.quote_account_blocked, "@" + quote.quotedStatusViewData.status.account.username),
                    onShowQuote = { listener.onShowQuote(statusViewData) },
                    modifier = modifier,
                )
            }
        }

        Quote.State.MUTED_ACCOUNT -> {
            if (quote.quoteShown || quote.quotedStatusViewData == null) {
                QuoteContent(
                    quotedStatusViewData = quote.quotedStatusViewData,
                    listener = listener
                )
            } else {
                QuoteHidden(
                    text = stringResource(R.string.quote_account_muted, "@" + quote.quotedStatusViewData.status.account.username),
                    onShowQuote = { listener.onShowQuote(statusViewData) },
                    modifier = modifier,
                )
            }
        }

        Quote.State.BLOCKED_DOMAIN -> {
            if (quote.quoteShown || quote.quotedStatusViewData == null) {
                QuoteContent(
                    quotedStatusViewData = quote.quotedStatusViewData,
                    listener = listener,
                    modifier = modifier
                )
            } else {
                QuoteHidden(
                    text = stringResource(R.string.quote_domain_blocked, getDomain(quote.quotedStatusViewData.status.account.url)),
                    onShowQuote = { listener.onShowQuote(statusViewData) },
                    modifier = modifier,
                )
            }
        }
    }
}

@Composable
private fun QuoteContent(
    quotedStatusViewData: StatusViewData.Concrete?,
    listener: StatusActionListener,
    modifier: Modifier = Modifier
) {
    if (quotedStatusViewData == null) {
        ShallowQuote(modifier)
    } else if (quotedStatusViewData.filter?.action == Filter.Action.HIDE) {
        QuotePlaceholder(
            stringResource(R.string.quote_hidden_by_filter),
            modifier = modifier
        )
    } else if (quotedStatusViewData.filterActive && quotedStatusViewData.filter?.action == Filter.Action.WARN) {
        QuoteHidden(
            stringResource(R.string.status_filter_placeholder_label_format, quotedStatusViewData.filter.title),
            onShowQuote = {
                listener.changeFilter(quotedStatusViewData, filtered = false)
            },
            modifier = modifier,
        )
    } else {
        val quotedStatus = quotedStatusViewData.status

        Column(
            modifier
                .fillMaxWidth()
                .border(
                    width = 1.dp,
                    color = tuskyColors.backgroundAccent,
                    shape = RoundedCornerShape(tuskyDefaultRadius + 8.dp)
                )
                .clip(RoundedCornerShape(tuskyDefaultRadius + 8.dp))
                .clickable {
                    listener.onViewThread(quotedStatusViewData)
                }
                .padding(8.dp)
        ) {
            Row {
                Avatar(
                    url = quotedStatus.account.avatar,
                    staticUrl = quotedStatus.account.staticAvatar,
                    isBot = quotedStatus.account.bot,
                    boostedAvatarUrl = null,
                    staticBoostedAvatarUrl = null,
                    onOpenProfile = {
                        listener.onViewAccount(quotedStatus.account.id)
                    },
                    modifier = Modifier
                        .size(40.dp)
                        .clearAndSetSemantics {
                            hideFromAccessibility()
                        }
                )

                Spacer(modifier = Modifier.width(12.dp))

                Box(
                    modifier = Modifier.weight(1f)
                ) {
                    val username = stringResource(R.string.post_username_format, quotedStatus.account.username)

                    Column(
                        modifier = Modifier
                            .clearAndSetSemantics {
                                contentDescription = "${quotedStatus.account.name} $username"
                            }
                            .clickable {
                                listener.onViewAccount(quotedStatus.account.id)
                            }
                    ) {
                        Text(
                            text = quotedStatus.account.name.emojify(quotedStatus.account.emojis),
                            fontWeight = FontWeight.Bold,
                            color = tuskyColors.primaryTextColor,
                            style = LocalPreferences.current.statusTextStyles.medium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            inlineContent = quotedStatus.account.emojis.toInlineContent()
                        )
                        val username = stringResource(R.string.post_username_format, quotedStatus.account.username)
                        Text(
                            text = username,
                            color = tuskyColors.secondaryTextColor,
                            style = LocalPreferences.current.statusTextStyles.medium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }

                Timestamp(
                    date = quotedStatus.createdAt,
                    isEdited = quotedStatus.editedAt != null,
                    textColor = tuskyColors.secondaryTextColor,
                    modifier = Modifier.padding(horizontal = 4.dp)
                )
            }

            StatusContent(
                statusViewData = quotedStatusViewData,
                listener = listener
            )
        }
    }
}

@Composable
private fun QuotePlaceholder(
    text: String,
    modifier: Modifier = Modifier
) {
    Text(
        text = text,
        color = tuskyColors.secondaryTextColor,
        fontSize = 16.sp,
        modifier = modifier
            .fillMaxWidth()
            .border(
                width = 1.dp,
                color = tuskyColors.backgroundAccent,
                shape = tuskyDefaultCornerShape
            )
            .padding(horizontal = 12.dp, vertical = 8.dp)
    )
}

@Composable
private fun ShallowQuote(modifier: Modifier = Modifier) {
    Text(
        text = stringResource(R.string.quote),
        color = tuskyColors.secondaryTextColor,
        fontWeight = FontWeight.Bold,
        fontSize = 16.sp,
        modifier = modifier
            .fillMaxWidth()
            .border(
                width = 1.dp,
                color = tuskyColors.backgroundAccent,
                shape = tuskyDefaultCornerShape
            )
            .padding(horizontal = 12.dp, vertical = 8.dp)
    )
}

@Composable
private fun QuoteHidden(
    text: String,
    onShowQuote: () -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .border(
                width = 1.dp,
                color = tuskyColors.backgroundAccent,
                shape = RoundedCornerShape(tuskyDefaultRadius + 8.dp)
            )
            .clip(RoundedCornerShape(tuskyDefaultRadius + 8.dp))
            .clickable {
                onShowQuote()
            }
            .padding(8.dp)
    ) {
        Text(
            text = text,
            color = tuskyColors.tertiaryTextColor,
            style = textStyle(16.sp),
            textAlign = TextAlign.Center,
            modifier = Modifier
                .align(Alignment.CenterHorizontally)
                .padding(horizontal = 4.dp)
        )
        Text(
            text = stringResource(R.string.quote_hidden_show_anyway),
            fontSize = 16.sp,
            color = colorScheme.primary,
            fontWeight = FontWeight.Bold,
            textAlign = TextAlign.Center,
            modifier = Modifier
                .align(Alignment.CenterHorizontally)
                .padding(top = 6.dp)
        )
    }
}
