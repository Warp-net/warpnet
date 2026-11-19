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
import androidx.annotation.DrawableRes
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
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
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withLink
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.lifecycleScope
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.launch
import site.warpnet.warpdroid.BaseActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.account.AccountActivity
import site.warpnet.warpdroid.components.accountlist.AccountListActivity
import site.warpnet.warpdroid.components.chats.ChatMessagesActivity
import site.warpnet.warpdroid.components.chats.ChatsActivity
import site.warpnet.warpdroid.components.viewthread.ViewThreadActivity
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.TimelineUser
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.entity.notificationTypeFromString
import site.warpnet.warpdroid.ui.WarpdroidPullToRefreshBox
import site.warpnet.warpdroid.ui.WarpdroidTheme
import site.warpnet.warpdroid.ui.preferences.LocalPreferences
import site.warpnet.warpdroid.ui.tweetcomponents.Avatar
import site.warpnet.warpdroid.ui.tweetcomponents.Timestamp
import site.warpnet.warpdroid.ui.warpdroidColors
import site.warpnet.warpdroid.util.startActivityWithSlideInAnimation

@AndroidEntryPoint
class NotificationsActivity : BaseActivity() {

    private val viewModel: NotificationsViewModel by viewModels()

    private enum class Tab { ALL, MENTIONS, FOLLOW_REQUESTS }

    private var resumedOnce = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            WarpdroidTheme {
                NotificationsContent()
            }
        }
    }

    // Coming back from a thread, a profile or another app picks up what
    // arrived meanwhile; the first resume is covered by the initial load.
    override fun onResume() {
        super.onResume()
        if (resumedOnce) viewModel.refresh(showSpinner = false)
        resumedOnce = true
    }

    @OptIn(ExperimentalMaterial3Api::class)
    @Composable
    private fun NotificationsContent() {
        val state by viewModel.state.collectAsStateWithLifecycle()
        var rawTab by remember { mutableStateOf(Tab.ALL) }
        var typeFilter by rememberSaveable { mutableStateOf("") }
        var typeMenuOpen by remember { mutableStateOf(false) }
        // Hide the FOLLOW_REQUESTS tab when the account isn't locked so the
        // view never renders a tab that has no data path behind it.
        val selectedTab = if (!state.locked && rawTab == Tab.FOLLOW_REQUESTS) Tab.ALL else rawTab

        Scaffold(
            contentWindowInsets = WindowInsets(0, 0, 0, 0),
            topBar = {
                TopAppBar(
                    title = {
                        Box {
                            Row(
                                modifier = Modifier.clickable { typeMenuOpen = true },
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Text(stringResource(TYPE_FILTERS.first { it.first == typeFilter }.second))
                                Icon(
                                    painterResource(R.drawable.ic_arrow_drop_down_24dp),
                                    stringResource(R.string.notification_type_filter),
                                )
                            }
                            DropdownMenu(expanded = typeMenuOpen, onDismissRequest = { typeMenuOpen = false }) {
                                TYPE_FILTERS.forEach { (type, label) ->
                                    DropdownMenuItem(
                                        text = { Text(stringResource(label)) },
                                        onClick = {
                                            typeFilter = type
                                            rawTab = Tab.ALL
                                            typeMenuOpen = false
                                        },
                                        leadingIcon = {
                                            Icon(
                                                painterResource(notificationTypeIcon(notificationTypeFromString(type))),
                                                contentDescription = null,
                                            )
                                        },
                                        trailingIcon = if (type == typeFilter) {
                                            { Icon(painterResource(R.drawable.ic_check_24dp), contentDescription = null) }
                                        } else {
                                            null
                                        },
                                    )
                                }
                            }
                        }
                    },
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
                            Tab.ALL -> NotificationList(
                                state.notifications.filter { typeFilter.isEmpty() || it.type.name == typeFilter },
                                typeFilter,
                                state,
                            )
                            Tab.MENTIONS -> NotificationList(
                                // Warpnet emits "reply" (never "mention") when someone responds
                                // to you, so the Mentions tab keys off replies too.
                                state.notifications.filter {
                                    it.type is Notification.Type.Mention || it.type is Notification.Type.Reply
                                },
                                "",
                                state,
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
    private fun NotificationList(items: List<Notification>, typeFilter: String, state: NotificationsViewModel.State) {
        val groups = remember(items, typeFilter) { items.distinctBy { it.id }.grouped(typeFilter) }
        val listState = rememberLazyListState()
        val nearEnd by remember {
            derivedStateOf {
                val info = listState.layoutInfo
                (info.visibleItemsInfo.lastOrNull()?.index ?: -1) >= info.totalItemsCount - 3
            }
        }
        // Keyed on everything loaded, not on what the filter shows: a page
        // with no matching items must still lead to the next one.
        LaunchedEffect(nearEnd, state.notifications.size) {
            if (nearEnd) viewModel.loadMore()
        }
        WarpdroidPullToRefreshBox(
            isRefreshing = state.refreshing,
            onRefresh = { viewModel.refresh(showSpinner = true) },
        ) {
            LazyColumn(state = listState, modifier = Modifier.fillMaxSize()) {
                if (groups.isEmpty() && !state.loadingMore) {
                    item {
                        Box(modifier = Modifier.fillParentMaxSize(), contentAlignment = Alignment.Center) {
                            Text(text = stringResource(R.string.notifications_empty))
                        }
                    }
                }
                items(groups, key = { it.key }) { group ->
                    NotificationRow(group)
                    HorizontalDivider()
                }
                if (state.loadingMore) {
                    item {
                        Box(modifier = Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) {
                            CircularProgressIndicator()
                        }
                    }
                }
            }
        }
    }

    @Composable
    private fun NotificationRow(group: NotificationGroup) {
        val n = group.head
        // A moderation notice has no actor: the type icon stands in for the avatar.
        val hasActor = n.type !is Notification.Type.ModerationWarning
        val single = hasActor && group.actors.size == 1
        val tweet = n.status
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clickable { onGroupClick(group) }
                .padding(horizontal = 16.dp, vertical = 12.dp),
        ) {
            Box(modifier = Modifier.size(48.dp)) {
                if (single) {
                    Avatar(
                        url = n.account.avatar,
                        staticUrl = n.account.staticAvatar,
                        isBot = false,
                        retweetedAvatarUrl = null,
                        staticRetweetedAvatarUrl = null,
                        onOpenProfile = { openAccount(n.account.id) },
                    )
                }
                Icon(
                    painter = painterResource(notificationTypeIcon(n.type)),
                    contentDescription = null,
                    tint = colorScheme.primary,
                    modifier = if (single) {
                        Modifier
                            .size(20.dp)
                            .offset(x = (-6).dp, y = (-6).dp)
                            .background(colorScheme.surface, CircleShape)
                            .padding(3.dp)
                    } else {
                        Modifier.fillMaxSize().padding(8.dp)
                    },
                )
            }
            Spacer(modifier = Modifier.width(12.dp))
            Column(modifier = Modifier.weight(1f)) {
                if (hasActor && !single) {
                    Row(
                        modifier = Modifier
                            .horizontalScroll(rememberScrollState())
                            .padding(bottom = 6.dp),
                        horizontalArrangement = Arrangement.spacedBy(6.dp),
                    ) {
                        group.actors.forEach { actor ->
                            Avatar(
                                url = actor.avatar,
                                staticUrl = actor.staticAvatar,
                                isBot = false,
                                retweetedAvatarUrl = null,
                                staticRetweetedAvatarUrl = null,
                                onOpenProfile = { openAccount(actor.id) },
                                modifier = Modifier.size(32.dp),
                            )
                        }
                    }
                }
                Row(verticalAlignment = Alignment.Top) {
                    Text(
                        text = notificationLabel(group, hasActor),
                        style = LocalPreferences.current.statusTextStyles.medium,
                        color = warpdroidColors.primaryTextColor,
                        modifier = Modifier.weight(1f),
                    )
                    n.createdAt?.let {
                        Spacer(modifier = Modifier.width(8.dp))
                        Timestamp(date = it, isEdited = false, textColor = warpdroidColors.tertiaryTextColor)
                    }
                }
                if (single && n.type is Notification.Type.Follow && n.account.note.isNotBlank()) {
                    Text(
                        text = n.account.note,
                        color = warpdroidColors.tertiaryTextColor,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.padding(top = 4.dp),
                    )
                }
                if (tweet != null) {
                    Text(
                        text = tweet.content.ifBlank { stringResource(R.string.action_open_post) },
                        color = warpdroidColors.tertiaryTextColor,
                        maxLines = 4,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier
                            .padding(top = 8.dp)
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(8.dp))
                            .border(1.dp, colorScheme.outlineVariant, RoundedCornerShape(8.dp))
                            .clickable { openThread(tweet) }
                            .padding(12.dp),
                    )
                }
            }
        }
    }

    // The node writes "<username> <action>"; the username becomes a link to
    // the actor, the action stays as the node phrased it.
    private fun notificationLabel(group: NotificationGroup, hasActor: Boolean): AnnotatedString {
        val n = group.head
        val name = n.account.name
        if (!hasActor || name == n.text) return AnnotatedString(n.text)
        val others = group.actors.size - 1
        return buildAnnotatedString {
            withLink(
                LinkAnnotation.Clickable(
                    tag = "actor",
                    styles = TextLinkStyles(SpanStyle(fontWeight = FontWeight.Bold)),
                ) { openAccount(n.account.id) },
            ) { append(name) }
            if (others > 0) append(resources.getQuantityString(R.plurals.notification_and_others, others, others))
            append(if (n.text.startsWith("$name ")) n.text.removePrefix(name) else " · ${n.text}")
        }
    }

    @DrawableRes
    private fun notificationTypeIcon(type: Notification.Type): Int = when (type) {
        Notification.Type.Mention -> R.drawable.ic_email_alternate_24dp
        Notification.Type.Reply -> R.drawable.ic_reply_24dp
        Notification.Type.Message -> R.drawable.ic_mail_24dp
        Notification.Type.Retweet -> R.drawable.ic_repeat_24dp
        Notification.Type.Reaction,
        Notification.Type.PleromaEmojiReaction -> R.drawable.ic_star_24dp_filled
        Notification.Type.Follow,
        Notification.Type.FollowRequest,
        Notification.Type.SignUp -> R.drawable.ic_person_add_24dp_mirrored
        Notification.Type.Status -> R.drawable.ic_notifications_active_24dp
        Notification.Type.Update,
        Notification.Type.QuotedUpdate -> R.drawable.ic_edit_24dp_filled
        Notification.Type.Quote -> R.drawable.ic_format_quote_24dp_filled
        Notification.Type.ModerationWarning -> R.drawable.ic_gavel_24dp
        Notification.Type.SeveredRelationship -> R.drawable.ic_block_24dp
        is Notification.Type.Unknown -> R.drawable.ic_notifications_24dp
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

    // A tweet opens its thread, a message opens the chat with its sender, a
    // group of follows opens the follower list, a single actor their profile.
    private fun onGroupClick(group: NotificationGroup) {
        val n = group.head
        val tweet = n.status
        when {
            tweet != null -> openThread(tweet)
            n.type is Notification.Type.Message -> lifecycleScope.launch {
                val chatId = viewModel.openChat(n.account.id)
                startActivityWithSlideInAnimation(
                    if (chatId == null) {
                        ChatsActivity.newIntent(this@NotificationsActivity)
                    } else {
                        ChatMessagesActivity.newIntent(this@NotificationsActivity, chatId, n.account.id, n.account.name)
                    },
                )
            }
            n.type is Notification.Type.ModerationWarning -> Unit
            group.actors.size > 1 -> startActivityWithSlideInAnimation(
                AccountListActivity.newIntent(this, AccountListActivity.Type.FOLLOWERS, viewModel.ownerId),
            )
            else -> openAccount(n.account.id)
        }
    }

    private fun openThread(tweet: Tweet) {
        startActivityWithSlideInAnimation(
            ViewThreadActivity.newIntent(this, tweet.id, tweet.url ?: "", tweet.account.id),
        )
    }

    private fun openAccount(accountId: String) {
        startActivityWithSlideInAnimation(AccountActivity.newIntent(this, accountId))
    }

    companion object {
        fun newIntent(context: Context): Intent =
            Intent(context, NotificationsActivity::class.java)
    }
}

// Follows group together, reactions and retweets group per tweet; the rest
// stay one row each. A list filtered to follows keeps them apart.
private class NotificationGroup(val key: String, val items: List<Notification>) {
    val head: Notification get() = items.first()
    val actors: List<TimelineUser> = items.map { it.account }.distinctBy { it.id }
}

private fun List<Notification>.grouped(typeFilter: String): List<NotificationGroup> =
    groupBy { n ->
        when {
            n.type is Notification.Type.Follow && typeFilter != "follow" -> "follow"
            (n.type is Notification.Type.Reaction || n.type is Notification.Type.Retweet) && n.status != null ->
                "${n.type.name}-${n.status.id}"
            else -> n.id
        }
    }.map { (key, items) -> NotificationGroup(key, items) }

// Type names as notificationTypeFromString resolves the node's types; the
// empty one shows everything.
private val TYPE_FILTERS = listOf(
    "" to R.string.title_notifications,
    "reaction" to R.string.notification_reaction_name,
    "retweet" to R.string.notification_retweet_name,
    "follow" to R.string.notification_follow_name,
    "reply" to R.string.notification_reply_name,
    "message" to R.string.notification_message_name,
    "moderation_warning" to R.string.notification_moderation_warning_name,
    "order_limit" to R.string.notification_order_limit_name,
)
