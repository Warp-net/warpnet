/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * History + composer for a single Warpnet 1:1 chat. Mirrors Vue's Messages
 * view: oldest message at the top, latest at the bottom, single-line input
 * with a send button.
 */
package site.warpnet.warpdroid.components.chats

import android.content.Context
import android.content.Intent
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.wrapContentSize
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.draw.clip
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dagger.hilt.android.AndroidEntryPoint
import site.warpnet.transport.dto.WarpnetMessage
import site.warpnet.warpdroid.BaseActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.ViewMediaActivity
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.ui.WarpdroidAsyncImage
import site.warpnet.warpdroid.ui.WarpdroidTheme
import site.warpnet.warpdroid.viewdata.AttachmentViewData
import site.warpnet.warpdroid.warpnet.WarpnetMapper

@AndroidEntryPoint
class ChatMessagesActivity : BaseActivity() {

    private val viewModel: ChatMessagesViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            WarpdroidTheme {
                ChatMessagesContent()
            }
        }
    }

    @OptIn(ExperimentalMaterial3Api::class)
    @Composable
    private fun ChatMessagesContent() {
        val state by viewModel.state.collectAsStateWithLifecycle()
        val otherName = intent.getStringExtra(EXTRA_OTHER_NAME).orEmpty()
        var draft by remember { mutableStateOf("") }
        var messageToDelete by remember { mutableStateOf<WarpnetMessage?>(null) }
        val listState = rememberLazyListState()

        messageToDelete?.let { pending ->
            AlertDialog(
                onDismissRequest = { messageToDelete = null },
                text = { Text(stringResource(R.string.dialog_delete_message_warning)) },
                confirmButton = {
                    TextButton(
                        onClick = {
                            viewModel.deleteMessage(pending)
                            messageToDelete = null
                        },
                    ) {
                        Text(stringResource(R.string.action_delete))
                    }
                },
                dismissButton = {
                    TextButton(onClick = { messageToDelete = null }) {
                        Text(stringResource(android.R.string.cancel))
                    }
                },
            )
        }

        LifecycleResumeEffect(Unit) {
            viewModel.startPolling()
            onPauseOrDispose { viewModel.stopPolling() }
        }

        // Whenever a new message lands, scroll to it so the user sees their
        // own send or the incoming reply without scrolling manually.
        LaunchedEffect(state.messages.size) {
            if (state.messages.isNotEmpty()) {
                listState.animateScrollToItem(state.messages.lastIndex)
            }
        }

        Scaffold(
            contentWindowInsets = WindowInsets(0, 0, 0, 0),
            topBar = {
                TopAppBar(
                    title = { Text(otherName.ifEmpty { stringResource(R.string.title_direct_messages) }) },
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
                    .fillMaxSize()
                    .imePadding(),
            ) {
                Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
                    if (state.loading && state.messages.isEmpty()) {
                        CircularProgressIndicator(modifier = Modifier.align(Alignment.Center))
                    } else if (state.messages.isEmpty()) {
                        Text(
                            text = stringResource(R.string.chat_messages_empty),
                            modifier = Modifier.align(Alignment.Center),
                        )
                    } else {
                        LazyColumn(
                            state = listState,
                            modifier = Modifier.fillMaxSize(),
                            verticalArrangement = Arrangement.spacedBy(6.dp),
                        ) {
                            items(state.messages, key = { messageDisplayKey(it) }) { msg ->
                                val isOwn = msg.senderId == state.ownUserId
                                MessageBubble(
                                    msg = msg,
                                    isOwn = isOwn,
                                    // Only own, id-carrying messages can be deleted —
                                    // PRIVATE_DELETE_MESSAGE addresses by message id.
                                    onLongPress = if (isOwn && msg.id.isNotEmpty()) {
                                        { messageToDelete = msg }
                                    } else {
                                        null
                                    },
                                )
                            }
                        }
                    }
                }
                Composer(
                    text = draft,
                    sending = state.sending,
                    onChange = { draft = it },
                    onSend = {
                        if (draft.isNotBlank()) {
                            viewModel.send(draft)
                            draft = ""
                        }
                    },
                )
            }
        }
    }

    @Composable
    private fun MessageBubble(msg: WarpnetMessage, isOwn: Boolean, onLongPress: (() -> Unit)? = null) {
        val longPressModifier = if (onLongPress != null) {
            Modifier.pointerInput(msg.id) {
                detectTapGestures(onLongPress = { onLongPress() })
            }
        } else {
            Modifier
        }
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp),
            horizontalArrangement = if (isOwn) Arrangement.End else Arrangement.Start,
        ) {
            Box(
                modifier = Modifier
                    .widthIn(max = 280.dp)
                    .background(
                        color = if (isOwn) colorScheme.primary else colorScheme.surfaceVariant,
                        shape = RoundedCornerShape(12.dp),
                    )
                    .then(longPressModifier)
                    .padding(horizontal = 12.dp, vertical = 8.dp),
            ) {
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    if (msg.text.isNotEmpty()) {
                        Text(
                            text = msg.text,
                            color = if (isOwn) colorScheme.onPrimary else colorScheme.onSurface,
                        )
                    }
                    Attachments(msg)
                }
            }
        }
    }

    /**
     * Chat attachments resolve through the chat media routes, so the thumbnail
     * URL carries the chat-image scheme rather than the public one. A video
     * message carries its still frame as its only image key, so the frame is
     * what the bubble shows; tapping hands the clip to the media viewer.
     */
    @Composable
    private fun Attachments(msg: WarpnetMessage) {
        val views = remember(msg.id, msg.imageKeys, msg.videoKey) { msg.toAttachmentViews() }
        if (views.isEmpty()) return

        val context = LocalContext.current
        views.forEachIndexed { index, view ->
            val thumb = view.attachment.previewUrl.orEmpty().ifEmpty { view.attachment.url }
            WarpdroidAsyncImage(
                model = thumb,
                contentDescription = stringResource(R.string.action_open_media_n, index + 1),
                contentScale = ContentScale.Crop,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 220.dp)
                    .clip(RoundedCornerShape(8.dp))
                    .clickable {
                        context.startActivity(ViewMediaActivity.newIntent(context, views, index))
                    },
            )
        }
    }

    private fun WarpnetMessage.toAttachmentViews(): List<AttachmentViewData> {
        val imageKeys = imageKeys.orEmpty().filter { it.isNotBlank() }
        val clip = videoKey?.takeIf { it.isNotBlank() }
        val attachments = if (clip != null) {
            listOf(
                Attachment(
                    id = clip,
                    url = WarpnetMapper.warpnetChatVideoUrl(senderId, clip),
                    previewUrl = WarpnetMapper.warpnetChatImageUrl(senderId, imageKeys.firstOrNull()),
                    type = Attachment.Type.VIDEO,
                ),
            )
        } else {
            imageKeys.map { key ->
                Attachment(
                    id = key,
                    url = WarpnetMapper.warpnetChatImageUrl(senderId, key),
                    previewUrl = WarpnetMapper.warpnetChatImageUrl(senderId, key),
                    type = Attachment.Type.IMAGE,
                )
            }
        }
        return attachments.map {
            AttachmentViewData(
                attachment = it,
                statusId = null,
                statusUrl = null,
                statusAuthorId = null,
                sensitive = false,
                isRevealed = true,
            )
        }
    }

    @Composable
    private fun Composer(
        text: String,
        sending: Boolean,
        onChange: (String) -> Unit,
        onSend: () -> Unit,
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            OutlinedTextField(
                value = text,
                onValueChange = onChange,
                modifier = Modifier.weight(1f),
                placeholder = { Text(stringResource(R.string.chat_compose_placeholder)) },
                singleLine = false,
                maxLines = 4,
            )
            Button(
                onClick = onSend,
                enabled = !sending && text.isNotBlank(),
                modifier = Modifier
                    .padding(start = 8.dp)
                    .wrapContentSize(),
            ) {
                Text(stringResource(R.string.action_chat_send))
            }
        }
    }

    companion object {
        private const val EXTRA_CHAT_ID = "chat_id"
        private const val EXTRA_OTHER_USER_ID = "other_user_id"
        private const val EXTRA_OTHER_NAME = "other_user_name"

        fun newIntent(
            context: Context,
            chatId: String,
            otherUserId: String,
            otherUserName: String,
        ): Intent =
            Intent(context, ChatMessagesActivity::class.java).apply {
                putExtra(EXTRA_CHAT_ID, chatId)
                putExtra(EXTRA_OTHER_USER_ID, otherUserId)
                putExtra(EXTRA_OTHER_NAME, otherUserName)
            }
    }
}
