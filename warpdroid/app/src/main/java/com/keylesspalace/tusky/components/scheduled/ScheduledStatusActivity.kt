/* Copyright 2019 Tusky Contributors
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

package com.keylesspalace.tusky.components.scheduled

import android.content.Context
import android.content.Intent
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.annotation.DrawableRes
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsBottomHeight
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush.Companion.verticalGradient
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.dimensionResource
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.Hyphens
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.paging.LoadState
import androidx.paging.compose.collectAsLazyPagingItems
import androidx.paging.compose.itemKey
import com.keylesspalace.tusky.BaseActivity
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.ViewMediaActivity
import com.keylesspalace.tusky.components.compose.ComposeActivity
import com.keylesspalace.tusky.components.compose.view.ComposeScheduleView.Companion.formatDate
import com.keylesspalace.tusky.entity.Attachment
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.ui.MessageViewMode
import com.keylesspalace.tusky.ui.TuskyButton
import com.keylesspalace.tusky.ui.TuskyButtonSize
import com.keylesspalace.tusky.ui.TuskyMessageView
import com.keylesspalace.tusky.ui.TuskyOutlinedButton
import com.keylesspalace.tusky.ui.TuskyPreviewTheme
import com.keylesspalace.tusky.ui.TuskyPullToRefreshBox
import com.keylesspalace.tusky.ui.TuskyTextButton
import com.keylesspalace.tusky.ui.TuskyTheme
import com.keylesspalace.tusky.ui.preferences.LocalAccount
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.statuscomponents.MediaAttachments
import com.keylesspalace.tusky.ui.statuscomponents.PollPreview
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.ui.tuskyDefaultCornerShape
import com.keylesspalace.tusky.viewdata.AttachmentViewData
import dagger.hilt.android.AndroidEntryPoint
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import kotlinx.coroutines.launch
import retrofit2.HttpException

@AndroidEntryPoint
class ScheduledStatusActivity : BaseActivity() {

    private val viewModel: ScheduledStatusViewModel by viewModels()

    private val dateTimeFormat = SimpleDateFormat("d MMM yyyy, H:mm", Locale.getDefault())

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        setContent {
            TuskyTheme {
                FiltersActivityContent()
            }
        }
    }

    @OptIn(ExperimentalMaterial3Api::class)
    @Composable
    private fun FiltersActivityContent() {
        var showDeleteConfirmation by remember { mutableStateOf<ScheduledStatusViewData?>(null) }

        val snackbarHostState = remember { SnackbarHostState() }

        val listState = rememberLazyListState()
        val coroutineScope = rememberCoroutineScope()

        Scaffold(
            contentWindowInsets = WindowInsets(0, 0, 0, 0),
            topBar = {
                TopAppBar(
                    title = {
                        Text(
                            text = stringResource(R.string.title_scheduled_posts),
                            modifier = Modifier.clickable(
                                interactionSource = remember { MutableInteractionSource() },
                                indication = null
                            ) {
                                coroutineScope.launch {
                                    listState.scrollToItem(0)
                                }
                            }
                        )
                    },
                    navigationIcon = {
                        IconButton(
                            onClick = ::finish
                        ) {
                            Icon(
                                painterResource(R.drawable.ic_arrow_back_24dp),
                                stringResource(R.string.button_back)
                            )
                        }
                    }
                )
            },
            snackbarHost = {
                SnackbarHost(snackbarHostState)
            },
            containerColor = tuskyColors.windowBackground
        ) { contentPadding ->
            Box(
                modifier = Modifier
                    .padding(contentPadding)
                    .fillMaxSize()
            ) {
                val pagingItems = viewModel.data.collectAsLazyPagingItems()

                if (pagingItems.itemCount == 0) {
                    Box(
                        modifier = Modifier
                            .fillMaxHeight()
                            .widthIn(min = 640.dp)
                            .align(Alignment.Center)
                            .background(colorScheme.background)
                    ) {
                        when (pagingItems.loadState.refresh) {
                            is LoadState.Loading -> {
                                CircularProgressIndicator(modifier = Modifier.align(Alignment.Center))
                            }

                            is LoadState.Error -> {
                                val error = (pagingItems.loadState.refresh as LoadState.Error).error
                                if (error is HttpException && error.code() == 404) {
                                    val currentAccount = LocalAccount.current
                                    TuskyMessageView(
                                        message = stringResource(
                                            R.string.scheduled_posts_not_supported,
                                            currentAccount?.domain.orEmpty()
                                        ),
                                        mode = MessageViewMode.ERROR_OTHER,
                                        modifier = Modifier.align(Alignment.Center)
                                    )
                                } else {
                                    TuskyMessageView(
                                        onRetry = pagingItems::retry,
                                        error = (pagingItems.loadState.refresh as LoadState.Error).error,
                                        modifier = Modifier.align(Alignment.Center)
                                    )
                                }
                            }

                            else -> {
                                TuskyMessageView(
                                    onRetry = null,
                                    message = stringResource(R.string.no_scheduled_posts),
                                    mode = MessageViewMode.EMPTY,
                                    modifier = Modifier.align(Alignment.Center)
                                )
                            }
                        }
                    }
                } else {
                    TuskyPullToRefreshBox(
                        isRefreshing = pagingItems.loadState.refresh is LoadState.Loading,
                        onRefresh = pagingItems::refresh,
                        modifier = Modifier.fillMaxSize(),
                    ) {
                        Box(
                            modifier = Modifier
                                .widthIn(max = 640.dp)
                                .fillMaxWidth()
                                .fillMaxHeight()
                                .align(Alignment.Center)
                                .background(colorScheme.background)
                        )
                        LazyColumn(
                            state = listState,
                            modifier = Modifier.fillMaxSize(),
                            horizontalAlignment = Alignment.CenterHorizontally
                        ) {
                            items(
                                count = pagingItems.itemCount,
                                key = pagingItems.itemKey { it.id }
                            ) { index ->
                                pagingItems[index]?.let { status ->
                                    ScheduledStatus(
                                        status = status,
                                        modifier = Modifier
                                            .widthIn(max = 640.dp),
                                        onDelete = {
                                            showDeleteConfirmation = status
                                        }
                                    )
                                }
                            }

                            item(key = "bottomSpacer") {
                                Column {
                                    Spacer(
                                        modifier = Modifier.windowInsetsBottomHeight(WindowInsets.systemBars)
                                    )
                                    Spacer(
                                        modifier = Modifier.height(dimensionResource(R.dimen.recyclerview_bottom_padding_no_actionbutton))
                                    )
                                }
                            }
                        }
                    }
                }
            }

            showDeleteConfirmation?.let { statusToDelete ->
                AlertDialog(
                    text = { Text(stringResource(R.string.delete_scheduled_post_warning)) },
                    onDismissRequest = {
                        showDeleteConfirmation = null
                    },
                    confirmButton = {
                        TuskyTextButton(
                            text = stringResource(R.string.dialog_delete_filter_positive_action),
                            onClick = {
                                viewModel.deleteScheduledStatus(statusToDelete)
                                showDeleteConfirmation = null
                            }
                        )
                    },
                    dismissButton = {
                        TuskyTextButton(
                            text = stringResource(android.R.string.cancel),
                            onClick = {
                                showDeleteConfirmation = null
                            }
                        )
                    },
                    shape = RoundedCornerShape(16.dp),
                    containerColor = colorScheme.background,
                    tonalElevation = 6.dp
                )
            }
        }

        val error by viewModel.error.collectAsStateWithLifecycle(initialValue = null)

        val snackBarMessage = error?.let {
            stringResource(R.string.scheduled_post_deletion_error)
        }

        LaunchedEffect(error) {
            if (snackBarMessage != null) {
                snackbarHostState.showSnackbar(snackBarMessage)
            }
        }
    }

    @Composable
    private fun ScheduledStatus(
        status: ScheduledStatusViewData,
        modifier: Modifier,
        onDelete: () -> Unit
    ) {
        Column(
            modifier = modifier
        ) {
            Spacer(modifier = Modifier.height(4.dp))

            if (status.inReplyToId != null) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier
                        .padding(top = 8.dp, start = 16.dp, end = 16.dp)
                        .semantics(mergeDescendants = true) { },
                ) {
                    Icon(
                        painter = painterResource(R.drawable.ic_reply_18dp),
                        tint = tuskyColors.tertiaryTextColor,
                        contentDescription = null
                    )
                    Spacer(modifier = Modifier.width(4.dp))
                    Text(
                        text = stringResource(R.string.scheduled_post_reply),
                        color = tuskyColors.tertiaryTextColor,
                        fontSize = 16.sp,
                    )
                }
            }

            if (!status.spoilerText.isNullOrBlank()) {
                Text(
                    text = status.spoilerText,
                    color = tuskyColors.primaryTextColor,
                    fontSize = 16.sp,
                    modifier = Modifier.padding(top = 8.dp, start = 16.dp, end = 16.dp)
                )
                TuskyOutlinedButton(
                    text = if (status.spoilerExpanded) {
                        stringResource(R.string.post_content_warning_show_less)
                    } else {
                        stringResource(R.string.post_content_warning_show_more)
                    },
                    onClick = { viewModel.expandSpoiler(!status.spoilerExpanded, status.id) },
                    size = TuskyButtonSize.Small,
                    modifier = Modifier
                        .padding(top = 8.dp, start = 16.dp, end = 16.dp)
                        .widthIn(min = 150.dp)
                )
            }

            AnimatedVisibility(
                visible = status.text.isNotBlank() && status.spoilerExpanded
            ) {
                Box {
                    Text(
                        text = status.text,
                        color = tuskyColors.primaryTextColor,
                        fontSize = 16.sp,
                        overflow = TextOverflow.Clip,
                        maxLines = if (!status.hasLongText || status.overflowVisible) Int.MAX_VALUE else 10,
                        style = LocalTextStyle.current.copy(hyphens = Hyphens.Auto),
                        modifier = Modifier.padding(top = 8.dp, start = 16.dp, end = 16.dp)
                    )
                    if (status.hasLongText && !status.overflowVisible) {
                        // Fading Edge overlay
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(80.dp)
                                .align(Alignment.BottomCenter)
                                .background(
                                    verticalGradient(
                                        colors = listOf(Color.Transparent, colorScheme.background)
                                    )
                                )
                        )
                    }
                }
            }

            if (status.hasLongText && status.spoilerExpanded) {
                TuskyTextButton(
                    text = if (status.overflowVisible) {
                        stringResource(R.string.post_content_show_less)
                    } else {
                        stringResource(R.string.post_content_show_more)
                    },
                    onClick = { viewModel.showOverflow(!status.overflowVisible, status.id) },
                    modifier = Modifier.align(Alignment.CenterHorizontally)
                )
            }

            MediaAttachments(
                attachments = status.attachments,
                onOpenAttachment = { index ->
                    val attachmentViewData = status.attachments.map { attachment ->
                        AttachmentViewData(
                            attachment = attachment,
                            statusId = null,
                            statusUrl = null,
                            sensitive = status.sensitive,
                            isRevealed = true
                        )
                    }
                    val intent = ViewMediaActivity.newIntent(this@ScheduledStatusActivity, attachmentViewData, index)
                    startActivity(intent)
                },
                onMediaHiddenChanged = {
                    viewModel.showMedia(!status.mediaVisible, status.id)
                },
                sensitive = status.sensitive,
                showMedia = status.mediaVisible,
                isStatusExpanded = status.spoilerExpanded,
                downloadPreviews = LocalAccount.current?.mediaPreviewEnabled ?: true,
                showBlurhash = LocalPreferences.current.useBlurhash,
                filter = null,
                modifier = Modifier.padding(top = 12.dp, start = 16.dp, end = 16.dp)
            )
            status.poll?.let { poll ->
                PollPreview(
                    poll = poll,
                    modifier = Modifier.padding(top = 12.dp, start = 16.dp, end = 16.dp)
                )
            }
            FlowRow(
                modifier = Modifier.padding(top = 12.dp, start = 16.dp, end = 16.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Badge(status.scheduledAt.format(), R.drawable.ic_schedule_24dp)
                Badge(stringResource(status.visibility.text()), status.visibility.icon())

                Badge(status.language.uppercase())
            }
            Row(
                modifier = Modifier.padding(top = 12.dp, start = 16.dp, end = 16.dp, bottom = 16.dp),
                horizontalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                TuskyOutlinedButton(
                    text = stringResource(R.string.action_delete),
                    modifier = Modifier.weight(1f),
                    onClick = onDelete
                )
                TuskyButton(
                    text = stringResource(R.string.action_edit),
                    onClick = { edit(status) },
                    modifier = Modifier.weight(1f)
                )
            }
            HorizontalDivider()
        }
    }

    @Composable
    private fun Badge(
        text: String,
        @DrawableRes drawable: Int? = null,
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier
                .border(
                    BorderStroke(1.dp, colorScheme.primary),
                    tuskyDefaultCornerShape
                )
                .padding(start = 8.dp, end = 8.dp, top = 4.dp, bottom = 4.dp)
                .semantics(mergeDescendants = true) { }
        ) {
            drawable?.let { drawableRes ->
                Icon(
                    modifier = Modifier.size(16.dp),
                    painter = painterResource(drawableRes),
                    contentDescription = null,
                    tint = colorScheme.primary
                )
                Spacer(Modifier.width(6.dp))
            }
            Text(
                text,
                fontSize = 16.sp,
                color = colorScheme.primary
            )
        }
    }

    private fun Status.Visibility.icon() = when (this) {
        Status.Visibility.PUBLIC -> R.drawable.ic_public_24dp
        Status.Visibility.PRIVATE -> R.drawable.ic_lock_24dp
        Status.Visibility.DIRECT -> R.drawable.ic_mail_24dp
        Status.Visibility.UNLISTED -> R.drawable.ic_lock_open_24dp
        else -> R.drawable.ic_lock_open_24dp
    }

    private fun Status.Visibility.text() = when (this) {
        Status.Visibility.PUBLIC -> R.string.post_privacy_public
        Status.Visibility.PRIVATE -> R.string.post_privacy_followers_only
        Status.Visibility.DIRECT -> R.string.post_privacy_direct
        Status.Visibility.UNLISTED -> R.string.post_privacy_unlisted
        else -> R.string.post_privacy_unlisted
    }

    private fun Date.format(): String {
        return dateTimeFormat.format(this)
    }

    private fun edit(item: ScheduledStatusViewData) {
        val intent = ComposeActivity.newIntent(
            this,
            ComposeActivity.ComposeOptions(
                scheduledTootId = item.id,
                content = item.text,
                contentWarning = item.spoilerText,
                mediaAttachments = item.attachments,
                inReplyToId = item.inReplyToId,
                visibility = item.visibility,
                language = item.language,
                poll = item.poll,
                scheduledAt = formatDate(item.scheduledAt),
                sensitive = item.sensitive,
                kind = ComposeActivity.ComposeKind.EDIT_SCHEDULED
            )
        )
        startActivity(intent)
    }

    companion object {
        fun newIntent(context: Context) = Intent(context, ScheduledStatusActivity::class.java)
    }

    @PreviewLightDark
    @Composable
    fun PreviewScheduledStatus() {
        TuskyPreviewTheme {
            ScheduledStatus(
                status = ScheduledStatusViewData(
                    id = "0",
                    scheduledAt = Date(1756467793000),
                    text = "This is a scheduled status",
                    spoilerText = "Some spoiler Text",
                    visibility = Status.Visibility.PUBLIC,
                    sensitive = false,
                    inReplyToId = "X",
                    language = "EN",
                    attachments = listOf(
                        Attachment(
                            id = "1",
                            url = "https://example.com/1",
                            previewUrl = "https://example.com/preview/1",
                            type = Attachment.Type.IMAGE,
                            description = "description 1"
                        )
                    ),
                    poll = null,
                    spoilerExpanded = true,
                    overflowVisible = true,
                    mediaVisible = true,
                ),
                Modifier.background(colorScheme.background),
                onDelete = {}
            )
        }
    }
}
