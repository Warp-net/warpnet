/* Copyright 2021 Tusky Contributors
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

package com.keylesspalace.tusky.components.search.fragments

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.paging.PagingData
import androidx.paging.compose.LazyPagingItems
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.components.instanceinfo.InstanceInfo
import com.keylesspalace.tusky.db.entity.AccountEntity
import com.keylesspalace.tusky.entity.HashTag
import com.keylesspalace.tusky.ui.tuskyColors
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.flow.Flow

@AndroidEntryPoint
class SearchHashtagsFragment : SearchFragment<HashTag>() {

    override val data: Flow<PagingData<HashTag>>
        get() = viewModel.hashtagsFlow

    override fun LazyListScope.searchResult(
        result: LazyPagingItems<HashTag>,
        instanceInfo: InstanceInfo,
        accounts: List<AccountEntity>
    ) {
        items(
            count = result.itemCount,
            itemContent = { index ->
                result[index]?.let { hashtag ->
                    Column {
                        Box(
                            Modifier
                                .heightIn(min = 48.dp)
                                .fillMaxWidth()
                                .clickable {
                                    onViewTag(hashtag.name)
                                }
                                .padding(horizontal = 16.dp, vertical = 8.dp)
                        ) {
                            Text(
                                text = stringResource(R.string.hashtag_format, hashtag.name),
                                fontWeight = FontWeight.Bold,
                                fontSize = 16.sp,
                                color = tuskyColors.primaryTextColor,
                                modifier = Modifier
                                    .align(Alignment.CenterStart)
                            )
                        }
                        HorizontalDivider()
                    }
                }
            }
        )
    }

    companion object {
        fun newInstance() = SearchHashtagsFragment()
    }
}
