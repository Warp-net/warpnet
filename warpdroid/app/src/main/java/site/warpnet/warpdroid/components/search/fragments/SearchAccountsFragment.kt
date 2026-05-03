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

package site.warpnet.warpdroid.components.search.fragments

import android.content.SharedPreferences
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.paging.PagingData
import androidx.paging.compose.LazyPagingItems
import site.warpnet.warpdroid.components.instanceinfo.InstanceInfo
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.entity.TimelineAccount
import site.warpnet.warpdroid.ui.preferences.LocalPreferences
import site.warpnet.warpdroid.ui.statuscomponents.Avatar
import site.warpnet.warpdroid.ui.statuscomponents.text.emojify
import site.warpnet.warpdroid.ui.statuscomponents.text.toInlineContent
import site.warpnet.warpdroid.ui.warpdroidColors
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.flow.Flow

@AndroidEntryPoint
class SearchAccountsFragment : SearchFragment<TimelineAccount>() {

    @Inject
    lateinit var preferences: SharedPreferences

    override val data: Flow<PagingData<TimelineAccount>>
        get() = viewModel.accountsFlow

    override fun LazyListScope.searchResult(
        result: LazyPagingItems<TimelineAccount>,
        instanceInfo: InstanceInfo,
        accounts: List<AccountEntity>
    ) {
        items(
            count = result.itemCount,
            itemContent = { index ->
                result[index]?.let { account ->
                    Column {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable {
                                    bottomSheetActivity?.viewAccount(account.id)
                                }
                                .padding(horizontal = 16.dp, vertical = 8.dp)
                        ) {
                            Avatar(
                                url = account.avatar,
                                staticUrl = account.staticAvatar,
                                isBot = account.bot,
                                boostedAvatarUrl = null,
                                staticBoostedAvatarUrl = null,
                                onOpenProfile = null,
                            )
                            Spacer(modifier = Modifier.width(8.dp))
                            Column {
                                Text(
                                    text = account.name.emojify(account.emojis),
                                    fontWeight = FontWeight.Bold,
                                    color = warpdroidColors.primaryTextColor,
                                    style = LocalPreferences.current.statusTextStyles.medium,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                    inlineContent = account.emojis.toInlineContent()
                                )
                                Text(
                                    text = account.username,
                                    color = warpdroidColors.secondaryTextColor,
                                    style = LocalPreferences.current.statusTextStyles.medium,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                            }
                        }
                        HorizontalDivider()
                    }
                }
            }
        )
    }

    companion object {
        fun newInstance() = SearchAccountsFragment()
    }
}
