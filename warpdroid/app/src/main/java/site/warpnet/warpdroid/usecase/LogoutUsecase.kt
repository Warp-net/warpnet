package site.warpnet.warpdroid.usecase

import site.warpnet.warpdroid.components.systemnotifications.NotificationHelper
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.util.ShareShortcutHelper
import javax.inject.Inject

class LogoutUsecase @Inject constructor(
    private val api: WarpnetApi,
    private val accountManager: AccountManager,
    private val shareShortcutHelper: ShareShortcutHelper,
    private val notificationHelper: NotificationHelper,
) {

    /**
     * Logs the current account out and clears all caches associated with it
     * @return true if the user is logged in with other accounts, false if it was the only one
     */
    suspend fun logout(account: AccountEntity): Boolean {
        // invalidate the oauth token, if we have the client id & secret
        // (could be missing if user logged in with a previous version of Warpdroid)
        val clientId = account.clientId
        val clientSecret = account.clientSecret
        if (clientId != null && clientSecret != null) {
            api.revokeOAuthToken(
                clientId = clientId,
                clientSecret = clientSecret,
                token = account.accessToken
            )
        }

        notificationHelper.disableNotificationsForAccount(account)

        // remove account from local AccountManager
        val otherAccountAvailable = accountManager.remove(account) != null

        // No local database or drafts table in Warpdroid — nothing to clean.

        // remove shortcut associated with the account
        shareShortcutHelper.removeShortcut(account)

        return otherAccountAvailable
    }
}
