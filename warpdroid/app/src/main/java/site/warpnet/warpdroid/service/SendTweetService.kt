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

package site.warpnet.warpdroid.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.ClipData
import android.content.ClipDescription
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.IBinder
import android.os.Parcelable
import android.util.Log
import androidx.annotation.StringRes
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import at.connyduck.calladapter.networkresult.fold
import site.warpnet.warpdroid.MainActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.TweetChangedEvent
import site.warpnet.warpdroid.appstore.TweetComposedEvent
import site.warpnet.warpdroid.appstore.TweetScheduledEvent
import site.warpnet.warpdroid.components.compose.MediaUploader
import site.warpnet.warpdroid.components.compose.UploadEvent
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.entity.MediaAttribute
import site.warpnet.warpdroid.entity.NewPoll
import site.warpnet.warpdroid.entity.NewTweet
import site.warpnet.warpdroid.entity.ScheduledTweetReply
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.util.getParcelableExtraCompat
import site.warpnet.warpdroid.util.unsafeLazy
import dagger.hilt.android.AndroidEntryPoint
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit
import javax.inject.Inject
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.parcelize.Parcelize

@AndroidEntryPoint
class SendTweetService : Service() {

    @Inject
    lateinit var warpnetApi: WarpnetApi

    @Inject
    lateinit var accountManager: AccountManager

    @Inject
    lateinit var eventHub: EventHub

    @Inject
    lateinit var mediaUploader: MediaUploader

    private val supervisorJob = SupervisorJob()
    private val serviceScope = CoroutineScope(Dispatchers.Main + supervisorJob)

    private val statusesToSend = ConcurrentHashMap<Int, TweetToSend>()
    private val sendJobs = ConcurrentHashMap<Int, Job>()

    private val notificationManager by unsafeLazy {
        getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
    }

    override fun onBind(intent: Intent): IBinder? = null

    override fun onStartCommand(intent: Intent, flags: Int, startId: Int): Int {
        if (intent.hasExtra(KEY_STATUS)) {
            val statusToSend: TweetToSend = intent.getParcelableExtraCompat(KEY_STATUS)
                ?: throw IllegalStateException("SendTweetService started without $KEY_STATUS extra")

            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                val channel =
                    NotificationChannel(
                        CHANNEL_ID,
                        getString(R.string.send_post_notification_channel_name),
                        NotificationManager.IMPORTANCE_LOW
                    )
                notificationManager.createNotificationChannel(channel)
            }

            var notificationText = statusToSend.warningText
            if (notificationText.isBlank()) {
                notificationText = statusToSend.text
            }

            val builder = NotificationCompat.Builder(this, CHANNEL_ID)
                .setSmallIcon(R.drawable.warpdroid_notification_icon)
                .setContentTitle(getString(R.string.send_post_notification_title))
                .setContentText(notificationText)
                .setProgress(1, 0, true)
                .setOngoing(true)
                .setColor(getColor(R.color.notification_color))
                .addAction(
                    0,
                    getString(android.R.string.cancel),
                    cancelSendingIntent(sendingNotificationId)
                )

            if (statusesToSend.isEmpty() || Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_DETACH)
                startForeground(sendingNotificationId, builder.build())
            } else {
                notificationManager.notify(sendingNotificationId, builder.build())
            }

            statusesToSend[sendingNotificationId] = statusToSend
            sendStatus(sendingNotificationId--)
        } else if (intent.hasExtra(KEY_CANCEL)) {
            cancelSending(intent.getIntExtra(KEY_CANCEL, 0))
        }

        return START_NOT_STICKY
    }

    override fun onTimeout(startId: Int) {
        // https://developer.android.com/about/versions/14/changes/fgs-types-required#short-service
        // max time for short service reached on Android 14+, stop sending
        statusesToSend.forEach { (statusId, _) ->
            serviceScope.launch {
                failSending(statusId)
            }
        }
    }

    private fun sendStatus(statusId: Int) {
        // when statusToSend == null, sending has been canceled
        val statusToSend = statusesToSend[statusId] ?: return

        // when account == null, user has logged out, cancel sending
        val account = accountManager.getAccountById(statusToSend.accountId)

        if (account == null) {
            statusesToSend.remove(statusId)
            notificationManager.cancel(statusId)
            stopSelfWhenDone()
            return
        }

        statusToSend.retries++

        sendJobs[statusId] = serviceScope.launch {
            // first, wait for media uploads to finish
            val media = statusToSend.media.map { mediaItem ->
                if (mediaItem.id == null) {
                    when (val uploadState = mediaUploader.getMediaUploadState(mediaItem.localId)) {
                        is UploadEvent.FinishedEvent -> mediaItem.copy(id = uploadState.mediaId, processed = uploadState.processed)
                        is UploadEvent.ErrorEvent -> {
                            Log.w(TAG, "failed uploading media", uploadState.error)
                            failSending(statusId)
                            stopSelfWhenDone()
                            return@launch
                        }
                    }
                } else {
                    mediaItem
                }
            }

            // then wait until server finished processing the media
            try {
                var mediaCheckRetries = 0
                while (media.any { mediaItem -> !mediaItem.processed }) {
                    delay(1000L * mediaCheckRetries)
                    media.forEach { mediaItem ->
                        if (!mediaItem.processed) {
                            when (warpnetApi.getMedia(mediaItem.id!!).code()) {
                                200 -> mediaItem.processed = true // success
                                206 -> { } // media is still being processed, continue checking
                                else -> { // some kind of server error, retrying probably doesn't make sense
                                    failSending(statusId)
                                    stopSelfWhenDone()
                                    return@launch
                                }
                            }
                        }
                    }
                    mediaCheckRetries++
                }
            } catch (e: Exception) {
                Log.w(TAG, "failed getting media status", e)
                retrySending(statusId)
                return@launch
            }

            val isNew = statusToSend.statusId == null

            if (isNew) {
                media.forEach { mediaItem ->
                    if (mediaItem.processed && (mediaItem.description != null || mediaItem.focus != null)) {
                        warpnetApi.updateMedia(mediaItem.id!!, mediaItem.description, mediaItem.focus?.toWarpnetApiString())
                            .fold({
                            }, { throwable ->
                                Log.w(TAG, "failed to update media on status send", throwable)
                                failOrRetry(throwable, statusId)

                                return@launch
                            })
                    }
                }
            }

            // finally, send the new status
            val newStatus = NewTweet(
                status = statusToSend.text,
                warningText = statusToSend.warningText,
                inReplyToId = statusToSend.inReplyToId,
                visibility = statusToSend.visibility,
                sensitive = statusToSend.sensitive,
                mediaIds = media.map { it.id!! },
                scheduledAt = statusToSend.scheduledAt,
                poll = statusToSend.poll,
                language = statusToSend.language,
                mediaAttributes = media.map { mediaItem ->
                    MediaAttribute(
                        id = mediaItem.id!!,
                        description = mediaItem.description,
                        focus = mediaItem.focus?.toWarpnetApiString(),
                        thumbnail = null
                    )
                }
            )

            val scheduled = !statusToSend.scheduledAt.isNullOrEmpty()

            val sendResult = if (isNew) {
                if (!scheduled) {
                    warpnetApi.createStatus(
                        "Bearer " + account.accessToken,
                        account.domain,
                        statusToSend.idempotencyKey,
                        newStatus
                    )
                } else {
                    warpnetApi.createScheduledStatus(
                        "Bearer " + account.accessToken,
                        account.domain,
                        statusToSend.idempotencyKey,
                        newStatus
                    )
                }
            } else {
                warpnetApi.editStatus(
                    statusToSend.statusId!!,
                    "Bearer " + account.accessToken,
                    account.domain,
                    statusToSend.idempotencyKey,
                    newStatus
                )
            }

            sendResult.fold({ sentStatus ->
                statusesToSend.remove(statusId)
                // Warpdroid has no drafts table, so nothing to delete here.

                mediaUploader.cancelUploadScope(*statusToSend.media.map { it.localId }.toIntArray())

                if (scheduled) {
                    eventHub.dispatch(TweetScheduledEvent((sentStatus as ScheduledTweetReply).id))
                } else if (!isNew) {
                    eventHub.dispatch(TweetChangedEvent(sentStatus as Tweet))
                } else {
                    eventHub.dispatch(TweetComposedEvent(sentStatus as Tweet))
                }

                notificationManager.cancel(statusId)
            }, { throwable ->
                Log.w(TAG, "failed sending status", throwable)
                failOrRetry(throwable, statusId)
            })
            stopSelfWhenDone()
        }
    }

    /**
     * Retry any send failure with linear backoff up to MAX_SEND_RETRIES,
     * then surface a user-visible error notification.
     *
     * Warpnet never throws HttpException (no HTTP), so the original
     * Tusky branch on HttpException always fell through to retrySending
     * which busy-looped silently. Replace it with a bounded retry that
     * has to either land or fail loudly - "post or throw", no third
     * option.
     */
    private suspend fun failOrRetry(throwable: Throwable, statusId: Int) {
        val statusToSend = statusesToSend[statusId] ?: return
        if (statusToSend.retries >= MAX_SEND_RETRIES) {
            Log.w(TAG, "giving up on status $statusId after ${statusToSend.retries} attempts", throwable)
            failSending(statusId)
            return
        }
        retrySending(statusId)
    }

    private suspend fun retrySending(statusId: Int) {
        // when statusToSend == null, sending has been canceled
        val statusToSend = statusesToSend[statusId] ?: return

        val backoff = TimeUnit.SECONDS.toMillis(
            statusToSend.retries.toLong()
        ).coerceAtMost(MAX_RETRY_INTERVAL)

        delay(backoff)
        sendStatus(statusId)
    }

    private fun stopSelfWhenDone() {
        if (statusesToSend.isEmpty()) {
            ServiceCompat.stopForeground(
                this@SendTweetService,
                ServiceCompat.STOP_FOREGROUND_REMOVE
            )
            stopSelf()
        }
    }

    private suspend fun failSending(statusId: Int) {
        val failedStatus = statusesToSend.remove(statusId)
        if (failedStatus != null) {
            mediaUploader.cancelUploadScope(*failedStatus.media.map { it.localId }.toIntArray())

            saveStatusToDrafts(failedStatus, failedToSendAlert = true)

            val notification = buildDraftNotification(
                R.string.send_post_notification_error_title,
                R.string.send_post_notification_saved_content,
                failedStatus.accountId,
                statusId
            )

            notificationManager.cancel(statusId)
            notificationManager.notify(errorNotificationId++, notification)
        }

        // NOTE only this removes the "Sending..." notification (added with startForeground() above)
        stopSelfWhenDone()
    }

    private fun cancelSending(statusId: Int) = serviceScope.launch {
        val statusToCancel = statusesToSend.remove(statusId)
        if (statusToCancel != null) {
            mediaUploader.cancelUploadScope(*statusToCancel.media.map { it.localId }.toIntArray())

            val sendJob = sendJobs.remove(statusId)
            sendJob?.cancel()

            saveStatusToDrafts(statusToCancel, failedToSendAlert = false)

            val notification = buildDraftNotification(
                R.string.send_post_notification_cancel_title,
                R.string.send_post_notification_saved_content,
                statusToCancel.accountId,
                statusId
            )

            notificationManager.notify(statusId, notification)

            delay(5000)

            stopSelfWhenDone()
        }
    }

    @Suppress("UNUSED_PARAMETER")
    private suspend fun saveStatusToDrafts(status: TweetToSend, failedToSendAlert: Boolean) {
        // TODO(warpdroid): drafts persistence was removed with Room; no-op.
    }

    private fun cancelSendingIntent(statusId: Int): PendingIntent {
        val intent = Intent(this, SendTweetService::class.java)
        intent.putExtra(KEY_CANCEL, statusId)
        return PendingIntent.getService(
            this,
            statusId,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
    }

    private fun buildDraftNotification(
        @StringRes title: Int,
        @StringRes content: Int,
        accountId: Long,
        statusId: Int
    ): Notification {
        val intent = MainActivity.draftIntent(this, accountId)

        val pendingIntent = PendingIntent.getActivity(
            this,
            statusId,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )

        return NotificationCompat.Builder(this@SendTweetService, CHANNEL_ID)
            .setSmallIcon(R.drawable.warpdroid_notification_icon)
            .setContentTitle(getString(title))
            .setContentText(getString(content))
            .setColor(getColor(R.color.notification_color))
            .setAutoCancel(true)
            .setOngoing(false)
            .setContentIntent(pendingIntent)
            .build()
    }

    override fun onDestroy() {
        super.onDestroy()
        supervisorJob.cancel()
    }

    companion object {
        private const val TAG = "SendTweetService"

        private const val KEY_STATUS = "status"
        private const val KEY_CANCEL = "cancel_id"
        private const val CHANNEL_ID = "send_toots"

        private val MAX_RETRY_INTERVAL = TimeUnit.MINUTES.toMillis(1)
        private const val MAX_SEND_RETRIES = 5

        private var sendingNotificationId = -1 // use negative ids to not clash with other notis
        private var errorNotificationId = Int.MIN_VALUE // use even more negative ids to not clash with other notis

        fun sendTweetIntent(context: Context, statusToSend: TweetToSend): Intent {
            val intent = Intent(context, SendTweetService::class.java)
            intent.putExtra(KEY_STATUS, statusToSend)

            if (statusToSend.media.isNotEmpty()) {
                // forward uri permissions
                intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                val uriClip = ClipData(
                    ClipDescription("Tweet Media", arrayOf("image/*", "video/*")),
                    ClipData.Item(statusToSend.media[0].uri)
                )
                statusToSend.media
                    .drop(1)
                    .forEach { mediaItem ->
                        uriClip.addItem(ClipData.Item(mediaItem.uri))
                    }

                intent.clipData = uriClip
            }

            return intent
        }
    }
}

@Parcelize
data class TweetToSend(
    val text: String,
    val warningText: String,
    val visibility: String,
    val sensitive: Boolean,
    val media: List<MediaToSend>,
    val scheduledAt: String?,
    val inReplyToId: String?,
    val poll: NewPoll?,
    val replyingTweetContent: String?,
    val replyingStatusAuthorUsername: String?,
    val accountId: Long,
    val draftId: Int,
    val idempotencyKey: String,
    var retries: Int,
    val language: String?,
    val statusId: String?
) : Parcelable

@Parcelize
data class MediaToSend(
    val localId: Int,
    // null if media is not yet completely uploaded
    val id: String?,
    val uri: String,
    val description: String?,
    val focus: Attachment.Focus?,
    var processed: Boolean
) : Parcelable
