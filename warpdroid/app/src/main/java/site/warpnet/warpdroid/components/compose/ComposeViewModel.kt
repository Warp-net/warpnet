/* Copyright 2019 Warpdroid Contributors
 *
 * This file is a part of Warpdroid.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Warpdroid is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Warpdroid; if not,
 * see <http://www.gnu.org/licenses>. */

package site.warpnet.warpdroid.components.compose

import android.net.Uri
import android.os.Parcelable
import android.util.Log
import androidx.core.net.toUri
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import at.connyduck.calladapter.networkresult.fold
import site.warpnet.warpdroid.components.compose.ComposeActivity.ComposeKind
import site.warpnet.warpdroid.components.compose.ComposeAutoCompleteAdapter.AutocompleteResult
import site.warpnet.warpdroid.components.instanceinfo.InstanceInfo
import site.warpnet.warpdroid.components.instanceinfo.InstanceInfoRepository
import site.warpnet.warpdroid.components.search.SearchType
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.entity.Emoji
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.service.MediaToSend
import site.warpnet.warpdroid.service.ServiceClient
import site.warpnet.warpdroid.service.TweetToSend
import site.warpnet.warpdroid.util.randomAlphanumericString
import site.warpnet.warpdroid.util.savedstateflow.SavedStateFlow
import dagger.assisted.Assisted
import dagger.assisted.AssistedFactory
import dagger.assisted.AssistedInject
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.shareIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.parcelize.Parcelize

@HiltViewModel(assistedFactory = ComposeViewModel.Factory::class)
class ComposeViewModel @AssistedInject constructor(
    private val api: WarpnetApi,
    private val accountManager: AccountManager,
    private val mediaUploader: MediaUploader,
    private val serviceClient: ServiceClient,
    private val state: SavedStateHandle,
    @Assisted("options") private val composeOptions: ComposeActivity.ComposeOptions?,
    instanceInfoRepo: InstanceInfoRepository
) : ViewModel() {

    internal var startingText: String? = null
    internal var postLanguage: String? = null
    private var startingContentWarning: String = ""
    private var currentContent: String? = ""
    private var currentContentWarning: String? = ""

    val instanceInfo: SharedFlow<InstanceInfo> = instanceInfoRepo::getUpdatedInstanceInfoOrFallback.asFlow()
        .shareIn(viewModelScope, SharingStarted.Eagerly, replay = 1)

    val emoji: SharedFlow<List<Emoji>> = instanceInfoRepo::getEmojis.asFlow()
        .shareIn(viewModelScope, SharingStarted.Eagerly, replay = 1)

    private val _markMediaAsSensitive: SavedStateFlow<Boolean> = SavedStateFlow(
        savedStateHandle = state,
        key = "MARK_MEDIA_AS_SENSITIVE",
        initialValue = composeOptions?.sensitive ?: (accountManager.activeAccount?.defaultMediaSensitivity == true)
    )
    val markMediaAsSensitive: StateFlow<Boolean> = _markMediaAsSensitive.asStateFlow()

    private val _statusVisibility: SavedStateFlow<Tweet.Visibility> = SavedStateFlow(
        savedStateHandle = state,
        key = "STATUS_VISIBILITY",
        initialValue = startingVisibility()
    )
    val statusVisibility: StateFlow<Tweet.Visibility> = _statusVisibility.asStateFlow()

    private val _showContentWarning: SavedStateFlow<Boolean> = SavedStateFlow(
        savedStateHandle = state,
        key = "SHOW_CONTENT_WARNING",
        initialValue = !composeOptions?.contentWarning.isNullOrEmpty()
    )
    val showContentWarning: StateFlow<Boolean> = _showContentWarning.asStateFlow()

    private val _media: SavedStateFlow<List<QueuedMedia>> = SavedStateFlow(
        savedStateHandle = state,
        key = MEDIA_KEY,
        initialValue = emptyList()
    )
    val media: StateFlow<List<QueuedMedia>> = _media.asStateFlow()

    private val _uploadError = MutableSharedFlow<Throwable>(
        replay = 0,
        extraBufferCapacity = 1,
        onBufferOverflow = BufferOverflow.DROP_OLDEST
    )
    val uploadError: SharedFlow<Throwable> = _uploadError.asSharedFlow()

    private val _closeConfirmation = MutableStateFlow(ConfirmationKind.NONE)
    val closeConfirmation: StateFlow<ConfirmationKind> = _closeConfirmation.asStateFlow()

    private val composeKind: ComposeKind
        get() = composeOptions?.kind ?: ComposeKind.NEW
    private val inReplyToId: String?
        get() = composeOptions?.inReplyToId
    private val modifiedInitialState: Boolean
        get() = composeOptions?.modifiedInitialState == true
    val editing: Boolean
        get() = !composeOptions?.statusId.isNullOrEmpty()

    // Used in ComposeActivity to pass state to result function when cropImage contract inflight
    var cropImageItemOld: QueuedMedia? = null

    init {
        // recreate media list
        val savedMediaState: List<QueuedMedia>? = state[MEDIA_KEY]
        if (savedMediaState != null) {
            // restart upload for media that was not completely uploaded
            _media.value.forEach { m ->
                if (m.state == QueuedMedia.State.UPLOADING) {
                    addMediaToQueue(
                        m.type,
                        m.uri,
                        m.mediaSize,
                        m.description,
                        m.focus,
                        m
                    )
                }
            }
        } else {
            // Warpdroid: drafts aren't persisted, so there are no draft attachments to restore.
            composeOptions?.mediaAttachments?.forEach { a ->
                // when coming from redraft or ScheduledTootActivity
                val mediaType = when (a.type) {
                    Attachment.Type.VIDEO, Attachment.Type.GIFV -> QueuedMedia.Type.VIDEO
                    Attachment.Type.UNKNOWN, Attachment.Type.IMAGE -> QueuedMedia.Type.IMAGE
                    Attachment.Type.AUDIO -> QueuedMedia.Type.AUDIO
                }
                addUploadedMedia(a.id, mediaType, a.url.toUri(), a.description, a.meta?.focus)
            }
        }

        startingText = composeOptions?.content
        currentContent = composeOptions?.content
        postLanguage = composeOptions?.language

        val mentionedUsernames = composeOptions?.mentionedUsernames
        if (mentionedUsernames != null) {
            val builder = StringBuilder()
            for (name in mentionedUsernames) {
                builder.append('@')
                builder.append(name)
                builder.append(' ')
            }
            startingText = builder.toString()
        }

        updateCloseConfirmation()
    }

    fun startingVisibility(): Tweet.Visibility {
        val tweetVisibility = composeOptions?.visibility
        if (tweetVisibility != null) {
            return tweetVisibility
        }

        val activeAccount = accountManager.activeAccount ?: return Tweet.Visibility.UNKNOWN
        val preferredVisibility = if (inReplyToId != null) {
            activeAccount.defaultReplyPrivacy.toVisibilityOr(activeAccount.defaultPostPrivacy)
        } else {
            activeAccount.defaultPostPrivacy
        }

        val replyVisibility = composeOptions?.replyVisibility ?: Tweet.Visibility.UNKNOWN
        return Tweet.Visibility.fromInt(
            preferredVisibility.int.coerceAtLeast(replyVisibility.int)
        )
    }

    fun pickMedia(uri: Uri) {
        pickMedia(listOf(MediaData(uri)))
    }

    fun pickMedia(mediaList: List<MediaData>) = viewModelScope.launch(Dispatchers.IO) {
        val instanceInfo = instanceInfo.first()
        mediaList.map { m ->
            async { mediaUploader.prepareMedia(m.uri, instanceInfo) }
        }.forEachIndexed { index, preparedMedia ->
            preparedMedia.await().fold({ (type, uri, size) ->
                if (type != QueuedMedia.Type.IMAGE &&
                    _media.value.firstOrNull()?.type == QueuedMedia.Type.IMAGE
                ) {
                    _uploadError.emit(VideoOrImageException())
                } else {
                    val pickedMedia = mediaList[index]
                    addMediaToQueue(type, uri, size, pickedMedia.description, pickedMedia.focus)
                }
            }, { error ->
                _uploadError.emit(error)
            })
        }
    }

    fun addMediaToQueue(
        type: QueuedMedia.Type,
        uri: Uri,
        mediaSize: Long,
        description: String? = null,
        focus: Attachment.Focus? = null,
        replaceItem: QueuedMedia? = null
    ): QueuedMedia {
        val mediaItem = QueuedMedia(
            localId = mediaUploader.getNewLocalMediaId(),
            uri = uri,
            type = type,
            mediaSize = mediaSize,
            description = description,
            focus = focus,
            state = QueuedMedia.State.UPLOADING
        )

        _media.update { mediaList ->
            if (replaceItem != null) {
                mediaUploader.cancelUploadScope(replaceItem.localId)
                mediaList.map {
                    if (it.localId == replaceItem.localId) mediaItem else it
                }
            } else { // Append
                mediaList + mediaItem
            }
        }

        viewModelScope.launch {
            mediaUploader
                .uploadMedia(mediaItem, instanceInfo.first())
                .collect { event ->
                    val item = _media.value.find { it.localId == mediaItem.localId }
                        ?: return@collect
                    val newMediaItem = when (event) {
                        is UploadEvent.ProgressEvent ->
                            item.copy(uploadPercent = event.percentage)

                        is UploadEvent.FinishedEvent ->
                            item.copy(
                                id = event.mediaId,
                                uploadPercent = -1,
                                state = if (event.processed) {
                                    QueuedMedia.State.PROCESSED
                                } else {
                                    QueuedMedia.State.UNPROCESSED
                                }
                            )
                        is UploadEvent.ErrorEvent -> {
                            _media.update { mediaList -> mediaList.filter { it.localId != mediaItem.localId } }
                            _uploadError.emit(event.error)
                            return@collect
                        }
                    }
                    _media.update { mediaList ->
                        mediaList.map { mediaItem ->
                            if (mediaItem.localId == newMediaItem.localId) {
                                newMediaItem
                            } else {
                                mediaItem
                            }
                        }
                    }
                }
        }
        updateCloseConfirmation()
        return mediaItem
    }

    fun changeStatusVisibility(visibility: Tweet.Visibility) {
        _statusVisibility.value = visibility
    }

    private fun addUploadedMedia(
        id: String,
        type: QueuedMedia.Type,
        uri: Uri,
        description: String?,
        focus: Attachment.Focus?
    ) {
        _media.update { mediaList ->
            val mediaItem = QueuedMedia(
                localId = mediaUploader.getNewLocalMediaId(),
                uri = uri,
                type = type,
                mediaSize = 0,
                uploadPercent = -1,
                id = id,
                description = description,
                focus = focus,
                state = QueuedMedia.State.PUBLISHED
            )
            mediaList + mediaItem
        }
    }

    fun removeMediaFromQueue(item: QueuedMedia) {
        mediaUploader.cancelUploadScope(item.localId)
        _media.update { mediaList -> mediaList.filter { it.localId != item.localId } }
        updateCloseConfirmation()
    }

    fun toggleMarkSensitive() {
        this._markMediaAsSensitive.value = this._markMediaAsSensitive.value != true
    }

    fun updateContent(newContent: String?) {
        currentContent = newContent
        updateCloseConfirmation()
    }

    fun updateContentWarning(newContentWarning: String?) {
        currentContentWarning = newContentWarning
        updateCloseConfirmation()
    }

    private fun updateCloseConfirmation() {
        val contentWarning = if (_showContentWarning.value) {
            currentContentWarning
        } else {
            ""
        }
        this._closeConfirmation.value = if (didChange(currentContent, contentWarning)) {
            when (composeKind) {
                ComposeKind.NEW -> if (isEmpty(currentContent, contentWarning)) {
                    ConfirmationKind.NONE
                } else {
                    ConfirmationKind.SAVE_OR_DISCARD
                }
                ComposeKind.EDIT_DRAFT -> if (isEmpty(currentContent, contentWarning)) {
                    ConfirmationKind.CONTINUE_EDITING_OR_DISCARD_DRAFT
                } else {
                    ConfirmationKind.UPDATE_OR_DISCARD
                }
                ComposeKind.EDIT_POSTED -> ConfirmationKind.CONTINUE_EDITING_OR_DISCARD_CHANGES
                ComposeKind.EDIT_SCHEDULED -> ConfirmationKind.CONTINUE_EDITING_OR_DISCARD_CHANGES
            }
        } else {
            ConfirmationKind.NONE
        }
    }

    private fun didChange(content: String?, contentWarning: String?): Boolean {
        val textChanged = content.orEmpty() != startingText.orEmpty()
        val contentWarningChanged = contentWarning.orEmpty() != startingContentWarning
        val mediaChanged = _media.value.isNotEmpty()

        return modifiedInitialState || textChanged || contentWarningChanged || mediaChanged
    }

    private fun isEmpty(content: String?, contentWarning: String?): Boolean {
        return !modifiedInitialState && (content.isNullOrBlank() && contentWarning.isNullOrBlank() && _media.value.isEmpty())
    }

    fun contentWarningChanged(value: Boolean) {
        _showContentWarning.value = value
        updateCloseConfirmation()
    }

    fun deleteDraft() {
        // TODO(warpdroid): drafts are not persisted; nothing to delete.
    }

    fun stopUploads() {
        mediaUploader.cancelUploadScope(*_media.value.map { it.localId }.toIntArray())
    }

    @Suppress("UNUSED_PARAMETER")
    suspend fun saveDraft(content: String, contentWarning: String) {
        // TODO(warpdroid): drafts persistence was removed with Room; no-op.
    }

    /**
     * Send status to the server.
     * Uses current state plus provided arguments.
     */
    suspend fun sendStatus(content: String, spoilerText: String, accountId: Long) {
        val attachedMedia = _media.value.map { item ->
            MediaToSend(
                localId = item.localId,
                id = item.id,
                uri = item.uri.toString(),
                description = item.description,
                focus = item.focus,
                processed = item.state == QueuedMedia.State.PROCESSED || item.state == QueuedMedia.State.PUBLISHED
            )
        }
        val tweetToSend = TweetToSend(
            text = content,
            warningText = spoilerText,
            visibility = _statusVisibility.value.stringValue,
            sensitive = attachedMedia.isNotEmpty() && (_markMediaAsSensitive.value || _showContentWarning.value),
            media = attachedMedia,
            inReplyToId = inReplyToId,
            replyingTweetContent = null,
            replyingStatusAuthorUsername = null,
            accountId = accountId,
            draftId = composeOptions?.draftId ?: 0,
            idempotencyKey = randomAlphanumericString(16),
            retries = 0,
            language = postLanguage,
            statusId = composeOptions?.statusId
        )

        serviceClient.sendTweet(tweetToSend)
    }

    private fun updateMediaItem(localId: Int, mutator: (QueuedMedia) -> QueuedMedia) {
        _media.update { mediaList ->
            mediaList.map { mediaItem ->
                if (mediaItem.localId == localId) {
                    mutator(mediaItem)
                } else {
                    mediaItem
                }
            }
        }
    }

    fun updateDescription(localId: Int, description: String) {
        updateMediaItem(localId) { mediaItem ->
            mediaItem.copy(description = description)
        }
    }

    fun updateFocus(localId: Int, focus: Attachment.Focus) {
        updateMediaItem(localId) { mediaItem ->
            mediaItem.copy(focus = focus)
        }
    }

    fun searchAutocompleteSuggestions(token: String): List<AutocompleteResult> {
        return when (token[0]) {
            '@' -> runBlocking {
                api.searchAccounts(query = token.substring(1), limit = 10)
                    .fold({ accounts ->
                        accounts.map { AutocompleteResult.AccountResult(it) }
                    }, { e ->
                        Log.e(TAG, "Autocomplete search for $token failed.", e)
                        emptyList()
                    })
            }
            '#' -> runBlocking {
                api.search(
                    query = token,
                    type = SearchType.Hashtag.apiParameter,
                    limit = 10
                )
                    .fold({ searchResult ->
                        searchResult.hashtags.map { AutocompleteResult.HashtagResult(it.name) }
                    }, { e ->
                        Log.e(TAG, "Autocomplete search for $token failed.", e)
                        emptyList()
                    })
            }

            ':' -> {
                val emojiList = emoji.replayCache.firstOrNull() ?: return emptyList()
                val incomplete = token.substring(1)

                emojiList.filter { emoji ->
                    emoji.shortcode.contains(incomplete, ignoreCase = true)
                }.sortedBy { emoji ->
                    emoji.shortcode.indexOf(incomplete, ignoreCase = true)
                }.map { emoji ->
                    AutocompleteResult.EmojiResult(emoji)
                }
            }

            else -> {
                Log.w(TAG, "Unexpected autocompletion token: $token")
                emptyList()
            }
        }
    }

    private companion object {
        const val TAG = "ComposeViewModel"

        private const val MEDIA_KEY = "MEDIA"
    }

    enum class ConfirmationKind {
        NONE, // just close
        SAVE_OR_DISCARD,
        UPDATE_OR_DISCARD,
        CONTINUE_EDITING_OR_DISCARD_CHANGES, // editing post
        CONTINUE_EDITING_OR_DISCARD_DRAFT // edit draft
    }

    @Parcelize
    data class QueuedMedia(
        val localId: Int,
        val uri: Uri,
        val type: Type,
        val mediaSize: Long,
        val uploadPercent: Int = 0,
        val id: String? = null,
        val description: String? = null,
        val focus: Attachment.Focus? = null,
        val state: State
    ) : Parcelable {
        enum class Type {
            IMAGE,
            VIDEO,
            AUDIO
        }
        enum class State {
            UPLOADING,
            UNPROCESSED,
            PROCESSED,
            PUBLISHED
        }
    }

    data class MediaData(
        val uri: Uri,
        val description: String? = null,
        val focus: Attachment.Focus? = null
    )

    @AssistedFactory
    interface Factory {
        fun create(
            @Assisted("options") options: ComposeActivity.ComposeOptions?
        ): ComposeViewModel
    }
}

/**
 * Thrown when trying to add an image when video is already present or the other way around
 */
class VideoOrImageException : Exception()
