/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package com.keylesspalace.tusky.db

import com.keylesspalace.tusky.db.entity.AccountEntity
import com.keylesspalace.tusky.entity.Account
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.SharingStarted

/**
 * In-memory account store for Warpdroid. There is no login flow: the single
 * stub account is created at process start and lives until the process dies.
 *
 * Mutation surface is preserved so call sites keep compiling, but state only
 * exists in [accountsFlow] — nothing hits disk. Phase 4 wires the real peer
 * identity in here once [com.keylesspalace.tusky.db.entity.AccountEntity] is
 * populated from the Warpnet `getOwner` event.
 */
@Singleton
class AccountManager @Inject constructor() {

    private val _accountsFlow = MutableStateFlow(listOf(STUB_ACCOUNT))

    val accountsFlow: StateFlow<List<AccountEntity>> = _accountsFlow.asStateFlow()

    val accounts: List<AccountEntity>
        get() = _accountsFlow.value

    val activeAccount: AccountEntity?
        get() = accounts.firstOrNull { it.isActive }

    fun activeAccount(scope: CoroutineScope): StateFlow<AccountEntity?> {
        val current = activeAccount
        return accountsFlow
            .map { list -> list.find { it.id == current?.id } }
            .stateIn(scope, SharingStarted.Lazily, current)
    }

    /** @return true — Warpdroid is always "signed in" to the stub account. */
    fun hasActiveAccount(): Boolean = activeAccount != null

    /** Mutates the active account in place. */
    fun updateActiveAccount(changer: AccountEntity.() -> AccountEntity) {
        _accountsFlow.value = accounts.map { if (it.isActive) changer(it) else it }
    }

    suspend fun updateAccount(account: AccountEntity, changer: AccountEntity.() -> AccountEntity) {
        _accountsFlow.value = accounts.map { if (it.id == account.id) changer(it) else it }
    }

    /**
     * Mirror-update the stub [AccountEntity] from a Warpnet-sourced [Account]
     * DTO (populated in Phase 4 by the Mastodon→Warpnet mapper).
     */
    suspend fun updateAccount(accountEntity: AccountEntity, account: Account) {
        updateAccount(accountEntity) {
            copy(
                accountId = account.id,
                username = account.username,
                displayName = account.name,
                profilePictureUrl = account.avatar,
                profileHeaderUrl = account.header,
                emojis = account.emojis,
                locked = account.locked,
            )
        }
    }

    fun getAccountById(accountId: Long): AccountEntity? =
        accounts.find { it.id == accountId }

    fun getAccountByIdentifier(identifier: String): AccountEntity? =
        accounts.find { it.identifier == identifier }

    /** No-op: single-account model. Preserved for call-site compatibility. */
    suspend fun setActiveAccount(accountId: Long) {
        // Warpdroid only ever has the stub account active.
    }

    /**
     * No-op: there is nothing to remove. Returns null to signal "no other
     * account available" so the caller's "last account was logged out" path
     * runs — except the caller is dead code in Warpdroid.
     */
    suspend fun remove(account: AccountEntity): AccountEntity? = null

    /** @return true — at least the stub account has notifications enabled. */
    fun areNotificationsEnabled(): Boolean = accounts.any { it.notificationsEnabled }

    /** Single-account UI never disambiguates by username. */
    fun shouldDisplaySelfUsername(): Boolean = false

    companion object {
        const val STUB_ACCOUNT_ID: Long = 1L
        const val STUB_DOMAIN: String = "warpnet.local"
        const val STUB_USERNAME: String = "me"

        private val STUB_ACCOUNT = AccountEntity(
            id = STUB_ACCOUNT_ID,
            domain = STUB_DOMAIN,
            accessToken = "",
            clientId = null,
            clientSecret = null,
            isActive = true,
            accountId = STUB_USERNAME,
            username = STUB_USERNAME,
            displayName = STUB_USERNAME,
        )
    }
}
