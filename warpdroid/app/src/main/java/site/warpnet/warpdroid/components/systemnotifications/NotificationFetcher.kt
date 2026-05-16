package site.warpnet.warpdroid.components.systemnotifications

import android.util.Log
import androidx.annotation.WorkerThread
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.NewNotificationsEvent
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.util.HttpHeaderLink
import site.warpnet.warpdroid.util.isLessThan
import javax.inject.Inject

/** Models next/prev links from the "Links" header in an API response */
data class Links(val next: String?, val prev: String?) {
    companion object {
        fun from(linkHeader: String?): Links {
            val links = HttpHeaderLink.parse(linkHeader)
            return Links(
                next = HttpHeaderLink.findByRelationType(links, "next")?.uri?.getQueryParameter(
                    "max_id"
                ),
                prev = HttpHeaderLink.findByRelationType(links, "prev")?.uri?.getQueryParameter(
                    "min_id"
                )
            )
        }
    }
}

/**
 * Fetch Warpnet notifications and show Android notifications, with summaries, for them.
 *
 * Should only be called by a worker thread.
 *
 * @see site.warpnet.warpdroid.worker.NotificationWorker
 * @see <a href="https://developer.android.com/guide/background/persistent/threading/worker">Background worker</a>
 */
@WorkerThread
class NotificationFetcher @Inject constructor(
    private val warpnetApi: WarpnetApi,
    private val accountManager: AccountManager,
    private val eventHub: EventHub,
    private val notificationHelper: NotificationHelper,
) {
    suspend fun fetchAndShow(accountId: Long?) {
        for (account in accountManager.accounts) {
            if (accountId != null && account.id != accountId) {
                continue
            }

            if (account.notificationsEnabled) {
                try {
                    val notifications = fetchNewNotifications(account)
                        .filter { notificationHelper.filterNotification(account, it.type) }
                        .sortedWith(
                            compareBy({ it.id.length }, { it.id })
                        ) // oldest notifications first

                    eventHub.dispatch(NewNotificationsEvent(account.accountId, notifications))

                    notificationHelper.show(account, notifications)
                } catch (e: Exception) {
                    Log.e(TAG, "Error while fetching notifications", e)
                }
            }
        }
    }

    /**
     * Fetch new Warpnet Notifications and update the marker position.
     *
     * Here, "new" means "notifications with IDs newer than notifications the user has already
     * seen."
     *
     * The "water mark" for Warpnet Notification IDs are stored in three places.
     *
     * - acccount.lastNotificationId -- the ID of the top-most notification when the user last
     *   left the Notifications tab.
     * - The Warpnet "marker" API -- the ID of the most recent notification fetched here.
     * - account.notificationMarkerId -- local version of the value from the Warpnet marker
     *   API, in case the Warpnet server does not implement that API.
     *
     * The user may have refreshed the "Notifications" tab and seen notifications newer than the
     * ones that were last fetched here. So `lastNotificationId` takes precedence if it is greater
     * than the marker.
     */
    private suspend fun fetchNewNotifications(account: AccountEntity): List<Notification> {
        val authHeader = "Bearer ${account.accessToken}"

        // Pick where to read from. Warpnet has no server-side read-marker —
        // local "last seen" + the most recent visited id is the only source.
        val localMarkerId = account.notificationMarkerId
        val readingPosition = account.lastNotificationId

        var minId: String? = if (readingPosition.isLessThan(localMarkerId)) localMarkerId else readingPosition
        Log.d(TAG, "  localMarkerId: $localMarkerId")
        Log.d(TAG, "  readingPosition: $readingPosition")

        Log.d(TAG, "Getting Notifications for ${account.fullName}, min_id: $minId.")

        // Fetch all outstanding notifications
        val notifications: List<Notification> = buildList {
            while (minId != null) {
                val response = warpnetApi.notificationsWithAuth(
                    authHeader,
                    account.domain,
                    minId = minId
                )
                if (!response.isSuccessful) break

                // Notifications are returned in the page in order, newest first,
                // (https://github.com/mastodon/documentation/issues/1226), insert the
                // new page at the head of the list.
                response.body()?.let { addAll(0, it) }

                // Get the previous page, which will be chronologically newer
                // notifications. If it doesn't exist this is null and the loop
                // will exit.
                val links = Links.from(response.headers()["link"])
                minId = links.prev
            }
        }

        // Bump the local marker so we don't re-process the same notifications.
        notifications.firstOrNull()?.let {
            val newMarkerId = notifications.first().id
            accountManager.updateAccount(account) { copy(notificationMarkerId = newMarkerId) }
        }

        Log.d(TAG, "Got ${notifications.size} Notifications.")

        return notifications
    }

    companion object {
        private const val TAG = "NotificationFetcher"
    }
}
