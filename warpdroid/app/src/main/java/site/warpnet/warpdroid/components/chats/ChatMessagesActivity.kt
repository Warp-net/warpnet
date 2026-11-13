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
import android.widget.ImageView
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
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
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.wrapContentSize
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.bumptech.glide.Glide
import dagger.hilt.android.AndroidEntryPoint
import site.warpnet.transport.WarpnetLimits
import site.warpnet.transport.dto.WarpnetMessage
import site.warpnet.warpdroid.BaseActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.ViewMediaActivity
import site.warpnet.warpdroid.entity.Attachment
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
        val pickMedia = rememberLauncherForActivityResult(
            ActivityResultContracts.PickMultipleVisualMedia(WarpnetLimits.MAX_IMAGES_PER_TWEET),
        ) { uris ->
            if (!viewModel.attach(uris)) {
                Toast.makeText(this, R.string.error_media_upload_image_or_video, Toast.LENGTH_LONG).show()
            }
        }

        LaunchedEffect(state.sendFailed) {
            if (state.sendFailed) {
                Toast.makeText(this@ChatMessagesActivity, R.string.error_media_upload_sending, Toast.LENGTH_LONG).show()
                viewModel.consumeSendFailure()
            }
        }

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
                    title = {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(otherName.ifEmpty { stringResource(R.string.title_direct_messages) })
                            UserStatusDot(intent.getStringExtra(EXTRA_OTHER_USER_ID).orEmpty(), viewModel::isOnline)
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
                    .fillMaxSize()
                    .navigationBarsPadding()
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
                    attachments = state.attachments.size,
                    onChange = { draft = it },
                    onAttach = {
                        pickMedia.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageAndVideo))
                    },
                    onClearAttachments = viewModel::clearAttachments,
                    onSend = {
                        if (draft.isNotBlank() || state.attachments.isNotEmpty()) {
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
                    MessageMedia(msg)
                    if (msg.text.isNotEmpty()) {
                        Text(
                            text = msg.text,
                            color = if (isOwn) colorScheme.onPrimary else colorScheme.onSurface,
                        )
                    }
                }
            }
        }
    }

    // A video message carries its still frame as the only image key.
    @Composable
    private fun MessageMedia(msg: WarpnetMessage) {
        val imageKeys = msg.imageKeys.orEmpty().filter { it.isNotBlank() }
        val videoKey = msg.videoKey?.takeIf { it.isNotBlank() }
        val attachments = if (videoKey != null) {
            listOf(
                Attachment(
                    id = videoKey,
                    url = WarpnetMapper.warpnetChatVideoUrl(msg.senderId, videoKey),
                    previewUrl = imageKeys.firstOrNull()?.let { WarpnetMapper.warpnetChatImageUrl(msg.senderId, it) },
                    type = Attachment.Type.VIDEO,
                ),
            )
        } else {
            imageKeys.map { key ->
                val url = WarpnetMapper.warpnetChatImageUrl(msg.senderId, key)
                Attachment(id = key, url = url, previewUrl = url, type = Attachment.Type.IMAGE)
            }
        }
        attachments.forEachIndexed { index, attachment ->
            Box(
                modifier = Modifier
                    .size(240.dp, 180.dp)
                    .clip(RoundedCornerShape(8.dp))
                    .clickable { openMedia(attachments, index) },
                contentAlignment = Alignment.Center,
            ) {
                AndroidView(
                    factory = { ctx -> ImageView(ctx).apply { scaleType = ImageView.ScaleType.CENTER_CROP } },
                    update = { view ->
                        Glide.with(view)
                            .load(attachment.previewUrl)
                            .placeholder(R.drawable.card_image_placeholder)
                            .error(R.drawable.card_image_placeholder)
                            .into(view)
                    },
                    modifier = Modifier.fillMaxSize(),
                )
                if (attachment.type == Attachment.Type.VIDEO) {
                    Icon(
                        painterResource(R.drawable.ic_play_arrow_24dp),
                        stringResource(R.string.post_media_video),
                        tint = colorScheme.onPrimary,
                        modifier = Modifier
                            .size(48.dp)
                            .background(colorScheme.scrim.copy(alpha = 0.5f), RoundedCornerShape(24.dp)),
                    )
                }
            }
        }
    }

    private fun openMedia(attachments: List<Attachment>, index: Int) {
        val viewData = attachments.map {
            AttachmentViewData(
                attachment = it,
                statusId = null,
                statusUrl = null,
                statusAuthorId = null,
                sensitive = false,
                isRevealed = true,
            )
        }
        startActivity(ViewMediaActivity.newIntent(this, viewData, index))
    }

    @Composable
    private fun Composer(
        text: String,
        sending: Boolean,
        attachments: Int,
        onChange: (String) -> Unit,
        onAttach: () -> Unit,
        onClearAttachments: () -> Unit,
        onSend: () -> Unit,
    ) {
        if (attachments > 0) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = stringResource(R.string.title_media) + ": $attachments",
                    modifier = Modifier.weight(1f),
                )
                IconButton(onClick = onClearAttachments, enabled = !sending) {
                    Icon(painterResource(R.drawable.ic_close_24dp), stringResource(R.string.action_remove))
                }
            }
        }
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IconButton(onClick = onAttach, enabled = !sending) {
                Icon(painterResource(R.drawable.ic_attach_file_24dp), stringResource(R.string.action_add_media))
            }
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
                enabled = !sending && (text.isNotBlank() || attachments > 0),
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
