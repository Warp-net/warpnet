/* Copyright 2025 Tusky Contributors
 *
 * This file is a part of Tusky.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Tusky is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Tusky; if not,
 * see <http://www.gnu.org/licenses>. */

package com.keylesspalace.tusky.viewmodel

import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import at.connyduck.calladapter.networkresult.fold
import at.connyduck.calladapter.networkresult.onSuccess
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.appstore.BlockEvent
import com.keylesspalace.tusky.appstore.EventHub
import com.keylesspalace.tusky.appstore.MuteEvent
import com.keylesspalace.tusky.appstore.PollVoteEvent
import com.keylesspalace.tusky.appstore.StatusChangedEvent
import com.keylesspalace.tusky.appstore.StatusDeletedEvent
import com.keylesspalace.tusky.components.compose.ComposeActivity
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.network.MastodonApi
import com.keylesspalace.tusky.ui.SnackbarError
import com.keylesspalace.tusky.util.getServerErrorMessage
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.launch

abstract class StatusActionsViewModel(
    private val mastodonApi: MastodonApi,
    private val eventHub: EventHub
) : ViewModel() {

    protected val errors = MutableSharedFlow<SnackbarError?>(
        replay = 0,
        extraBufferCapacity = 1,
        onBufferOverflow = BufferOverflow.DROP_OLDEST
    )
    val snackbarErrors: SharedFlow<SnackbarError?> = errors.asSharedFlow()

    private val _startComposing = MutableSharedFlow<ComposeActivity.ComposeOptions>(
        replay = 0,
        extraBufferCapacity = 1,
        onBufferOverflow = BufferOverflow.DROP_OLDEST
    )
    val startComposing = _startComposing.asSharedFlow()

    fun snackbarErrorShown() {
        viewModelScope.launch {
            errors.emit(null)
        }
    }

    fun reblog(statusId: String, reblog: Boolean, visibility: Status.Visibility = Status.Visibility.PUBLIC) {
        viewModelScope.launch {
            if (reblog) {
                mastodonApi.reblogStatus(statusId, visibility.stringValue)
            } else {
                mastodonApi.unreblogStatus(statusId)
            }.fold(
                onSuccess = { status ->
                    if (status.reblog != null) {
                        // when reblogging, the Mastodon Api does not return the reblogged status directly
                        // but the newly created status with reblog set to the reblogged status
                        eventHub.dispatch(StatusChangedEvent(status.reblog))
                    } else {
                        eventHub.dispatch(StatusChangedEvent(status))
                    }
                },
                onFailure = { e ->
                    Log.w(TAG, "Failed to reblog", e)
                }
            )
        }
    }

    fun favorite(statusId: String, favourite: Boolean) {
        viewModelScope.launch {
            if (favourite) {
                mastodonApi.favouriteStatus(statusId)
            } else {
                mastodonApi.unfavouriteStatus(statusId)
            }.fold(
                onSuccess = { status ->
                    eventHub.dispatch(StatusChangedEvent(status))
                },
                onFailure = { e ->
                    Log.w(TAG, "Failed to favorite", e)
                }
            )
        }
    }

    fun bookmark(statusId: String, bookmark: Boolean) {
        viewModelScope.launch {
            if (bookmark) {
                mastodonApi.bookmarkStatus(statusId)
            } else {
                mastodonApi.unbookmarkStatus(statusId)
            }.fold(
                onSuccess = { status ->
                    eventHub.dispatch(StatusChangedEvent(status))
                },
                onFailure = { e ->
                    Log.w(TAG, "Failed to bookmark", e)
                }
            )
        }
    }

    fun muteConversation(statusId: String, mute: Boolean) {
        viewModelScope.launch {
            if (mute) {
                mastodonApi.muteConversation(statusId)
            } else {
                mastodonApi.unmuteConversation(statusId)
            }.fold(
                onSuccess = { status ->
                    eventHub.dispatch(StatusChangedEvent(status))
                },
                onFailure = { e ->
                    Log.w(TAG, "Failed to mute conversation", e)
                }
            )
        }
    }

    fun mute(accountId: String, notifications: Boolean, duration: Int?) {
        viewModelScope.launch {
            mastodonApi.muteAccount(accountId, notifications, duration)
                .fold(
                    onSuccess = {
                        eventHub.dispatch(MuteEvent(accountId))
                    },
                    onFailure = { t ->
                        Log.w(TAG, "Failed to mute account", t)
                    }
                )
        }
    }

    fun block(accountId: String) {
        viewModelScope.launch {
            mastodonApi.blockAccount(accountId)
                .fold(
                    onSuccess = {
                        eventHub.dispatch(BlockEvent(accountId))
                    },
                    onFailure = { t ->
                        Log.w(TAG, "Failed to block account", t)
                    }
                )
        }
    }

    fun delete(statusId: String) {
        viewModelScope.launch {
            mastodonApi.deleteStatus(statusId, deleteMedia = true)
                .fold(
                    onSuccess = { eventHub.dispatch(StatusDeletedEvent(statusId)) },
                    onFailure = { Log.w(TAG, "Failed to delete status", it) }
                )
        }
    }

    fun pin(statusId: String, pin: Boolean) {
        viewModelScope.launch {
            if (pin) {
                mastodonApi.pinStatus(statusId)
            } else {
                mastodonApi.unpinStatus(statusId)
            }.fold({ status ->
                eventHub.dispatch(StatusChangedEvent(status))
            }, { e ->
                Log.w(TAG, "Failed to change pin state", e)
                val errorMessage = e.getServerErrorMessage()
                if (errorMessage != null) {
                    errors.emit(SnackbarError.StringMessage(errorMessage))
                } else if (pin) {
                    errors.emit(SnackbarError.ResourceMessage(R.string.failed_to_pin))
                } else {
                    errors.emit(SnackbarError.ResourceMessage(R.string.failed_to_unpin))
                }
            })
        }
    }

    fun voteInPoll(
        statusId: String,
        pollId: String,
        choices: List<Int>
    ) {
        if (choices.isEmpty()) {
            return
        }
        viewModelScope.launch {
            mastodonApi.voteInPoll(pollId, choices)
                .onSuccess { poll ->
                    eventHub.dispatch(PollVoteEvent(statusId, poll))
                }
        }
    }

    fun editStatus(status: Status) {
        viewModelScope.launch {
            mastodonApi.statusSource(status.id)
                .fold(
                    onSuccess = { source ->
                        val composeOptions = ComposeActivity.ComposeOptions(
                            content = source.text,
                            inReplyToId = status.inReplyToId,
                            visibility = status.visibility,
                            contentWarning = source.spoilerText,
                            mediaAttachments = status.attachments,
                            sensitive = status.sensitive,
                            language = status.language,
                            statusId = source.id,
                            poll = status.poll?.toNewPoll(status.createdAt),
                            kind = ComposeActivity.ComposeKind.EDIT_POSTED
                        )
                        _startComposing.emit(composeOptions)
                    },
                    onFailure = { e ->
                        Log.w(TAG, "Failed to load status source", e)
                        errors.emit(SnackbarError.ResourceMessage(R.string.error_status_source_load))
                    }
                )
        }
    }

    fun redraftStatus(status: Status) {
        viewModelScope.launch {
            mastodonApi.deleteStatus(status.id, deleteMedia = false)
                .fold(
                    onSuccess = { deletedStatus ->
                        eventHub.dispatch(StatusDeletedEvent(status.id))
                        val sourceStatus = if (deletedStatus.isEmpty) {
                            status.toDeletedStatus()
                        } else {
                            deletedStatus
                        }
                        val composeOptions = ComposeActivity.ComposeOptions(
                            content = sourceStatus.text,
                            inReplyToId = sourceStatus.inReplyToId,
                            visibility = sourceStatus.visibility,
                            contentWarning = sourceStatus.spoilerText,
                            mediaAttachments = sourceStatus.attachments,
                            sensitive = sourceStatus.sensitive,
                            modifiedInitialState = true,
                            language = sourceStatus.language,
                            poll = sourceStatus.poll?.toNewPoll(sourceStatus.createdAt),
                            kind = ComposeActivity.ComposeKind.NEW
                        )
                        _startComposing.emit(composeOptions)
                    },
                    onFailure = { e ->
                        Log.w(TAG, "Failed to load status source", e)
                        errors.emit(SnackbarError.ResourceMessage(R.string.error_deleting_status))
                    }
                )
        }
    }

    fun removeQuote(status: Status) {
        val quotedStatusId = status.quote?.quotedStatus?.id ?: return
        viewModelScope.launch {
            mastodonApi.removeQuote(quotedStatusId, status.id)
                .fold(
                    onSuccess = { status ->
                        eventHub.dispatch(StatusChangedEvent(status))
                    },
                    onFailure = { e ->
                        Log.w(TAG, "Failed to remove quote from status", e)
                        errors.emit(SnackbarError.ResourceMessage(R.string.error_status_source_load))
                    }
                )
        }
    }

    companion object {
        private const val TAG = "StatusActionsViewModel"
    }
}
