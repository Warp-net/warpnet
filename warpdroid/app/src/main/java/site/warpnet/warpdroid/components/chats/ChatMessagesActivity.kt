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
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.wrapContentSize
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dagger.hilt.android.AndroidEntryPoint
import site.warpnet.transport.dto.WarpnetMessage
import site.warpnet.warpdroid.BaseActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.ui.WarpdroidTheme

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
        val listState = rememberLazyListState()

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
                                MessageBubble(msg, isOwn = msg.senderId == state.ownUserId)
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
    private fun MessageBubble(msg: WarpnetMessage, isOwn: Boolean) {
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
                    .padding(horizontal = 12.dp, vertical = 8.dp),
            ) {
                Text(
                    text = msg.text,
                    color = if (isOwn) colorScheme.onPrimary else colorScheme.onSurface,
                )
            }
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
