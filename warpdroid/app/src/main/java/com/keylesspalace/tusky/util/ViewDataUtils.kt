/*
 * Copyright 2023 Tusky Contributors
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
 * see <http://www.gnu.org/licenses>.
 */

@file:JvmName("ViewDataUtils")

/* Copyright 2017 Andrew Dawson
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

package com.keylesspalace.tusky.util

import androidx.paging.CombinedLoadStates
import androidx.paging.LoadState
import com.keylesspalace.tusky.entity.Filter
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.entity.TrendingTag
import com.keylesspalace.tusky.viewdata.QuoteViewData
import com.keylesspalace.tusky.viewdata.StatusViewData
import com.keylesspalace.tusky.viewdata.TranslationViewData
import com.keylesspalace.tusky.viewdata.TrendingViewData

fun Status.toViewData(
    isShowingContent: Boolean,
    isExpanded: Boolean,
    isCollapsed: Boolean,
    isDetailed: Boolean = false,
    filterKind: Filter.Kind,
    translation: TranslationViewData? = null,
    filterActive: Boolean,
    isQuoteShowingContent: Boolean,
    isQuoteExpanded: Boolean,
    isQuoteCollapsed: Boolean,
    isQuoteShown: Boolean
): StatusViewData.Concrete = StatusViewData.Concrete(
    status = this,
    isShowingContent = isShowingContent,
    isCollapsed = isCollapsed,
    isExpanded = isExpanded,
    isDetailed = isDetailed,
    translation = translation,
    filter = this.getApplicableFilter(filterKind),
    filterActive = filterActive,
    quote = quote?.let {
        QuoteViewData(
            state = quote.state,
            quotedStatusViewData = quote.quotedStatus?.toViewData(
                isShowingContent = isQuoteShowingContent,
                isExpanded = isQuoteExpanded,
                isCollapsed = isQuoteCollapsed,
                isDetailed = false,
                filterKind = filterKind,
                translation = null,
                filterActive = true,
                isQuoteShowingContent = false,
                isQuoteExpanded = false,
                isQuoteCollapsed = false,
                isQuoteShown = false
            ),
            quoteShown = isQuoteShown
        )
    }
)

fun List<TrendingTag>.toViewData(): List<TrendingViewData.Tag> {
    val maxTrendingValue = flatMap { tag -> tag.history }
        .mapNotNull { it.uses.toLongOrNull() }
        .maxOrNull() ?: 1

    return map { tag ->
        val reversedHistory = tag.history.asReversed()

        TrendingViewData.Tag(
            name = tag.name,
            usage = reversedHistory.mapNotNull { it.uses.toLongOrNull() },
            accounts = reversedHistory.mapNotNull { it.accounts.toLongOrNull() },
            maxTrendingValue = maxTrendingValue
        )
    }
}

fun CombinedLoadStates.isAnyLoading(): Boolean {
    return this.refresh == LoadState.Loading || this.append == LoadState.Loading || this.prepend == LoadState.Loading
}

fun CombinedLoadStates.isRefreshing(): Boolean {
    return this.source.refresh == LoadState.Loading ||
        this.mediator?.refresh == LoadState.Loading ||
        this.source.prepend == LoadState.Loading ||
        this.mediator?.prepend == LoadState.Loading
}
