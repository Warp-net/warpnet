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

    private suspend fun fetchNewNotifications(account: AccountEntity): List<Notification> {
        val stored = account.notificationMarkerId
        val cursor = if (stored.isEmpty() || stored == "0") "" else stored

        val response = warpnetApi.notificationsPage(cursor = cursor)
        if (!response.isSuccessful) return emptyList()

        val notifications = response.body().orEmpty()

        val newCursor = Links.from(response.headers()["link"]).next
        if (!newCursor.isNullOrEmpty()) {
            accountManager.updateAccount(account) { copy(notificationMarkerId = newCursor) }
        }

        Log.d(TAG, "Got ${notifications.size} notifications for ${account.fullName}.")
        return notifications
    }

    companion object {
        private const val TAG = "NotificationFetcher"
    }
}
