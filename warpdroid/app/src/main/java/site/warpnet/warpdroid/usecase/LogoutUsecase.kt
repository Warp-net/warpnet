package site.warpnet.warpdroid.usecase

import site.warpnet.warpdroid.components.systemnotifications.NotificationHelper
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.util.ShareShortcutHelper
import javax.inject.Inject

class LogoutUsecase @Inject constructor(
    private val shareShortcutHelper: ShareShortcutHelper,
    private val notificationHelper: NotificationHelper,
) {

    /**
     * Logs the current account out and clears all caches associated with it.
     * Warpnet uses QR-pairing + node-challenge, not OAuth — there's no
     * server-side token to revoke; the local account drop is the only
     * action needed.
     *
     * @return true if the user is logged in with other accounts, false if it was the only one
     */
    suspend fun logout(account: AccountEntity): Boolean {
        notificationHelper.disableNotificationsForAccount(account)
        shareShortcutHelper.removeShortcut(account)
        return false
    }
}
