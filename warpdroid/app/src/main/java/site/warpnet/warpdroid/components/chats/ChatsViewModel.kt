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
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import site.warpnet.transport.dto.WarpnetChat
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.TimelineUser
import site.warpnet.warpdroid.warpnet.WarpnetRepository

@HiltViewModel
class ChatsViewModel @Inject constructor(
    private val repo: WarpnetRepository,
    private val accountManager: AccountManager,
) : ViewModel() {

    data class ChatRow(
        val chat: WarpnetChat,
        val other: TimelineUser?,
    )

    data class State(
        val chats: List<ChatRow> = emptyList(),
        val loading: Boolean = true,
        val error: Throwable? = null,
    )

    // New-chat composer: a user search that, on pick, creates the 1:1 chat
    // and reports the resulting chat id back so the screen can open it.
    data class NewChatState(
        val visible: Boolean = false,
        val query: String = "",
        val results: List<TimelineUser> = emptyList(),
        val searching: Boolean = false,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    private val _newChat = MutableStateFlow(NewChatState())
    val newChat: StateFlow<NewChatState> = _newChat.asStateFlow()

    private var loadJob: Job? = null
    private var searchJob: Job? = null

    init {
        reload()
    }

    fun showNewChat() = _newChat.update { NewChatState(visible = true) }

    fun hideNewChat() {
        searchJob?.cancel()
        _newChat.update { NewChatState(visible = false) }
    }

    fun onNewChatQuery(query: String) {
        _newChat.update { it.copy(query = query) }
        searchJob?.cancel()
        if (query.isBlank()) {
            _newChat.update { it.copy(results = emptyList(), searching = false) }
            return
        }
        searchJob = viewModelScope.launch {
            delay(300) // debounce keystrokes before hitting the fat node
            _newChat.update { it.copy(searching = true) }
            val (users, _) = runCatching { repo.searchAccounts(query) }
                .getOrDefault(emptyList<TimelineUser>() to "")
            _newChat.update { it.copy(results = users, searching = false) }
        }
    }

    fun startChat(other: TimelineUser, onOpen: (chatId: String, otherUserId: String, name: String) -> Unit) {
        val userId = accountManager.activeAccount?.accountId ?: return
        viewModelScope.launch {
            val chat = runCatching { repo.createChat(ownerId = userId, otherUserId = other.id) }.getOrNull()
            if (chat != null && chat.id.isNotEmpty()) {
                _newChat.update { NewChatState(visible = false) }
                onOpen(chat.id, other.id, other.name)
                reload()
            }
        }
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
                    val other = runCatching { repo.getTimelineUser(chat.otherUserId) }.getOrNull()
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
