package site.warpnet.warpdroid.usecase

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import site.warpnet.transport.WarpnetClient
import site.warpnet.warpdroid.components.pairing.PairedNodeStore
import site.warpnet.warpdroid.components.systemnotifications.NotificationHelper
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.util.ShareShortcutHelper
import javax.inject.Inject

class LogoutUsecase @Inject constructor(
    private val shareShortcutHelper: ShareShortcutHelper,
    private val notificationHelper: NotificationHelper,
    private val pairedNodeStore: PairedNodeStore,
    private val client: WarpnetClient,
) {

    /**
     * Logs the current account out and drops the pairing with it: the stored
     * QR payload goes, and so does the libp2p host built from it. Warpnet has
     * no server-side token to revoke — leaving the material on disk would
     * have logout restart straight back into the same paired session. The
     * node still honours this device's peer id until the pairing expires;
     * ending that early is the node owner's call, from Settings → Devices.
     *
     * Single-account model, so there's no other account to fall back to —
     * the caller is expected to restart [MainActivity], which finds no
     * pairing and lands on the QR scanner.
     */
    suspend fun logout(account: AccountEntity) {
        notificationHelper.clearNotificationsForAccount(account)
        notificationHelper.disableNotificationsForAccount(account)
        shareShortcutHelper.removeShortcut(account)

        withContext(Dispatchers.IO) { pairedNodeStore.clear() }
        client.shutdown()
    }
}
