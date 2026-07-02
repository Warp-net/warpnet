/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Instagram-style account-recommendation block embedded in the home timeline:
 * a horizontally scrolling row of suggested accounts, each with a Follow
 * button. Sits inside the vertical timeline LazyColumn as one item.
 */
package site.warpnet.warpdroid.components.timeline

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.entity.TimelineUser
import site.warpnet.warpdroid.ui.tweetcomponents.Avatar
import site.warpnet.warpdroid.ui.warpdroidColors

@Composable
fun WhoToFollowCarousel(
    accounts: List<TimelineUser>,
    followedIds: Set<String>,
    onOpenProfile: (String) -> Unit,
    onFollow: (TimelineUser) -> Unit,
    modifier: Modifier = Modifier,
) {
    if (accounts.isEmpty()) return
    Column(modifier = modifier.fillMaxWidth().padding(vertical = 8.dp)) {
        Text(
            text = stringResource(R.string.who_to_follow),
            fontWeight = FontWeight.Bold,
            color = warpdroidColors.primaryTextColor,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
        LazyRow(
            contentPadding = PaddingValues(horizontal = 12.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            items(accounts, key = { it.id }) { account ->
                WhoToFollowCard(
                    account = account,
                    isFollowed = account.id in followedIds,
                    onOpenProfile = { onOpenProfile(account.id) },
                    onFollow = { onFollow(account) },
                )
            }
        }
        HorizontalDivider(modifier = Modifier.padding(top = 8.dp))
    }
}

@Composable
private fun WhoToFollowCard(
    account: TimelineUser,
    isFollowed: Boolean,
    onOpenProfile: () -> Unit,
    onFollow: () -> Unit,
) {
    Card(onClick = onOpenProfile, modifier = Modifier.width(150.dp)) {
        Column(
            modifier = Modifier.fillMaxWidth().padding(12.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Avatar(
                url = account.avatar,
                staticUrl = account.staticAvatar,
                isBot = account.bot,
                retweetedAvatarUrl = null,
                staticRetweetedAvatarUrl = null,
                onOpenProfile = onOpenProfile,
            )
            Text(
                text = account.name,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.fillMaxWidth(),
            )
            Text(
                text = "@" + account.username,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.fillMaxWidth(),
            )
            if (isFollowed) {
                OutlinedButton(
                    onClick = {},
                    enabled = false,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text(stringResource(R.string.action_following))
                }
            } else {
                Button(onClick = onFollow, modifier = Modifier.fillMaxWidth()) {
                    Text(stringResource(R.string.action_follow))
                }
            }
        }
    }
}
