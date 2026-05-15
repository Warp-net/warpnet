/* Copyright 2025 Warpdroid Contributors
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

package site.warpnet.warpdroid.viewmodel

import android.os.SystemClock
import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import at.connyduck.calladapter.networkresult.fold
import at.connyduck.calladapter.networkresult.onSuccess
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.appstore.BlockEvent
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.MuteEvent
import site.warpnet.warpdroid.appstore.PollVoteEvent
import site.warpnet.warpdroid.appstore.TweetChangedEvent
import site.warpnet.warpdroid.appstore.TweetDeletedEvent
import site.warpnet.warpdroid.components.compose.ComposeActivity
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.ui.SnackbarError
import site.warpnet.warpdroid.util.getServerErrorMessage
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.launch

abstract class TweetActionsViewModel(
    private val warpnetApi: WarpnetApi,
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

    fun retweet(statusId: String, retweet: Boolean, visibility: Tweet.Visibility = Tweet.Visibility.PUBLIC) {
        viewModelScope.launch {
            if (retweet) {
                warpnetApi.retweetStatus(statusId, visibility.stringValue)
            } else {
                warpnetApi.unretweetStatus(statusId)
            }.fold(
                onSuccess = { status ->
                    if (status.retweet != null) {
                        // when retweeting, the Warpnet Api does not return the retweeted status directly
                        // but the newly created status with retweet set to the retweeted status
                        eventHub.dispatch(TweetChangedEvent(status.retweet))
                    } else {
                        eventHub.dispatch(TweetChangedEvent(status))
                    }
                },
                onFailure = { e ->
                    Log.w(TAG, "Failed to retweet", e)
                }
            )
        }
    }

    /**
     * Fire-and-forget view recording for a status that just appeared
     * on screen. Backend dedupes per (tweet, viewer) within a 30-min
     * window; we mirror that window client-side so a single status
     * doesn't fire the network call every recomposition while a long
     * timeline session is open, but a re-view after the window passes
     * is still recorded (matches what the server would accept).
     * Failures are logged in [WarpnetApi.recordView] and never
     * surface to the UI.
     */
    private val viewedStatusAt = LinkedHashMap<String, Long>()

    fun recordView(statusId: String, authorId: String) {
        if (statusId.isBlank() || authorId.isBlank()) return
        // Monotonic clock so wall-clock changes (manual / NTP) can't
        // skew dedupe windows.
        val now = SystemClock.elapsedRealtime()
        synchronized(viewedStatusAt) {
            // Drop entries older than the dedup window so the cache
            // can't grow without bound and so a re-view after the
            // window expires is allowed through.
            val it = viewedStatusAt.entries.iterator()
            while (it.hasNext()) {
                val (_, ts) = it.next()
                if (now - ts < VIEW_DEDUP_WINDOW_MS) break // LinkedHashMap is insertion-ordered
                it.remove()
            }
            val last = viewedStatusAt[statusId]
            if (last != null && now - last < VIEW_DEDUP_WINDOW_MS) return
            // Re-insert at the tail so the LRU-ish eviction above
            // walks oldest-first.
            viewedStatusAt.remove(statusId)
            viewedStatusAt[statusId] = now
            // Hard cap so a runaway recomposition burst can't pin
            // unbounded memory even if eviction by time is slow.
            while (viewedStatusAt.size > VIEW_DEDUP_MAX_ENTRIES) {
                val oldest = viewedStatusAt.entries.iterator()
                if (oldest.hasNext()) {
                    oldest.next()
                    oldest.remove()
                } else {
                    break
                }
            }
        }
        viewModelScope.launch {
            warpnetApi.recordView(statusId, authorId)
        }
    }

    fun like(statusId: String, authorId: String, like: Boolean) {
        viewModelScope.launch {
            if (like) {
                warpnetApi.likeStatus(statusId, authorId)
            } else {
                warpnetApi.unlikeStatus(statusId, authorId)
            }.fold(
                onSuccess = { status ->
                    eventHub.dispatch(TweetChangedEvent(status))
                },
                onFailure = { e ->
                    Log.w(TAG, "Failed to like", e)
                }
            )
        }
    }

    fun bookmark(statusId: String, bookmark: Boolean) {
        viewModelScope.launch {
            if (bookmark) {
                warpnetApi.bookmarkStatus(statusId)
            } else {
                warpnetApi.unbookmarkStatus(statusId)
            }.fold(
                onSuccess = { status ->
                    eventHub.dispatch(TweetChangedEvent(status))
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
                warpnetApi.muteConversation(statusId)
            } else {
                warpnetApi.unmuteConversation(statusId)
            }.fold(
                onSuccess = { status ->
                    eventHub.dispatch(TweetChangedEvent(status))
                },
                onFailure = { e ->
                    Log.w(TAG, "Failed to mute conversation", e)
                }
            )
        }
    }

    fun mute(accountId: String, notifications: Boolean, duration: Int?) {
        viewModelScope.launch {
            warpnetApi.muteAccount(accountId, notifications, duration)
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
            warpnetApi.blockAccount(accountId)
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
            warpnetApi.deleteStatus(statusId, deleteMedia = true)
                .fold(
                    onSuccess = { eventHub.dispatch(TweetDeletedEvent(statusId)) },
                    onFailure = { Log.w(TAG, "Failed to delete status", it) }
                )
        }
    }

    fun pin(statusId: String, pin: Boolean) {
        viewModelScope.launch {
            if (pin) {
                warpnetApi.pinStatus(statusId)
            } else {
                warpnetApi.unpinStatus(statusId)
            }.fold({ status ->
                eventHub.dispatch(TweetChangedEvent(status))
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
            warpnetApi.voteInPoll(pollId, choices)
                .onSuccess { poll ->
                    eventHub.dispatch(PollVoteEvent(statusId, poll))
                }
        }
    }

    fun editStatus(status: Tweet) {
        viewModelScope.launch {
            warpnetApi.statusSource(status.id)
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

    fun redraftStatus(status: Tweet) {
        viewModelScope.launch {
            warpnetApi.deleteStatus(status.id, deleteMedia = false)
                .fold(
                    onSuccess = { deletedStatus ->
                        eventHub.dispatch(TweetDeletedEvent(status.id))
                        val sourceStatus = if (deletedStatus.isEmpty) {
                            status.toDeletedTweet()
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

    fun removeQuote(status: Tweet) {
        val quotedStatusId = status.quote?.quotedStatus?.id ?: return
        viewModelScope.launch {
            warpnetApi.removeQuote(quotedStatusId, status.id)
                .fold(
                    onSuccess = { status ->
                        eventHub.dispatch(TweetChangedEvent(status))
                    },
                    onFailure = { e ->
                        Log.w(TAG, "Failed to remove quote from status", e)
                        errors.emit(SnackbarError.ResourceMessage(R.string.error_status_source_load))
                    }
                )
        }
    }

    companion object {
        private const val TAG = "TweetActionsViewModel"

        // Mirrors warpnet's database.ViewDedupTTL so the client doesn't
        // bother the node with calls it would dedupe anyway, while
        // still letting a re-view after the window through.
        private const val VIEW_DEDUP_WINDOW_MS: Long = 30L * 60 * 1000
        private const val VIEW_DEDUP_MAX_ENTRIES: Int = 512
    }
}
