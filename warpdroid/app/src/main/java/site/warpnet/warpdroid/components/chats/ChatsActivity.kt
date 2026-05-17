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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
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
    }

    @Composable
    private fun ChatRow(row: ChatsViewModel.ChatRow, onClick: () -> Unit) {
        val name = row.other?.name ?: row.chat.otherUserId
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
                otherUserId = row.chat.otherUserId,
                otherUserName = other.name,
            ),
        )
    }

    companion object {
        fun newIntent(context: Context): Intent = Intent(context, ChatsActivity::class.java)
    }
}
