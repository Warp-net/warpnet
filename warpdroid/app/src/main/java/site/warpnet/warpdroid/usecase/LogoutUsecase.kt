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
     * action needed. Single-account model, so there's no other account to
     * fall back to — the caller is expected to restart [MainActivity] into
     * a fresh stub account.
     */
    suspend fun logout(account: AccountEntity) {
        notificationHelper.disableNotificationsForAccount(account)
        shareShortcutHelper.removeShortcut(account)
    }
}
