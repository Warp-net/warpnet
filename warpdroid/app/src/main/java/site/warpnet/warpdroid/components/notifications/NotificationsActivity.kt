/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Simple notifications screen mirroring the Vue frontend's Notifications view.
 * Tabs: All / Mentions / Follow Requests. The Follow Requests tab is only
 * shown when the active account is locked (manually-approve followers).
 */
package site.warpnet.warpdroid.components.notifications

import android.content.Context
import android.content.Intent
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dagger.hilt.android.AndroidEntryPoint
import site.warpnet.warpdroid.BaseActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.account.AccountActivity
import site.warpnet.warpdroid.components.viewthread.ViewThreadActivity
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.TimelineUser
import site.warpnet.warpdroid.ui.WarpdroidTheme
import site.warpnet.warpdroid.util.startActivityWithSlideInAnimation

@AndroidEntryPoint
class NotificationsActivity : BaseActivity() {

    private val viewModel: NotificationsViewModel by viewModels()

    private enum class Tab { ALL, MENTIONS, FOLLOW_REQUESTS }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            WarpdroidTheme {
                NotificationsContent()
            }
        }
    }

    @OptIn(ExperimentalMaterial3Api::class)
    @Composable
    private fun NotificationsContent() {
        val state by viewModel.state.collectAsStateWithLifecycle()
        var rawTab by remember { mutableStateOf(Tab.ALL) }
        // Hide the FOLLOW_REQUESTS tab when the account isn't locked so the
        // view never renders a tab that has no data path behind it.
        val selectedTab = if (!state.locked && rawTab == Tab.FOLLOW_REQUESTS) Tab.ALL else rawTab

        Scaffold(
            contentWindowInsets = WindowInsets(0, 0, 0, 0),
            topBar = {
                TopAppBar(
                    title = { Text(stringResource(R.string.title_notifications)) },
                    navigationIcon = {
                        IconButton(onClick = ::finish) {
                            Icon(
                                painterResource(R.drawable.ic_arrow_back_24dp),
                                stringResource(R.string.button_back),
                            )
                        }
                    },
                )
            },
        ) { contentPadding ->
            Column(
                modifier = Modifier
                    .padding(contentPadding)
                    .fillMaxSize(),
            ) {
                TabBar(
                    selected = selectedTab,
                    showFollowRequests = state.locked,
                    onSelect = { rawTab = it },
                )
                HorizontalDivider()
                Box(modifier = Modifier.fillMaxSize()) {
                    if (state.loading && state.notifications.isEmpty() && state.followRequests.isEmpty()) {
                        CircularProgressIndicator(modifier = Modifier.align(Alignment.Center))
                    } else {
                        when (selectedTab) {
                            Tab.ALL -> NotificationList(state.notifications)
                            Tab.MENTIONS -> NotificationList(
                                state.notifications.filter { it.type is Notification.Type.Mention }
                            )
                            Tab.FOLLOW_REQUESTS -> FollowRequestList(
                                requests = state.followRequests,
                                onAuthorize = viewModel::authorize,
                                onReject = viewModel::reject,
                            )
                        }
                    }
                }
            }
        }
    }

    @Composable
    private fun TabBar(
        selected: Tab,
        showFollowRequests: Boolean,
        onSelect: (Tab) -> Unit,
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceEvenly,
        ) {
            TabButton(stringResource(R.string.notifications_tab_all), selected == Tab.ALL) {
                onSelect(Tab.ALL)
            }
            TabButton(stringResource(R.string.notifications_tab_mentions), selected == Tab.MENTIONS) {
                onSelect(Tab.MENTIONS)
            }
            if (showFollowRequests) {
                TabButton(
                    stringResource(R.string.title_follow_requests),
                    selected == Tab.FOLLOW_REQUESTS,
                ) {
                    onSelect(Tab.FOLLOW_REQUESTS)
                }
            }
        }
    }

    @Composable
    private fun TabButton(label: String, isSelected: Boolean, onClick: () -> Unit) {
        TextButton(onClick = onClick) {
            Text(
                text = label,
                fontWeight = if (isSelected) FontWeight.Bold else FontWeight.Normal,
                color = if (isSelected) colorScheme.primary else colorScheme.onSurface,
            )
        }
    }

    @Composable
    private fun NotificationList(items: List<Notification>) {
        if (items.isEmpty()) {
            EmptyState(stringResource(R.string.notifications_empty))
            return
        }
        LazyColumn(modifier = Modifier.fillMaxSize()) {
            items(items.distinctBy { it.id }, key = { it.id }) { n ->
                NotificationRow(n) { onNotificationClick(n) }
                HorizontalDivider()
            }
        }
    }

    @Composable
    private fun NotificationRow(n: Notification, onClick: () -> Unit) {
        TextButton(
            onClick = onClick,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Column(modifier = Modifier.fillMaxWidth()) {
                // The backend pre-composes the actor + verb into [text]
                // (e.g. "Echo liked your tweet"), surfaced via account.name.
                Text(
                    text = n.account.name,
                    fontWeight = FontWeight.SemiBold,
                )
            }
        }
    }

    @Composable
    private fun FollowRequestList(
        requests: List<TimelineUser>,
        onAuthorize: (String) -> Unit,
        onReject: (String) -> Unit,
    ) {
        if (requests.isEmpty()) {
            EmptyState(stringResource(R.string.follow_requests_empty))
            return
        }
        LazyColumn(modifier = Modifier.fillMaxSize()) {
            items(requests.distinctBy { it.id }, key = { it.id }) { acc ->
                FollowRequestRow(
                    account = acc,
                    onAuthorize = { onAuthorize(acc.id) },
                    onReject = { onReject(acc.id) },
                    onOpen = {
                        startActivityWithSlideInAnimation(
                            AccountActivity.newIntent(this@NotificationsActivity, acc.id),
                        )
                    },
                )
                HorizontalDivider()
            }
        }
    }

    @Composable
    private fun FollowRequestRow(
        account: TimelineUser,
        onAuthorize: () -> Unit,
        onReject: () -> Unit,
        onOpen: () -> Unit,
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f)) {
                TextButton(onClick = onOpen) {
                    Text(text = account.name, fontWeight = FontWeight.Bold)
                }
                Text(text = "@${account.id}", color = Color.Gray)
            }
            Button(
                onClick = onAuthorize,
                colors = ButtonDefaults.buttonColors(containerColor = colorScheme.primary),
            ) {
                Text(stringResource(R.string.action_accept))
            }
            Spacer(modifier = Modifier.width(8.dp))
            OutlinedButton(onClick = onReject) {
                Text(stringResource(R.string.action_reject))
            }
        }
    }

    @Composable
    private fun EmptyState(message: String) {
        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Text(text = message)
        }
    }

    private fun onNotificationClick(n: Notification) {
        val tweet = n.status
        if (tweet != null) {
            startActivityWithSlideInAnimation(
                ViewThreadActivity.newIntent(this, tweet.id, tweet.url ?: "", n.account.id),
            )
        } else {
            startActivityWithSlideInAnimation(AccountActivity.newIntent(this, n.account.id))
        }
    }

    companion object {
        fun newIntent(context: Context): Intent =
            Intent(context, NotificationsActivity::class.java)
    }
}
