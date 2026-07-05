package site.warpnet.warpdroid.components.systemnotifications

import android.Manifest
import android.app.Activity
import android.app.NotificationChannel
import android.app.NotificationChannelGroup
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.graphics.BitmapFactory
import android.net.ConnectivityManager
import android.net.Network
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.service.notification.StatusBarNotification
import androidx.annotation.StringRes
import androidx.annotation.VisibleForTesting
import androidx.core.app.ActivityCompat
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.app.NotificationManagerCompat.NotificationWithIdAndTag
import androidx.core.app.RemoteInput
import androidx.core.app.TaskStackBuilder
import androidx.core.content.ContextCompat
import androidx.core.net.toUri
import androidx.work.Constraints
import androidx.work.Data
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequest
import androidx.work.OutOfQuotaPolicy
import androidx.work.WorkManager
import at.connyduck.calladapter.networkresult.fold
import at.connyduck.calladapter.networkresult.onFailure
import at.connyduck.calladapter.networkresult.onSuccess
import com.bumptech.glide.Glide
import com.bumptech.glide.load.resource.bitmap.RoundedCorners
import site.warpnet.warpdroid.BuildConfig
import site.warpnet.warpdroid.MainActivity
import site.warpnet.warpdroid.MainActivity.Companion.composeIntent
import site.warpnet.warpdroid.MainActivity.Companion.openNotificationIntent
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.compose.ComposeActivity
import site.warpnet.warpdroid.components.compose.ComposeActivity.ComposeOptions
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.di.ApplicationScope
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.RelationshipSeveranceEvent
import site.warpnet.warpdroid.entity.visibleNotificationTypes
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.receiver.SendTweetBroadcastReceiver
import site.warpnet.warpdroid.service.WarpnetNotificationService
import site.warpnet.warpdroid.util.parseAsWarpnetHtml
import site.warpnet.warpdroid.util.unicodeWrap
import site.warpnet.warpdroid.worker.NotificationWorker
import dagger.hilt.android.qualifiers.ApplicationContext
import java.text.NumberFormat
import java.util.concurrent.ExecutionException
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.CoroutineScope
import timber.log.Timber

@Singleton
class NotificationHelper @Inject constructor(
    private val notificationManager: NotificationManager,
    private val accountManager: AccountManager,
    private val api: WarpnetApi,
    @ApplicationContext private val context: Context,
    @ApplicationScope private val applicationScope: CoroutineScope
) {
    private var workManager: WorkManager = WorkManager.getInstance(context)

    private var notificationId: Int = NOTIFICATION_ID_PRUNE_CACHE + 1

    private var chargingReceiver: BroadcastReceiver? = null
    private var networkCallback: ConnectivityManager.NetworkCallback? = null

    init {
        createWorkerNotificationChannel()
        createSyncServiceNotificationChannel()
    }

    fun areNotificationsEnabledBySystem(): Boolean {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            // on Android >= O, notifications are enabled, if at least one channel is enabled

            if (notificationManager.areNotificationsEnabled()) {
                for (channel in notificationManager.notificationChannels) {
                    if (channel != null && channel.importance > NotificationManager.IMPORTANCE_NONE) {
                        Timber.tag(TAG).d("Notifications enabled for app by the system.")
                        return true
                    }
                }
            }
            Timber.tag(TAG).d("Notifications disabled for app by the system.")

            return false
        } else {
            // on Android < O, notifications are enabled, if at least one account has notification enabled
            return accountManager.areNotificationsEnabled()
        }
    }

    suspend fun setupNotifications(activity: Activity) {
        // No FCM; delivered by the always-on push service (Briar-style).
        enablePullNotifications()
    }

    fun enablePullNotifications() {
        // The foreground service replaces the old periodic poll; drop any
        // leftover periodic work from a previous version.
        workManager.cancelUniqueWork(NOTIFICATION_PULL_NAME)
        runCatching { WarpnetNotificationService.start(context) }
            .onFailure { Timber.tag(TAG).w(it, "could not start push service") }
    }

    fun createNotificationChannelsForAccount(account: AccountEntity) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channelGroup = NotificationChannelGroup(account.identifier, account.fullName)
            notificationManager.createNotificationChannelGroup(channelGroup)

            val channels = NotificationChannelData.entries.map {
                NotificationChannel(
                    it.getChannelId(account),
                    context.getString(it.title),
                    NotificationManager.IMPORTANCE_DEFAULT
                ).apply {
                    description = context.getString(it.description)
                    enableLights(true)
                    lightColor = -0xd46f27
                    enableVibration(true)
                    setShowBadge(true)
                    group = account.identifier
                }
            }

            notificationManager.createNotificationChannels(channels)
        }
    }

    private fun deleteNotificationChannelsForAccount(account: AccountEntity) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            notificationManager.deleteNotificationChannelGroup(account.identifier)
        }
    }

    fun fetchNotificationsNow() = enqueueOneTimeWorker(null)

    fun startOpportunisticRefresh() {
        if (chargingReceiver == null) {
            val receiver = object : BroadcastReceiver() {
                override fun onReceive(context: Context?, intent: Intent?) {
                    if (intent?.action == Intent.ACTION_POWER_CONNECTED) {
                        fetchNotificationsNow()
                    }
                }
            }
            ContextCompat.registerReceiver(
                context,
                receiver,
                IntentFilter(Intent.ACTION_POWER_CONNECTED),
                ContextCompat.RECEIVER_NOT_EXPORTED,
            )
            chargingReceiver = receiver
        }
        if (networkCallback == null) {
            val connectivityManager =
                context.getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager
            if (connectivityManager != null) {
                val hadNetwork = runCatching { connectivityManager.activeNetwork != null }.getOrDefault(false)
                val callback = object : ConnectivityManager.NetworkCallback() {
                    private var skipInitialCallback = hadNetwork
                    override fun onAvailable(network: Network) {
                        if (skipInitialCallback) {
                            skipInitialCallback = false
                            return
                        }
                        fetchNotificationsNow()
                    }
                }
                val registered = runCatching {
                    connectivityManager.registerDefaultNetworkCallback(callback)
                }.isSuccess
                if (registered) {
                    networkCallback = callback
                }
            }
        }
    }

    fun stopOpportunisticRefresh() {
        chargingReceiver?.let { receiver ->
            runCatching { context.unregisterReceiver(receiver) }
            chargingReceiver = null
        }
        networkCallback?.let { callback ->
            val connectivityManager =
                context.getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager
            runCatching { connectivityManager?.unregisterNetworkCallback(callback) }
            networkCallback = null
        }
    }

    private fun enqueueOneTimeWorker(account: AccountEntity?) {
        val oneTimeRequestBuilder = OneTimeWorkRequest.Builder(NotificationWorker::class.java)
            .setExpedited(OutOfQuotaPolicy.DROP_WORK_REQUEST)
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())

        account?.let {
            val data = Data.Builder()
            data.putLong(NotificationWorker.KEY_ACCOUNT_ID, account.id)
            oneTimeRequestBuilder.setInputData(data.build())
        }

        workManager.enqueueUniqueWork(
            NOTIFICATION_PULL_ONESHOT_NAME,
            ExistingWorkPolicy.KEEP,
            oneTimeRequestBuilder.build(),
        )
    }

    fun disablePullNotifications() {
        workManager.cancelUniqueWork(NOTIFICATION_PULL_NAME)
        WarpnetNotificationService.stop(context)
        Timber.tag(TAG).d("Disabled pull checks.")
    }

    fun clearNotificationsForAccount(account: AccountEntity) {
        for (androidNotification in notificationManager.activeNotifications) {
            if (account.id.toInt() == androidNotification.id) {
                notificationManager.cancel(androidNotification.tag, androidNotification.id)
            }
        }
    }

    fun filterNotification(account: AccountEntity, type: Notification.Type): Boolean {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channelId = getChannelId(account, type)
                ?: // unknown notificationtype
                return false
            val channel = notificationManager.getNotificationChannel(channelId)

            return channel != null && channel.importance > NotificationManager.IMPORTANCE_NONE
        }

        return when (type) {
            Notification.Type.Mention -> account.notificationsMentioned
            Notification.Type.Status -> account.notificationsSubscriptions
            Notification.Type.Follow -> account.notificationsFollowed
            Notification.Type.FollowRequest -> account.notificationsFollowRequested
            Notification.Type.Retweet -> account.notificationsRetweeted
            Notification.Type.PleromaEmojiReaction,
            Notification.Type.Like -> account.notificationsLiked
            Notification.Type.SignUp -> account.notificationsAdmin
            Notification.Type.Update -> account.notificationsUpdates
            else -> account.notificationsOther
        }
    }

    fun show(account: AccountEntity, notifications: List<Notification>) {
        if (ActivityCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            return
        }

        if (notifications.isEmpty()) {
            return
        }

        val newNotifications = ArrayList<NotificationWithIdAndTag>()

        val notificationsByType: Map<Notification.Type, List<Notification>> = notifications.groupBy { it.type }
        for ((type, notificationsForOneType) in notificationsByType) {
            val summary = createSummaryNotification(account, type, notificationsForOneType) ?: continue

            // NOTE Enqueue the summary first: Needed to avoid rate limit problems:
            //   ie. single notification is enqueued but later the summary one is filtered and thus no grouping
            //   takes place.
            newNotifications.add(summary)

            for (notification in notificationsForOneType) {
                val single = createNotification(notification, account) ?: continue
                newNotifications.add(single)
            }
        }

        val notificationManagerCompat = NotificationManagerCompat.from(context)
        // NOTE having multiple summary notifications: this here should still collapse them in only one occurrence
        notificationManagerCompat.notify(newNotifications)
    }

    private fun createNotification(apiNotification: Notification, account: AccountEntity): NotificationWithIdAndTag? {
        val baseNotification = createBaseNotification(apiNotification, account) ?: return null

        return NotificationWithIdAndTag(
            apiNotification.id,
            account.id.toInt(),
            baseNotification
        )
    }

    @VisibleForTesting
    fun createBaseNotification(apiNotification: Notification, account: AccountEntity): android.app.Notification? {
        val channelId = getChannelId(account, apiNotification.type) ?: return null

        val body = apiNotification.rewriteToStatusTypeIfNeeded(account.accountId)

        // Check for an existing notification matching this account and api notification
        var existingAndroidNotification: android.app.Notification? = null
        val activeNotifications = notificationManager.activeNotifications
        for (androidNotification in activeNotifications) {
            if (body.id == androidNotification.tag && account.id.toInt() == androidNotification.id) {
                existingAndroidNotification = androidNotification.notification
            }
        }

        notificationId++

        val builder = if (existingAndroidNotification == null) {
            getNotificationBuilder(body, account, channelId)
        } else {
            NotificationCompat.Builder(context, existingAndroidNotification)
        }

        builder
            .setContentTitle(titleForType(body, account))
            .setContentText(bodyForType(body, account))

        if (body.type == Notification.Type.Mention) {
            builder.setStyle(
                NotificationCompat.BigTextStyle()
                    .bigText(bodyForType(body, account))
            )
        }

        if (body.type != Notification.Type.SeveredRelationship &&
            body.type != Notification.Type.ModerationWarning &&
            body.type != Notification.Type.Message
        ) {
            val accountAvatar = try {
                Glide.with(context)
                    .asBitmap()
                    .load(body.account.avatar)
                    .transform(RoundedCorners(20))
                    .submit()
                    .get()
            } catch (e: ExecutionException) {
                Timber.tag(TAG).d(e, "Error loading account avatar")
                BitmapFactory.decodeResource(context.resources, R.drawable.avatar_default)
            } catch (e: InterruptedException) {
                Timber.tag(TAG).d(e, "Error loading account avatar")
                BitmapFactory.decodeResource(context.resources, R.drawable.avatar_default)
            }

            builder.setLargeIcon(accountAvatar)
        }

        // Reply to mention action; RemoteInput is available from KitKat Watch, but buttons are available from Nougat
        if (body.type == Notification.Type.Mention) {
            val replyRemoteInput = RemoteInput.Builder(KEY_REPLY)
                .setLabel(context.getString(R.string.label_quick_reply))
                .build()

            val quickReplyPendingIntent = getStatusReplyIntent(body, account, notificationId)

            val quickReplyAction =
                NotificationCompat.Action.Builder(
                    R.drawable.ic_reply_24dp,
                    context.getString(R.string.action_quick_reply),
                    quickReplyPendingIntent
                )
                    .addRemoteInput(replyRemoteInput)
                    .build()

            builder.addAction(quickReplyAction)

            val composeIntent = getStatusComposeIntent(body, account, notificationId)

            val composeAction =
                NotificationCompat.Action.Builder(
                    R.drawable.ic_reply_24dp,
                    context.getString(R.string.action_compose_shortcut),
                    composeIntent
                )
                    .setShowsUserInterface(true)
                    .build()

            builder.addAction(composeAction)
        }

        builder.addExtras(
            Bundle().apply {
                // Add the sending account's name, so it can be used also later when summarising this notification
                putString(EXTRA_ACCOUNT_NAME, body.account.name)
                putString(EXTRA_NOTIFICATION_TYPE, body.type.name)
            }
        )

        return builder.build()
    }

    /**
     * Create a notification that summarises the other notifications in this group.
     *
     * NOTE: We always create a summary notification (even for only one notification of that type):
     *   - No need to especially track the grouping
     *   - No need to change an existing single notification when there arrives another one of its group
     *   - Only the summary one will get announced
     */
    private fun createSummaryNotification(
        account: AccountEntity,
        type: Notification.Type,
        additionalNotifications: List<Notification>
    ): NotificationWithIdAndTag? {
        val typeChannelId = getChannelId(account, type) ?: return null

        val summaryStackBuilder = TaskStackBuilder.create(context)
        summaryStackBuilder.addParentStack(MainActivity::class.java)
        val summaryResultIntent = openNotificationIntent(context, account.id, type)
        summaryStackBuilder.addNextIntent(summaryResultIntent)

        val summaryResultPendingIntent = summaryStackBuilder.getPendingIntent(
            (notificationId + account.id * 10000).toInt(),
            pendingIntentFlags(false)
        )

        val activeNotifications = getActiveNotifications(account.id, typeChannelId)

        val notificationCount = activeNotifications.size + additionalNotifications.size

        val title = context.resources.getQuantityString(R.plurals.notification_title_summary, notificationCount, notificationCount)
        val text = joinNames(activeNotifications, additionalNotifications)

        val summaryBuilder = NotificationCompat.Builder(context, typeChannelId)
            .setSmallIcon(R.drawable.warpdroid_notification_icon)
            .setContentIntent(summaryResultPendingIntent)
            .setColor(context.getColor(R.color.notification_color))
            .setAutoCancel(true)
            .setContentTitle(title)
            .setContentText(text)
            .setShortcutId(account.id.toString())
            .setSubText(account.fullName)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setCategory(NotificationCompat.CATEGORY_SOCIAL)
            .setGroup(typeChannelId)
            .setGroupSummary(true)
            .setGroupAlertBehavior(NotificationCompat.GROUP_ALERT_SUMMARY)

        setSoundVibrationLight(account, summaryBuilder)

        val summaryTag = "$GROUP_SUMMARY_TAG.$typeChannelId"

        return NotificationWithIdAndTag(summaryTag, account.id.toInt(), summaryBuilder.build())
    }

    fun createWorkerNotification(@StringRes titleResource: Int): android.app.Notification {
        val title = context.getString(titleResource)
        return NotificationCompat.Builder(context, CHANNEL_BACKGROUND_TASKS)
            .setContentTitle(title)
            .setTicker(title)
            .setSmallIcon(R.drawable.warpdroid_notification_icon)
            .setOngoing(true)
            .build()
    }

    // Ongoing "Warpnet is running" notification for the push service.
    fun createSyncServiceNotification(): android.app.Notification {
        val contentIntent = PendingIntent.getActivity(
            context,
            0,
            Intent(context, MainActivity::class.java),
            pendingIntentFlags(false),
        )
        return NotificationCompat.Builder(context, CHANNEL_SYNC_SERVICE)
            .setContentTitle(context.getString(R.string.notification_sync_service_title))
            .setContentText(context.getString(R.string.notification_sync_service_text))
            .setSmallIcon(R.drawable.warpdroid_notification_icon)
            .setContentIntent(contentIntent)
            .setColor(context.getColor(R.color.notification_color))
            .setOngoing(true)
            .setShowWhen(false)
            .setVisibility(NotificationCompat.VISIBILITY_SECRET)
            .build()
    }

    private fun getChannelId(account: AccountEntity, type: Notification.Type): String? {
        return NotificationChannelData.entries.find { data ->
            data.notificationTypes.contains(type)
        }?.getChannelId(account)
    }

    /**
     * Return all active notifications, ignoring notifications that:
     * - belong to a different account
     * - belong to a different type
     * - are summary notifications
     */
    private fun getActiveNotifications(accountId: Long, typeChannelId: String): List<StatusBarNotification> {
        return notificationManager.activeNotifications.filter {
            val channelId = it.notification.group
            it.id == accountId.toInt() && channelId == typeChannelId && it.tag != "$GROUP_SUMMARY_TAG.$channelId"
        }
    }

    private fun getNotificationBuilder(notification: Notification, account: AccountEntity, channelId: String): NotificationCompat.Builder {
        val notificationType = notification.type
        // Warpnet moderation verdicts arrive as ModerationWarning without the
        // Mastodon AccountWarning payload — fall through to the generic intent.
        val warning = if (notificationType == Notification.Type.ModerationWarning) notification.moderationWarning else null
        val eventResultPendingIntent = if (warning != null) {
            val intent = Intent(Intent.ACTION_VIEW, "https://${account.domain}/disputes/strikes/${warning.id}".toUri())
            PendingIntent.getActivity(context, account.id.toInt(), intent, pendingIntentFlags(false))
        } else {
            val eventResultIntent = openNotificationIntent(context, account.id, notificationType)

            val eventStackBuilder = TaskStackBuilder.create(context)
            eventStackBuilder.addParentStack(MainActivity::class.java)
            eventStackBuilder.addNextIntent(eventResultIntent)

            eventStackBuilder.getPendingIntent(
                account.id.toInt(),
                pendingIntentFlags(false)
            )
        }

        val builder = NotificationCompat.Builder(context, channelId)
            .setSmallIcon(R.drawable.warpdroid_notification_icon)
            .setContentIntent(eventResultPendingIntent)
            .setColor(context.getColor(R.color.notification_color))
            .setAutoCancel(true)
            .setShortcutId(account.id.toString())
            .setSubText(account.fullName)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setCategory(NotificationCompat.CATEGORY_SOCIAL)
            .setOnlyAlertOnce(true)
            .setGroup(channelId)
            .setGroupAlertBehavior(NotificationCompat.GROUP_ALERT_SUMMARY) // Only ever alert for the summary notification

        setSoundVibrationLight(account, builder)

        return builder
    }

    private fun titleForType(notification: Notification, account: AccountEntity): String? {
        val accountName = notification.account.name.unicodeWrap()
        return when (notification.type) {
            Notification.Type.Mention -> context.getString(R.string.notification_mention_format, accountName)
            Notification.Type.Reply -> context.getString(R.string.notification_mention_format, accountName)
            // The fat node pre-composes "<sender> sent you a message" into the
            // notification text, surfaced by the mapper as account.name.
            Notification.Type.Message -> accountName
            Notification.Type.Status -> context.getString(R.string.notification_subscription_format, accountName)
            Notification.Type.Follow -> context.getString(R.string.notification_follow_format, accountName)
            Notification.Type.FollowRequest -> context.getString(R.string.notification_follow_request_format, accountName)
            Notification.Type.Like -> context.getString(R.string.notification_like_format, accountName)
            Notification.Type.PleromaEmojiReaction -> context.getString(R.string.notification_pleroma_reaction_format, accountName)
            Notification.Type.Retweet -> context.getString(R.string.notification_retweet_format, accountName)
            Notification.Type.SignUp -> context.getString(R.string.notification_sign_up_format, accountName)
            Notification.Type.Update -> context.getString(R.string.notification_update_format, accountName)
            Notification.Type.SeveredRelationship -> context.getString(R.string.relationship_severance_event_title)
            Notification.Type.ModerationWarning -> context.getString(R.string.moderation_warning)
            Notification.Type.Quote -> context.getString(R.string.notification_quote_format, accountName)
            Notification.Type.QuotedUpdate -> context.getString(R.string.notification_quoted_update_format, accountName)
            is Notification.Type.Unknown -> null
        }
    }

    private fun bodyForType(notification: Notification, account: AccountEntity): String? {
        val alwaysOpenSpoiler = account.alwaysOpenSpoiler

        when (notification.type) {
            Notification.Type.Follow,
            Notification.Type.FollowRequest,
            Notification.Type.SignUp -> return "@" + notification.account.username
            Notification.Type.Mention,
            Notification.Type.Like,
            Notification.Type.Retweet,
            Notification.Type.Status -> return if (!notification.status?.spoilerText.isNullOrEmpty() && !alwaysOpenSpoiler) {
                notification.status.spoilerText
            } else {
                notification.status?.content?.parseAsWarpnetHtml()?.toString()
            }
            Notification.Type.SeveredRelationship -> return severedRelationShipText(context, notification.event!!, account.domain)
            Notification.Type.ModerationWarning -> {
                // Warpnet verdicts carry the whole message in the pre-composed
                // text (surfaced as account.name by the mapper).
                val warning = notification.moderationWarning ?: return notification.account.name
                return context.getString(warning.action.text)
            }
            else -> return null
        }
    }

    private fun createWorkerNotificationChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
            return
        }

        val channel = NotificationChannel(
            CHANNEL_BACKGROUND_TASKS,
            context.getString(R.string.notification_listenable_worker_name),
            NotificationManager.IMPORTANCE_NONE
        )

        channel.description = context.getString(R.string.notification_listenable_worker_description)
        channel.enableLights(false)
        channel.enableVibration(false)
        channel.setShowBadge(false)

        notificationManager.createNotificationChannel(channel)
    }

    private fun createSyncServiceNotificationChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
            return
        }

        val channel = NotificationChannel(
            CHANNEL_SYNC_SERVICE,
            context.getString(R.string.notification_sync_service_channel_name),
            NotificationManager.IMPORTANCE_LOW,
        )
        channel.description = context.getString(R.string.notification_sync_service_channel_description)
        channel.enableLights(false)
        channel.enableVibration(false)
        channel.setShowBadge(false)

        notificationManager.createNotificationChannel(channel)
    }

    private fun setSoundVibrationLight(account: AccountEntity, builder: NotificationCompat.Builder) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            return // Do nothing on Android O or newer, the system uses only the channel settings
        }

        builder.setDefaults(0)

        if (account.notificationSound) {
            builder.setSound(Settings.System.DEFAULT_NOTIFICATION_URI)
        }

        if (account.notificationVibration) {
            builder.setVibrate(longArrayOf(500, 500))
        }

        if (account.notificationLight) {
            builder.setLights(-0xd46f27, 300, 1000)
        }
    }

    private fun joinNames(notifications1: List<StatusBarNotification>, notifications2: List<Notification>): String? {
        val names = java.util.ArrayList<String>(notifications1.size + notifications2.size)

        for (notification in notifications1) {
            val author = notification.notification.extras.getString(EXTRA_ACCOUNT_NAME) ?: continue
            names.add(author)
        }

        for (noti in notifications2) {
            names.add(noti.account.name)
        }

        if (names.size > 3) {
            val length = names.size
            return context.getString(
                R.string.notification_summary_large,
                names[length - 1].unicodeWrap(),
                names[length - 2].unicodeWrap(),
                names[length - 3].unicodeWrap(),
                length - 3
            )
        } else if (names.size == 3) {
            return context.getString(
                R.string.notification_summary_medium,
                names[2].unicodeWrap(),
                names[1].unicodeWrap(),
                names[0].unicodeWrap()
            )
        } else if (names.size == 2) {
            return context.getString(
                R.string.notification_summary_small,
                names[1].unicodeWrap(),
                names[0].unicodeWrap()
            )
        }

        return null
    }

    private fun getStatusReplyIntent(apiNotification: Notification, account: AccountEntity, requestCode: Int): PendingIntent {
        val status = checkNotNull(apiNotification.status)

        val inReplyToId = status.id
        val actionableStatus = status.actionableStatus
        val replyVisibility = actionableStatus.visibility
        val contentWarning = actionableStatus.spoilerText
        val mentions = actionableStatus.mentions

        val mentionedUsernames = buildSet {
            add(actionableStatus.account.username)
            for (mention in mentions) {
                add(mention.username)
            }
            remove(account.username)
        }

        val replyIntent = Intent(context, SendTweetBroadcastReceiver::class.java)
            .setAction(REPLY_ACTION)
            .putExtra(KEY_SENDER_ACCOUNT_ID, account.id)
            .putExtra(KEY_SENDER_ACCOUNT_IDENTIFIER, account.identifier)
            .putExtra(KEY_SENDER_ACCOUNT_FULL_NAME, account.fullName)
            .putExtra(KEY_SERVER_NOTIFICATION_ID, apiNotification.id)
            .putExtra(KEY_CITED_STATUS_ID, inReplyToId)
            .putExtra(KEY_VISIBILITY, replyVisibility)
            .putExtra(KEY_SPOILER, contentWarning)
            .putExtra(KEY_MENTIONS, mentionedUsernames.toTypedArray())

        return PendingIntent.getBroadcast(
            context.applicationContext,
            requestCode,
            replyIntent,
            pendingIntentFlags(true)
        )
    }

    private fun getStatusComposeIntent(apiNotification: Notification, account: AccountEntity, requestCode: Int): PendingIntent {
        val status = checkNotNull(apiNotification.status)

        val citedLocalAuthor = status.account.localUsername
        val citedText = status.content.parseAsWarpnetHtml().toString()
        val inReplyToId = status.id
        val actionableStatus = status.actionableStatus
        val replyVisibility = actionableStatus.visibility
        val contentWarning = actionableStatus.spoilerText
        val mentions = actionableStatus.mentions

        val mentionedUsernames = buildSet {
            add(actionableStatus.account.username)
            for (mention in mentions) {
                add(mention.username)
            }
            remove(account.username)
        }

        val composeOptions = ComposeOptions(
            inReplyToId = inReplyToId,
            replyVisibility = replyVisibility,
            contentWarning = contentWarning,
            replyingStatusAuthor = citedLocalAuthor,
            replyingTweetContent = citedText,
            mentionedUsernames = mentionedUsernames,
            modifiedInitialState = true,
            language = actionableStatus.language,
            kind = ComposeActivity.ComposeKind.NEW
        )

        val composeIntent = composeIntent(context, composeOptions, account.id, apiNotification.id, account.id.toInt())

        // make sure a new instance of MainActivity is started and old ones get destroyed
        composeIntent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)

        return PendingIntent.getActivity(
            context.applicationContext,
            requestCode,
            composeIntent,
            pendingIntentFlags(false)
        )
    }

    private fun pendingIntentFlags(mutable: Boolean): Int {
        return if (mutable) {
            PendingIntent.FLAG_UPDATE_CURRENT or (if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) PendingIntent.FLAG_MUTABLE else 0)
        } else {
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        }
    }

    suspend fun disableAllNotifications() {
        disablePullNotifications()
    }

    suspend fun disableNotificationsForAccount(account: AccountEntity) {
        deleteNotificationChannelsForAccount(account)

        if (!areNotificationsEnabledBySystem()) {
            // TODO this is sort of a hack, it means: are there now no active accounts?

            disablePullNotifications()
        }
    }

    companion object {
        const val TAG = "NotificationHelper"

        const val KEY_CITED_STATUS_ID: String = "KEY_CITED_STATUS_ID"
        const val KEY_MENTIONS: String = "KEY_MENTIONS"
        const val KEY_REPLY: String = "KEY_REPLY"
        const val KEY_SENDER_ACCOUNT_FULL_NAME: String = "KEY_SENDER_ACCOUNT_FULL_NAME"
        const val KEY_SENDER_ACCOUNT_ID: String = "KEY_SENDER_ACCOUNT_ID"
        const val KEY_SENDER_ACCOUNT_IDENTIFIER: String = "KEY_SENDER_ACCOUNT_IDENTIFIER"
        const val KEY_SERVER_NOTIFICATION_ID: String = "KEY_SERVER_NOTIFICATION_ID"
        const val KEY_SPOILER: String = "KEY_SPOILER"
        const val KEY_VISIBILITY: String = "KEY_VISIBILITY"
        const val NOTIFICATION_ID_FETCH_NOTIFICATION: Int = 0
        const val NOTIFICATION_ID_PRUNE_CACHE: Int = 1
        const val NOTIFICATION_ID_SYNC_SERVICE: Int = 2
        const val REPLY_ACTION: String = "REPLY_ACTION"

        private const val CHANNEL_BACKGROUND_TASKS: String = "CHANNEL_BACKGROUND_TASKS"
        private const val CHANNEL_SYNC_SERVICE: String = "CHANNEL_SYNC_SERVICE"
        private const val EXTRA_ACCOUNT_NAME = BuildConfig.APPLICATION_ID + ".notification.extra.account_name"
        private const val EXTRA_NOTIFICATION_TYPE = BuildConfig.APPLICATION_ID + ".notification.extra.notification_type"
        private const val GROUP_SUMMARY_TAG = BuildConfig.APPLICATION_ID + ".notification.group_summary"
        private const val NOTIFICATION_PULL_NAME = "pullNotifications"
        private const val NOTIFICATION_PULL_ONESHOT_NAME = "pullNotificationsNow"

        private val numberFormat = NumberFormat.getNumberInstance()

        fun severedRelationShipText(
            context: Context,
            event: RelationshipSeveranceEvent,
            instanceName: String
        ): String {
            return when (event.type) {
                RelationshipSeveranceEvent.Type.DOMAIN_BLOCK -> {
                    val followers = numberFormat.format(event.followersCount)
                    val following = numberFormat.format(event.followingCount)
                    val followingText = context.resources.getQuantityString(R.plurals.accounts, event.followingCount, following)
                    context.getString(R.string.relationship_severance_event_domain_block, instanceName, event.targetName, followers, followingText)
                }

                RelationshipSeveranceEvent.Type.USER_DOMAIN_BLOCK -> {
                    val followers = numberFormat.format(event.followersCount)
                    val following = numberFormat.format(event.followingCount)
                    val followingText = context.resources.getQuantityString(R.plurals.accounts, event.followingCount, following)
                    context.getString(R.string.relationship_severance_event_user_domain_block, event.targetName, followers, followingText)
                }

                RelationshipSeveranceEvent.Type.ACCOUNT_SUSPENSION -> {
                    context.getString(R.string.relationship_severance_event_account_suspension, instanceName, event.targetName)
                }
            }
        }
    }
}
