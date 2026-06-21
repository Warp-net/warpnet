/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.chats

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import java.time.Instant
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import site.warpnet.transport.dto.WarpnetMessage
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.warpnet.WarpnetRepository

@HiltViewModel
class ChatMessagesViewModel @Inject constructor(
    private val repo: WarpnetRepository,
    private val accountManager: AccountManager,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val chatId: String = savedStateHandle.get<String>("chat_id").orEmpty()
    private val otherUserId: String = savedStateHandle.get<String>("other_user_id").orEmpty()

    data class State(
        val messages: List<WarpnetMessage> = emptyList(),
        val loading: Boolean = true,
        val sending: Boolean = false,
        val error: Throwable? = null,
        val ownUserId: String = "",
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    private var loadJob: Job? = null
    private var pollJob: Job? = null

    init {
        reload()
    }

    fun startPolling() {
        pollJob?.cancel()
        pollJob = viewModelScope.launch {
            while (isActive) {
                val userId = accountManager.activeAccount?.accountId
                if (!userId.isNullOrEmpty() && chatId.isNotEmpty()) {
                    val limit = maxOf(_state.value.messages.size, DEFAULT_PAGE)
                    val msgs = runCatching {
                        repo.getMessages(ownerId = userId, chatId = chatId, limit = limit).first
                    }.getOrNull()
                    if (msgs != null) {
                        _state.update { s ->
                            val next = msgs.orderedForDisplay()
                            if (next == s.messages) s else s.copy(messages = next)
                        }
                    }
                }
                delay(POLL_INTERVAL_MS)
            }
        }
    }

    fun stopPolling() {
        pollJob?.cancel()
    }

    override fun onCleared() {
        pollJob?.cancel()
        super.onCleared()
    }

    fun reload() {
        loadJob?.cancel()
        loadJob = viewModelScope.launch {
            val userId = accountManager.activeAccount?.accountId
            if (userId.isNullOrEmpty() || chatId.isEmpty()) {
                _state.update { it.copy(loading = false, ownUserId = userId.orEmpty()) }
                return@launch
            }
            _state.update { it.copy(loading = true, error = null, ownUserId = userId) }
            try {
                val (msgs, _) = repo.getMessages(ownerId = userId, chatId = chatId)
                // Wire order isn't reliably time-sorted; sort it ourselves.
                _state.update { it.copy(messages = msgs.orderedForDisplay(), loading = false) }
            } catch (e: Throwable) {
                _state.update { it.copy(loading = false, error = e) }
            }
        }
    }

    fun send(text: String) {
        if (text.isBlank()) return
        val userId = accountManager.activeAccount?.accountId ?: return
        if (chatId.isEmpty() || otherUserId.isEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(sending = true) }
            val sent = runCatching {
                repo.sendMessage(
                    chatId = chatId,
                    senderId = userId,
                    receiverId = otherUserId,
                    text = text,
                )
            }.getOrNull()
            _state.update { s ->
                val next = if (sent != null) (s.messages + sent).orderedForDisplay() else s.messages
                s.copy(messages = next, sending = false)
            }
        }
    }

    private fun List<WarpnetMessage>.orderedForDisplay(): List<WarpnetMessage> {
        // Must equal the LazyColumn item key; used for both de-dup and the sort tie-break.
        fun key(m: WarpnetMessage) = m.id.ifEmpty { m.createdAt + m.text }
        return distinctBy(::key).sortedWith(
            compareBy(
                { runCatching { Instant.parse(it.createdAt) }.getOrNull() ?: Instant.EPOCH },
                ::key,
            ),
        )
    }

    companion object {
        private const val POLL_INTERVAL_MS = 3000L
        private const val DEFAULT_PAGE = 40
    }
}
