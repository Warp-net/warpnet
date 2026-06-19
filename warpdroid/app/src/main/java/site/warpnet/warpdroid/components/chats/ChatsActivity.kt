/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Direct-message list mirroring Vue's Chats view. Each row is a 1:1
 * conversation with a Warpnet user; tap to open the full message thread.
 */
package site.warpnet.warpdroid.components.chats

import android.content.Context
import android.content.Intent
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
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
import site.warpnet.warpdroid.entity.TimelineUser
import site.warpnet.warpdroid.ui.WarpdroidTheme
import site.warpnet.warpdroid.util.startActivityWithSlideInAnimation

@AndroidEntryPoint
class ChatsActivity : BaseActivity() {

    private val viewModel: ChatsViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            WarpdroidTheme {
                ChatsContent()
            }
        }
    }

    @OptIn(ExperimentalMaterial3Api::class)
    @Composable
    private fun ChatsContent() {
        val state by viewModel.state.collectAsStateWithLifecycle()
        val newChat by viewModel.newChat.collectAsStateWithLifecycle()
        Scaffold(
            contentWindowInsets = WindowInsets(0, 0, 0, 0),
            topBar = {
                TopAppBar(
                    title = { Text(stringResource(R.string.title_direct_messages)) },
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
            floatingActionButton = {
                FloatingActionButton(onClick = viewModel::showNewChat) {
                    Icon(
                        painterResource(R.drawable.ic_edit_24dp_filled),
                        stringResource(R.string.chats_new_message),
                    )
                }
            },
        ) { contentPadding ->
            Box(
                modifier = Modifier
                    .padding(contentPadding)
                    .fillMaxSize(),
            ) {
                when {
                    state.loading && state.chats.isEmpty() ->
                        CircularProgressIndicator(modifier = Modifier.align(Alignment.Center))
                    state.chats.isEmpty() ->
                        Text(
                            text = stringResource(R.string.chats_empty),
                            modifier = Modifier.align(Alignment.Center),
                        )
                    else -> LazyColumn(modifier = Modifier.fillMaxSize()) {
                        items(state.chats, key = { it.chat.id }) { row ->
                            ChatRow(row) { openChat(row) }
                            HorizontalDivider()
                        }
                    }
                }
            }
        }

        if (newChat.visible) {
            ModalBottomSheet(
                onDismissRequest = viewModel::hideNewChat,
                sheetState = rememberModalBottomSheetState(),
            ) {
                NewChatSheet(newChat)
            }
        }
    }

    @Composable
    private fun NewChatSheet(state: ChatsViewModel.NewChatState) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .imePadding()
                .padding(horizontal = 16.dp),
        ) {
            Text(
                text = stringResource(R.string.chats_new_message),
                fontWeight = FontWeight.SemiBold,
            )
            OutlinedTextField(
                value = state.query,
                onValueChange = viewModel::onNewChatQuery,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(vertical = 8.dp),
                placeholder = { Text(stringResource(R.string.chats_search_people)) },
                singleLine = true,
            )
            Box(modifier = Modifier.fillMaxWidth().heightIn(max = 360.dp)) {
                when {
                    state.searching ->
                        CircularProgressIndicator(
                            modifier = Modifier.align(Alignment.Center).padding(16.dp),
                        )
                    state.query.isNotBlank() && state.results.isEmpty() ->
                        Text(
                            text = stringResource(R.string.chats_no_results),
                            modifier = Modifier.align(Alignment.Center).padding(16.dp),
                        )
                    else -> LazyColumn(modifier = Modifier.fillMaxWidth()) {
                        items(state.results, key = { it.id }) { user ->
                            UserPickRow(user) { startNewChat(user) }
                            HorizontalDivider()
                        }
                    }
                }
            }
        }
    }

    @Composable
    private fun UserPickRow(user: TimelineUser, onClick: () -> Unit) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .clickable(onClick = onClick)
                .padding(vertical = 10.dp),
        ) {
            Text(text = user.name, fontWeight = FontWeight.SemiBold)
            if (user.username.isNotBlank()) {
                Text(text = "@" + user.username, color = Color.Gray, maxLines = 1)
            }
        }
    }

    private fun startNewChat(user: TimelineUser) {
        viewModel.startChat(user) { chatId, otherUserId, name ->
            startActivityWithSlideInAnimation(
                ChatMessagesActivity.newIntent(
                    this,
                    chatId = chatId,
                    otherUserId = otherUserId,
                    otherUserName = name,
                ),
            )
        }
    }

    @Composable
    private fun ChatRow(row: ChatsViewModel.ChatRow, onClick: () -> Unit) {
        val name = row.other?.name ?: row.otherUserId
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clickable(onClick = onClick)
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Column(modifier = Modifier.fillMaxWidth()) {
                Text(text = name, fontWeight = FontWeight.SemiBold)
                if (row.chat.lastMessage.isNotBlank()) {
                    Text(text = row.chat.lastMessage, color = Color.Gray, maxLines = 1)
                }
            }
        }
    }

    private fun openChat(row: ChatsViewModel.ChatRow) {
        val other = row.other ?: return
        startActivityWithSlideInAnimation(
            ChatMessagesActivity.newIntent(
                this,
                chatId = row.chat.id,
                otherUserId = row.otherUserId,
                otherUserName = other.name,
            ),
        )
    }

    companion object {
        fun newIntent(context: Context): Intent = Intent(context, ChatsActivity::class.java)
    }
}
