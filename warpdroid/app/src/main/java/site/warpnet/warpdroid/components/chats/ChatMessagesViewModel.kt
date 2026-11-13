/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.chats

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.media.MediaMetadataRetriever
import android.net.Uri
import android.util.Base64
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import dagger.hilt.android.qualifiers.ApplicationContext
import java.io.ByteArrayOutputStream
import java.time.Instant
import javax.inject.Inject
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import site.warpnet.transport.WarpnetLimits
import site.warpnet.transport.dto.WarpnetMessage
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.util.getImageOrientation
import site.warpnet.warpdroid.util.reorientBitmap
import site.warpnet.warpdroid.warpnet.WarpnetRepository

// Shared with ChatMessagesActivity's LazyColumn key so de-dup and rendered keys stay in sync.
// The fixed-length-ULID senderId prefix keeps the id-less fallback collision-resistant.
internal fun messageDisplayKey(m: WarpnetMessage): String =
    m.id.ifEmpty { "${m.senderId}${m.createdAt}${m.text}" }

@HiltViewModel
class ChatMessagesViewModel @Inject constructor(
    @ApplicationContext private val context: Context,
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
        // Up to four images or one video, like the desktop composer.
        val attachments: List<Uri> = emptyList(),
        val attachedVideo: Boolean = false,
        val sendFailed: Boolean = false,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    private var loadJob: Job? = null
    private var pollJob: Job? = null

    init {
        reload()
    }

    suspend fun isOnline(userId: String): Boolean? = repo.isUserOnline(userId)

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

    /** Returns false when the picked set mixes a video with anything else. */
    fun attach(uris: List<Uri>): Boolean {
        if (uris.isEmpty()) return true
        val videos = uris.count { context.contentResolver.getType(it).orEmpty().startsWith("video/") }
        if (videos > 0 && uris.size > 1) return false
        _state.update {
            it.copy(attachments = uris.take(WarpnetLimits.MAX_IMAGES_PER_TWEET), attachedVideo = videos > 0)
        }
        return true
    }

    fun clearAttachments() {
        _state.update { it.copy(attachments = emptyList(), attachedVideo = false) }
    }

    fun consumeSendFailure() {
        _state.update { it.copy(sendFailed = false) }
    }

    fun send(text: String) {
        val media = _state.value.attachments
        val isVideo = _state.value.attachedVideo
        if (text.isBlank() && media.isEmpty()) return
        val userId = accountManager.activeAccount?.accountId ?: return
        if (chatId.isEmpty() || otherUserId.isEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(sending = true, sendFailed = false) }
            val sent = runCatching {
                var imageKeys: List<String> = emptyList()
                var videoKey: String? = null
                if (isVideo) {
                    val (video, poster) = withContext(Dispatchers.IO) { readVideo(media.first()) }
                    imageKeys = listOfNotNull(poster?.let { repo.uploadImage(it, chat = true).ifBlank { null } })
                    videoKey = repo.uploadVideo(video, chat = true).ifBlank {
                        throw IllegalStateException("the node stored no video key")
                    }
                } else if (media.isNotEmpty()) {
                    imageKeys = media.map { uri ->
                        val image = withContext(Dispatchers.IO) { readImage(uri) }
                        repo.uploadImage(image, chat = true).ifBlank {
                            throw IllegalStateException("the node stored no image key")
                        }
                    }
                }
                repo.sendMessage(
                    chatId = chatId,
                    senderId = userId,
                    receiverId = otherUserId,
                    text = text,
                    imageKeys = imageKeys,
                    videoKey = videoKey,
                ) ?: throw IllegalStateException("empty send response")
            }.getOrNull()
            _state.update { s ->
                if (sent == null) {
                    s.copy(sending = false, sendFailed = true)
                } else {
                    s.copy(
                        messages = (s.messages + sent).orderedForDisplay(),
                        sending = false,
                        attachments = emptyList(),
                        attachedVideo = false,
                    )
                }
            }
        }
    }

    // Phone photos run to several MiB; scale to the desktop's working size
    // before base64 makes them a third bigger again.
    private fun readImage(uri: Uri): String {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        context.contentResolver.openInputStream(uri).use { BitmapFactory.decodeStream(it, null, bounds) }
        var sample = 1
        while (maxOf(bounds.outWidth, bounds.outHeight) / sample > MAX_IMAGE_SIDE) sample *= 2
        val decoded = context.contentResolver.openInputStream(uri).use {
            BitmapFactory.decodeStream(it, null, BitmapFactory.Options().apply { inSampleSize = sample })
        }
        val bitmap = reorientBitmap(decoded, getImageOrientation(uri, context.contentResolver))
            ?: throw IllegalArgumentException("not an image: $uri")
        return "image/jpeg,${bitmap.toJpegBase64()}"
    }

    private fun readVideo(uri: Uri): Pair<String, String?> {
        val mime = context.contentResolver.getType(uri).orEmpty().lowercase()
        require(mime in WarpnetLimits.ACCEPTED_VIDEO_MIME_TYPES) { "unsupported video type $mime" }
        val bytes = context.contentResolver.openInputStream(uri).use { requireNotNull(it).readBytes() }
        require(bytes.size <= WarpnetLimits.MAX_VIDEO_BYTES) { "video is larger than 36 MiB" }
        val poster = runCatching {
            MediaMetadataRetriever().run {
                try {
                    setDataSource(context, uri)
                    getFrameAtTime(0)
                } finally {
                    release()
                }
            }
        }.getOrNull()?.let { "image/jpeg,${it.toJpegBase64()}" }
        return "data:$mime;base64,${Base64.encodeToString(bytes, Base64.NO_WRAP)}" to poster
    }

    private fun Bitmap.toJpegBase64(): String {
        val out = ByteArrayOutputStream()
        compress(Bitmap.CompressFormat.JPEG, JPEG_QUALITY, out)
        return Base64.encodeToString(out.toByteArray(), Base64.NO_WRAP)
    }

    fun deleteMessage(message: WarpnetMessage) {
        val messageId = message.id
        if (chatId.isEmpty() || messageId.isEmpty()) return
        viewModelScope.launch {
            runCatching { repo.deleteMessage(chatId = chatId, messageId = messageId) }
                .onSuccess {
                    _state.update { s ->
                        s.copy(messages = s.messages.filterNot { it.id == messageId })
                    }
                }
        }
    }

    private fun List<WarpnetMessage>.orderedForDisplay(): List<WarpnetMessage> =
        distinctBy(::messageDisplayKey).sortedWith(
            compareBy(
                { runCatching { Instant.parse(it.createdAt) }.getOrNull() ?: Instant.EPOCH },
                ::messageDisplayKey,
            ),
        )

    companion object {
        private const val POLL_INTERVAL_MS = 3000L
        private const val DEFAULT_PAGE = 40
        private const val MAX_IMAGE_SIDE = 2048
        private const val JPEG_QUALITY = 85
    }
}
