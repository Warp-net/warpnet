/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.chats

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import site.warpnet.transport.dto.WarpnetChat
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.TimelineAccount
import site.warpnet.warpdroid.warpnet.WarpnetRepository

@HiltViewModel
class ChatsViewModel @Inject constructor(
    private val repo: WarpnetRepository,
    private val accountManager: AccountManager,
) : ViewModel() {

    data class ChatRow(
        val chat: WarpnetChat,
        val other: TimelineAccount?,
    )

    data class State(
        val chats: List<ChatRow> = emptyList(),
        val loading: Boolean = true,
        val error: Throwable? = null,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    private var loadJob: Job? = null

    init {
        reload()
    }

    fun reload() {
        loadJob?.cancel()
        loadJob = viewModelScope.launch {
            val userId = accountManager.activeAccount?.accountId
            if (userId.isNullOrEmpty()) {
                _state.update { it.copy(loading = false) }
                return@launch
            }
            _state.update { it.copy(loading = true, error = null) }
            try {
                val (chats, _) = repo.getChats(userId)
                val rows = chats.map { chat ->
                    val other = runCatching { repo.getTimelineAccount(chat.otherUserId) }.getOrNull()
                    ChatRow(chat = chat, other = other)
                }
                _state.update { it.copy(chats = rows, loading = false) }
            } catch (e: Throwable) {
                _state.update { it.copy(loading = false, error = e) }
            }
        }
    }

    fun deleteChat(chatId: String) {
        viewModelScope.launch {
            val userId = accountManager.activeAccount?.accountId ?: return@launch
            runCatching { repo.deleteChat(userId, chatId) }
                .onSuccess {
                    _state.update { s -> s.copy(chats = s.chats.filter { it.chat.id != chatId }) }
                }
        }
    }
}
