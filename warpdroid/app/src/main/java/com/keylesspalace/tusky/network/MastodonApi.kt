/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Drop-in stand-in for Tusky's Retrofit MastodonApi interface.
 *
 * Warpnet is a libp2p P2P network, not an HTTP Mastodon instance, so this
 * class is NOT backed by Retrofit. It keeps the same method signatures and
 * return types as the original so the ~50 view models / use cases that call
 * it keep compiling. Behaviour per method is one of two things:
 *
 *   - **Stubbed** — returns a predefined `NetworkResult.failure` or
 *     `Response.error(501, ...)`. Used for surfaces with no Warpnet
 *     equivalent (filters, lists, announcements, domain blocks, reports,
 *     scheduled statuses, trending, translate, polls, push subscriptions,
 *     custom emojis, OAuth, instance metadata, bookmarks, markers,
 *     conversations — the fediverse/instance-admin half of the Mastodon
 *     API). Chunk 1 stubs everything; subsequent chunks migrate tractable
 *     methods to [WarpnetRepository].
 *   - **Warpnet-backed** — delegates to [WarpnetRepository]. None in this
 *     chunk; chunks 2-5 wire these in one feature group at a time.
 */
package com.keylesspalace.tusky.network

import at.connyduck.calladapter.networkresult.NetworkResult
import com.keylesspalace.tusky.components.filters.FilterExpiration
import com.keylesspalace.tusky.db.AccountManager
import com.keylesspalace.tusky.entity.AccessToken
import com.keylesspalace.tusky.entity.Account
import com.keylesspalace.tusky.entity.Announcement
import com.keylesspalace.tusky.entity.AppCredentials
import com.keylesspalace.tusky.entity.Attachment
import com.keylesspalace.tusky.entity.Conversation
import com.keylesspalace.tusky.entity.DeletedStatus
import com.keylesspalace.tusky.entity.Emoji
import com.keylesspalace.tusky.entity.Filter
import com.keylesspalace.tusky.entity.FilterKeyword
import com.keylesspalace.tusky.entity.HashTag
import com.keylesspalace.tusky.entity.Instance
import com.keylesspalace.tusky.entity.InstanceConfiguration
import com.keylesspalace.tusky.entity.InstanceV1
import com.keylesspalace.tusky.entity.StatusConfiguration
import com.keylesspalace.tusky.entity.Marker
import com.keylesspalace.tusky.entity.MastoList
import com.keylesspalace.tusky.entity.MediaUploadResult
import com.keylesspalace.tusky.entity.NewStatus
import com.keylesspalace.tusky.entity.Notification
import com.keylesspalace.tusky.entity.NotificationPolicy
import com.keylesspalace.tusky.entity.NotificationRequest
import com.keylesspalace.tusky.entity.NotificationSubscribeResult
import com.keylesspalace.tusky.entity.Poll
import com.keylesspalace.tusky.entity.Relationship
import com.keylesspalace.tusky.entity.ScheduledStatus
import com.keylesspalace.tusky.entity.ScheduledStatusReply
import com.keylesspalace.tusky.entity.SearchResult
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.entity.StatusContext
import com.keylesspalace.tusky.entity.StatusEdit
import com.keylesspalace.tusky.entity.StatusSource
import com.keylesspalace.tusky.entity.TimelineAccount
import com.keylesspalace.tusky.entity.Translation
import com.keylesspalace.tusky.entity.TrendingTag
import com.keylesspalace.tusky.warpnet.WarpnetMapper
import com.keylesspalace.tusky.warpnet.WarpnetRepository
import java.util.Date
import javax.inject.Inject
import javax.inject.Singleton
import okhttp3.Headers
import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.MultipartBody
import okhttp3.RequestBody
import okhttp3.ResponseBody.Companion.toResponseBody
import retrofit2.Response

@Singleton
class MastodonApi @Inject constructor(
    @Suppress("unused") private val warpnet: WarpnetRepository,
    @Suppress("unused") private val accountManager: AccountManager,
) {

    companion object {
        const val ENDPOINT_AUTHORIZE = "oauth/authorize"
        const val DOMAIN_HEADER = "domain"
        const val PLACEHOLDER_DOMAIN = "dummy.placeholder"

        // Instance stub values — no Warpnet endpoint reports these, so we
        // hard-code the compose / onboarding UX against known node limits.
        private const val WARPNET_INSTANCE_VERSION = "0.0.0"
        private const val WARPNET_MAX_TWEET_CHARS = 2000

        private val STUB_BODY = "".toResponseBody("text/plain".toMediaTypeOrNull())
        private fun unsupported(name: String) =
            UnsupportedOperationException("MastodonApi.$name has no Warpnet equivalent")
        private fun <T> stubFailure(name: String): NetworkResult<T> =
            NetworkResult.failure(unsupported(name))
        private fun <T> stubError(): Response<T> = Response.error(501, STUB_BODY)
        private fun <T> stubList(): Response<List<T>> = Response.success(emptyList())
    }

    private suspend fun <T> response(block: suspend () -> T): Response<T> = try {
        Response.success(block())
    } catch (t: Throwable) {
        Response.error(
            500,
            (t.message ?: t.javaClass.simpleName).toResponseBody("text/plain".toMediaTypeOrNull()),
        )
    }

    /**
     * Wrap a Warpnet `(items, nextCursor)` page as a Retrofit [Response] and
     * synthesise a Mastodon-style `Link: <url?max_id=CURSOR>; rel="next"`
     * header when a follow-up cursor exists. NetworkTimelineRemoteMediator
     * (and peers) extract `max_id` from that header to drive pagination.
     */
    private suspend fun <T> paginated(block: suspend () -> Pair<List<T>, String>): Response<List<T>> = try {
        val (items, nextCursor) = block()
        val headers = if (nextCursor.isNotEmpty()) {
            Headers.headersOf(
                "Link",
                "<${WarpnetMapper.FAKE_BASE_URL}/?max_id=$nextCursor>; rel=\"next\"",
            )
        } else {
            Headers.headersOf()
        }
        Response.success(items, headers)
    } catch (t: Throwable) {
        Response.error(
            500,
            (t.message ?: t.javaClass.simpleName).toResponseBody("text/plain".toMediaTypeOrNull()),
        )
    }

    private suspend fun <T> result(block: suspend () -> T): NetworkResult<T> = try {
        NetworkResult.success(block())
    } catch (t: Throwable) {
        NetworkResult.failure(t)
    }

    // ---------------------------------------------------------------
    // instance metadata / custom emojis
    // ---------------------------------------------------------------

    suspend fun getCustomEmojis(): NetworkResult<List<Emoji>> = NetworkResult.success(emptyList())

    /**
     * Warpnet nodes expose [site.warpnet.transport.ProtocolIds.PUBLIC_GET_INFO]
     * which returns libp2p-peer-level metadata (peer id, protocols, start
     * time) — nothing that lines up with Mastodon's instance descriptor.
     * Return a static Warpnet-shaped stub so onboarding / compose / settings
     * screens have the fields they gate on.
     */
    suspend fun getInstanceV1(): NetworkResult<InstanceV1> = NetworkResult.success(
        InstanceV1(
            uri = PLACEHOLDER_DOMAIN,
            version = WARPNET_INSTANCE_VERSION,
            maxTootChars = WARPNET_MAX_TWEET_CHARS,
            maxMediaAttachments = 0,
            uploadLimit = 0,
            configuration = InstanceConfiguration(
                statuses = StatusConfiguration(
                    maxCharacters = WARPNET_MAX_TWEET_CHARS,
                    maxMediaAttachments = 0,
                    charactersReservedPerUrl = 23,
                ),
            ),
        ),
    )

    suspend fun getInstance(): NetworkResult<Instance> = NetworkResult.success(
        Instance(
            domain = PLACEHOLDER_DOMAIN,
            version = WARPNET_INSTANCE_VERSION,
            configuration = Instance.Configuration(
                statuses = Instance.Configuration.Statuses(
                    maxCharacters = WARPNET_MAX_TWEET_CHARS,
                    maxMediaAttachments = 0,
                    charactersReservedPerUrl = 23,
                ),
            ),
        ),
    )

    suspend fun getInstanceRules(domain: String? = null): NetworkResult<List<Instance.Rule>> =
        NetworkResult.success(emptyList())

    // ---------------------------------------------------------------
    // filters (no Warpnet equivalent — server-side filtering doesn't exist)
    // ---------------------------------------------------------------

    suspend fun getFilter(filterId: String): NetworkResult<Filter> = stubFailure("getFilter")
    suspend fun getFilters(): NetworkResult<List<Filter>> = NetworkResult.success(emptyList())
    suspend fun createFilter(
        title: String,
        context: List<Filter.Kind>,
        filterAction: Filter.Action,
        expiresIn: FilterExpiration?,
    ): NetworkResult<Filter> = stubFailure("createFilter")
    suspend fun updateFilter(
        id: String,
        title: String? = null,
        context: List<Filter.Kind>? = null,
        filterAction: Filter.Action? = null,
        expires: FilterExpiration? = null,
    ): NetworkResult<Filter> = stubFailure("updateFilter")
    suspend fun deleteFilter(id: String): NetworkResult<Unit> = NetworkResult.success(Unit)
    suspend fun addFilterKeyword(
        filterId: String,
        keyword: String,
        wholeWord: Boolean,
    ): NetworkResult<FilterKeyword> = stubFailure("addFilterKeyword")
    suspend fun updateFilterKeyword(
        keywordId: String,
        keyword: String,
        wholeWord: Boolean,
    ): NetworkResult<FilterKeyword> = stubFailure("updateFilterKeyword")
    suspend fun deleteFilterKeyword(keywordId: String): NetworkResult<Unit> = NetworkResult.success(Unit)

    // ---------------------------------------------------------------
    // timelines
    // ---------------------------------------------------------------

    suspend fun homeTimeline(
        maxId: String? = null,
        minId: String? = null,
        sinceId: String? = null,
        limit: Int? = null,
    ): Response<List<Status>> = paginated {
        warpnet.getHomeTimeline(cursor = maxId.orEmpty(), limit = limit ?: 40)
    }

    /**
     * Mastodon's public timeline has no direct Warpnet equivalent. Fall
     * back to the active account's own feed so the UI stays populated.
     */
    suspend fun publicTimeline(
        local: Boolean? = null,
        maxId: String? = null,
        minId: String? = null,
        sinceId: String? = null,
        limit: Int? = null,
    ): Response<List<Status>> {
        val userId = accountManager.activeAccount?.accountId.orEmpty()
        if (userId.isEmpty()) return stubList()
        return paginated {
            warpnet.getUserTimeline(userId = userId, cursor = maxId.orEmpty(), limit = limit ?: 40)
        }
    }

    suspend fun hashtagTimeline(
        hashtag: String,
        any: List<String>?,
        local: Boolean?,
        maxId: String?,
        minId: String? = null,
        sinceId: String?,
        limit: Int?,
    ): Response<List<Status>> = stubList()

    suspend fun listTimeline(
        listId: String,
        maxId: String?,
        minId: String? = null,
        sinceId: String?,
        limit: Int?,
    ): Response<List<Status>> = stubList()

    // ---------------------------------------------------------------
    // notifications
    // ---------------------------------------------------------------

    suspend fun notifications(
        maxId: String? = null,
        sinceId: String? = null,
        minId: String? = null,
        limit: Int? = null,
        excludes: Set<Notification.Type>? = null,
        accountId: String? = null,
    ): Response<List<Notification>> {
        val userId = accountManager.activeAccount?.accountId.orEmpty()
        if (userId.isEmpty()) return stubList()
        return paginated {
            warpnet.getNotifications(userId = userId, cursor = maxId.orEmpty(), limit = limit ?: 40)
        }
    }

    suspend fun notification(id: String): Response<Notification> = stubError()

    suspend fun markersWithAuth(
        auth: String,
        domain: String,
        timelines: List<String>,
    ): Map<String, Marker> = emptyMap()

    suspend fun updateMarkersWithAuth(
        auth: String,
        domain: String,
        homeLastReadId: String? = null,
        notificationsLastReadId: String? = null,
    ): NetworkResult<Unit> = NetworkResult.success(Unit)

    suspend fun notificationsWithAuth(
        auth: String,
        domain: String,
        minId: String?,
    ): Response<List<Notification>> {
        val userId = accountManager.activeAccount?.accountId.orEmpty()
        if (userId.isEmpty()) return stubList()
        return paginated {
            warpnet.getNotifications(userId = userId, cursor = minId.orEmpty(), limit = 40)
        }
    }

    suspend fun clearNotifications(): NetworkResult<Unit> = NetworkResult.success(Unit)

    // ---------------------------------------------------------------
    // media
    // ---------------------------------------------------------------

    suspend fun updateMedia(
        mediaId: String,
        description: String?,
        focus: String?,
    ): NetworkResult<Attachment> = stubFailure("updateMedia")

    suspend fun getMedia(mediaId: String): Response<MediaUploadResult> = stubError()

    // ---------------------------------------------------------------
    // statuses (CRUD + interactions)
    // ---------------------------------------------------------------

    suspend fun createStatus(
        auth: String,
        domain: String,
        idempotencyKey: String,
        status: NewStatus,
    ): NetworkResult<Status> {
        val active = accountManager.activeAccount ?: return stubFailure("createStatus")
        return result {
            warpnet.postStatus(
                text = status.status,
                authorUserId = active.accountId,
                authorUsername = active.username,
                parentId = status.inReplyToId,
            )
        }
    }

    suspend fun createScheduledStatus(
        auth: String,
        domain: String,
        idempotencyKey: String,
        status: NewStatus,
    ): NetworkResult<ScheduledStatusReply> = stubFailure("createScheduledStatus")

    suspend fun status(statusId: String): NetworkResult<Status> = stubFailure("status")

    suspend fun editStatus(
        statusId: String,
        auth: String,
        domain: String,
        idempotencyKey: String,
        editedStatus: NewStatus,
    ): NetworkResult<Status> = stubFailure("editStatus")

    suspend fun statusSource(statusId: String): NetworkResult<StatusSource> = stubFailure("statusSource")

    suspend fun statusContext(statusId: String): NetworkResult<StatusContext> = result {
        val userId = accountManager.activeAccount?.accountId.orEmpty()
        StatusContext(
            ancestors = warpnet.getAncestors(tweetId = statusId, userId = userId),
            descendants = warpnet.getReplies(rootId = statusId),
        )
    }

    suspend fun statusEdits(statusId: String): NetworkResult<List<StatusEdit>> =
        NetworkResult.success(emptyList())

    suspend fun statusRebloggedBy(
        statusId: String,
        maxId: String?,
    ): Response<List<TimelineAccount>> = stubList()

    suspend fun statusFavouritedBy(
        statusId: String,
        maxId: String?,
    ): Response<List<TimelineAccount>> = stubList()

    suspend fun deleteStatus(
        statusId: String,
        deleteMedia: Boolean? = null,
    ): NetworkResult<DeletedStatus> {
        val userId = accountManager.activeAccount?.accountId.orEmpty()
        if (userId.isEmpty()) return stubFailure("deleteStatus")
        return result {
            warpnet.deleteStatus(tweetId = statusId, userId = userId)
            // Warpnet's delete returns no body; the Mastodon shape expects a
            // draftable DeletedStatus. Hand back an empty one — callers use
            // `DeletedStatus.isEmpty` to skip reopening a draft.
            DeletedStatus(
                text = null,
                inReplyToId = null,
                spoilerText = "",
                visibility = Status.Visibility.PUBLIC,
                sensitive = false,
                attachments = emptyList(),
                poll = null,
                createdAt = java.util.Date(),
                language = null,
            )
        }
    }

    suspend fun reblogStatus(
        statusId: String,
        visibility: String?,
    ): NetworkResult<Status> {
        val active = accountManager.activeAccount ?: return stubFailure("reblogStatus")
        return result {
            warpnet.reblogStatus(
                tweetId = statusId,
                retweeterId = active.accountId,
                retweeterUsername = active.username,
            )
            warpnet.getStatus(tweetId = statusId, userId = active.accountId)
        }
    }

    suspend fun unreblogStatus(statusId: String): NetworkResult<Status> {
        val active = accountManager.activeAccount ?: return stubFailure("unreblogStatus")
        return result {
            warpnet.unreblogStatus(tweetId = statusId, retweeterId = active.accountId)
            warpnet.getStatus(tweetId = statusId, userId = active.accountId)
        }
    }

    suspend fun favouriteStatus(statusId: String): NetworkResult<Status> {
        val active = accountManager.activeAccount ?: return stubFailure("favouriteStatus")
        return result {
            warpnet.favouriteStatus(tweetId = statusId, userId = active.accountId)
            warpnet.getStatus(tweetId = statusId, userId = active.accountId)
        }
    }

    suspend fun unfavouriteStatus(statusId: String): NetworkResult<Status> {
        val active = accountManager.activeAccount ?: return stubFailure("unfavouriteStatus")
        return result {
            warpnet.unfavouriteStatus(tweetId = statusId, userId = active.accountId)
            warpnet.getStatus(tweetId = statusId, userId = active.accountId)
        }
    }

    suspend fun bookmarkStatus(statusId: String): NetworkResult<Status> = stubFailure("bookmarkStatus")

    suspend fun unbookmarkStatus(statusId: String): NetworkResult<Status> = stubFailure("unbookmarkStatus")

    suspend fun pinStatus(statusId: String): NetworkResult<Status> = stubFailure("pinStatus")

    suspend fun unpinStatus(statusId: String): NetworkResult<Status> = stubFailure("unpinStatus")

    suspend fun muteConversation(statusId: String): NetworkResult<Status> = stubFailure("muteConversation")

    suspend fun unmuteConversation(statusId: String): NetworkResult<Status> = stubFailure("unmuteConversation")

    suspend fun scheduledStatuses(
        limit: Int? = null,
        maxId: String? = null,
    ): Response<List<ScheduledStatus>> = stubList()

    suspend fun deleteScheduledStatus(scheduledStatusId: String): NetworkResult<Unit> =
        NetworkResult.success(Unit)

    // ---------------------------------------------------------------
    // accounts
    // ---------------------------------------------------------------

    // Warpdroid has no login flow — the stub account stands in for what
    // OAuth login would normally populate. Resolve locally from the
    // AccountEntity instead of calling Warpnet (which may be offline /
    // uninitialised at app start).
    suspend fun accountVerifyCredentials(
        domain: String? = null,
        auth: String? = null,
    ): NetworkResult<Account> {
        val active = accountManager.activeAccount
            ?: return stubFailure("accountVerifyCredentials")
        return NetworkResult.success(
            Account(
                id = active.accountId,
                localUsername = active.username,
                username = active.username,
                displayName = active.displayName,
                createdAt = Date(0),
                note = "",
                url = "${WarpnetMapper.FAKE_BASE_URL}/users/${active.accountId}",
                avatar = active.profilePictureUrl,
                header = active.profileHeaderUrl,
                locked = active.locked,
                emojis = active.emojis,
            )
        )
    }

    suspend fun accountUpdateSource(
        privacy: String?,
        sensitive: Boolean?,
        language: String?,
        quotePolicy: String?,
    ): NetworkResult<Account> = stubFailure("accountUpdateSource")

    suspend fun accountUpdateCredentials(
        displayName: RequestBody?,
        note: RequestBody?,
        locked: RequestBody?,
        avatar: MultipartBody.Part?,
        header: MultipartBody.Part?,
        fields: Map<String, RequestBody>,
    ): NetworkResult<Account> = stubFailure("accountUpdateCredentials")

    /**
     * Warpnet has no server-side text search, so we page through
     * [WarpnetRepository.listUsers] for the caller's viewpoint and filter
     * usernames/display names client-side. Cheap enough for short queries;
     * when the catalogue grows we should push this server-side.
     */
    suspend fun searchAccounts(
        query: String,
        resolve: Boolean? = null,
        limit: Int? = null,
        following: Boolean? = null,
    ): NetworkResult<List<TimelineAccount>> {
        val me = accountManager.activeAccount?.accountId.orEmpty()
        if (me.isEmpty() || query.isBlank()) return NetworkResult.success(emptyList())
        return result {
            val (users, _) = warpnet.listUsers(requesterUserId = me, limit = (limit ?: 40).coerceAtLeast(1))
            val needle = query.trim().lowercase()
            users.filter { acc ->
                acc.username.lowercase().contains(needle) ||
                    acc.displayName.orEmpty().lowercase().contains(needle)
            }
        }
    }

    suspend fun account(accountId: String): NetworkResult<Account> = result {
        warpnet.getAccount(accountId)
    }

    suspend fun accountStatuses(
        accountId: String,
        maxId: String? = null,
        minId: String? = null,
        sinceId: String? = null,
        limit: Int? = null,
        excludeReplies: Boolean? = null,
        excludeReblogs: Boolean? = null,
        onlyMedia: Boolean? = null,
        pinned: Boolean? = null,
    ): Response<List<Status>> = paginated {
        warpnet.getUserTimeline(userId = accountId, cursor = maxId.orEmpty(), limit = limit ?: 40)
    }

    suspend fun accountFollowers(
        accountId: String,
        maxId: String?,
    ): Response<List<TimelineAccount>> = paginated {
        warpnet.getFollowers(userId = accountId, cursor = maxId.orEmpty(), limit = 40)
    }

    suspend fun accountFollowing(
        accountId: String,
        maxId: String?,
    ): Response<List<TimelineAccount>> = paginated {
        warpnet.getFollowings(userId = accountId, cursor = maxId.orEmpty(), limit = 40)
    }

    suspend fun followAccount(
        accountId: String,
        showReblogs: Boolean? = null,
        notify: Boolean? = null,
    ): NetworkResult<Relationship> {
        val me = accountManager.activeAccount?.accountId.orEmpty()
        if (me.isEmpty()) return stubFailure("followAccount")
        return result { warpnet.followAccount(followerId = me, followeeId = accountId) }
    }

    suspend fun unfollowAccount(accountId: String): NetworkResult<Relationship> {
        val me = accountManager.activeAccount?.accountId.orEmpty()
        if (me.isEmpty()) return stubFailure("unfollowAccount")
        return result { warpnet.unfollowAccount(followerId = me, followeeId = accountId) }
    }

    suspend fun blockAccount(accountId: String): NetworkResult<Relationship> = stubFailure("blockAccount")

    suspend fun unblockAccount(accountId: String): NetworkResult<Relationship> = stubFailure("unblockAccount")

    suspend fun muteAccount(
        accountId: String,
        notifications: Boolean? = null,
        duration: Int? = null,
    ): NetworkResult<Relationship> = stubFailure("muteAccount")

    suspend fun unmuteAccount(accountId: String): NetworkResult<Relationship> = stubFailure("unmuteAccount")

    suspend fun relationships(accountIds: List<String>): NetworkResult<List<Relationship>> = result {
        accountIds.map { warpnet.relationshipFor(it) }
    }

    suspend fun subscribeAccount(accountId: String): NetworkResult<Relationship> = stubFailure("subscribeAccount")

    suspend fun unsubscribeAccount(accountId: String): NetworkResult<Relationship> = stubFailure("unsubscribeAccount")

    suspend fun blocks(maxId: String? = null): Response<List<TimelineAccount>> = stubList()
    suspend fun mutes(maxId: String? = null): Response<List<TimelineAccount>> = stubList()

    suspend fun domainBlocks(
        maxId: String? = null,
        sinceId: String? = null,
        limit: Int? = null,
    ): Response<List<String>> = stubList()

    suspend fun blockDomain(domain: String): NetworkResult<Unit> = NetworkResult.success(Unit)
    suspend fun unblockDomain(domain: String): NetworkResult<Unit> = NetworkResult.success(Unit)

    suspend fun favourites(
        maxId: String?,
        minId: String? = null,
        sinceId: String?,
        limit: Int?,
    ): Response<List<Status>> = stubList()

    suspend fun bookmarks(
        maxId: String?,
        minId: String? = null,
        sinceId: String?,
        limit: Int?,
    ): Response<List<Status>> = stubList()

    suspend fun followRequests(maxId: String?): Response<List<TimelineAccount>> = stubList()

    suspend fun authorizeFollowRequest(accountId: String): NetworkResult<Relationship> =
        stubFailure("authorizeFollowRequest")

    suspend fun rejectFollowRequest(accountId: String): NetworkResult<Relationship> =
        stubFailure("rejectFollowRequest")

    // ---------------------------------------------------------------
    // oauth (kept for symmetry — Warpnet pairing is handled elsewhere)
    // ---------------------------------------------------------------

    suspend fun authenticateApp(
        domain: String,
        clientName: String,
        redirectUris: String,
        scopes: String,
        website: String,
    ): NetworkResult<AppCredentials> = stubFailure("authenticateApp")

    suspend fun fetchOAuthToken(
        domain: String,
        clientId: String,
        clientSecret: String,
        redirectUri: String,
        code: String,
        grantType: String,
    ): NetworkResult<AccessToken> = stubFailure("fetchOAuthToken")

    suspend fun revokeOAuthToken(
        clientId: String,
        clientSecret: String,
        token: String,
    ): NetworkResult<Unit> = NetworkResult.success(Unit)

    // ---------------------------------------------------------------
    // lists
    // ---------------------------------------------------------------

    suspend fun getLists(): NetworkResult<List<MastoList>> = NetworkResult.success(emptyList())

    suspend fun getListsIncludesAccount(accountId: String): NetworkResult<List<MastoList>> =
        NetworkResult.success(emptyList())

    suspend fun createList(
        title: String,
        exclusive: Boolean?,
        replyPolicy: String,
    ): NetworkResult<MastoList> = stubFailure("createList")

    suspend fun updateList(
        listId: String,
        title: String,
        exclusive: Boolean?,
        replyPolicy: String,
    ): NetworkResult<MastoList> = stubFailure("updateList")

    suspend fun deleteList(listId: String): NetworkResult<Unit> = NetworkResult.success(Unit)

    suspend fun getAccountsInList(
        listId: String,
        limit: Int,
    ): NetworkResult<List<TimelineAccount>> = NetworkResult.success(emptyList())

    suspend fun deleteAccountFromList(
        listId: String,
        accountIds: List<String>,
    ): NetworkResult<Unit> = NetworkResult.success(Unit)

    suspend fun addAccountToList(
        listId: String,
        accountIds: List<String>,
    ): NetworkResult<Unit> = NetworkResult.success(Unit)

    // ---------------------------------------------------------------
    // conversations (Mastodon DMs; Warpnet chat has different shape)
    // ---------------------------------------------------------------

    suspend fun getConversations(
        maxId: String? = null,
        limit: Int? = null,
    ): Response<List<Conversation>> = stubList()

    suspend fun deleteConversation(conversationId: String): NetworkResult<Unit> =
        NetworkResult.success(Unit)

    // ---------------------------------------------------------------
    // polls, announcements, reports, search
    // ---------------------------------------------------------------

    suspend fun voteInPoll(id: String, choices: List<Int>): NetworkResult<Poll> = stubFailure("voteInPoll")

    suspend fun announcements(): NetworkResult<List<Announcement>> = NetworkResult.success(emptyList())

    suspend fun dismissAnnouncement(announcementId: String): NetworkResult<Unit> =
        NetworkResult.success(Unit)

    suspend fun addAnnouncementReaction(
        announcementId: String,
        name: String,
    ): NetworkResult<Unit> = NetworkResult.success(Unit)

    suspend fun removeAnnouncementReaction(
        announcementId: String,
        name: String,
    ): NetworkResult<Unit> = NetworkResult.success(Unit)

    suspend fun report(
        accountId: String,
        statusIds: Set<String>,
        comment: String,
        forward: Boolean?,
        category: String?,
        ruleIds: Set<String>?,
    ): NetworkResult<Unit> = NetworkResult.success(Unit)

    suspend fun search(
        query: String?,
        type: String? = null,
        resolve: Boolean? = null,
        limit: Int? = null,
        offset: Int? = null,
        following: Boolean? = null,
    ): NetworkResult<SearchResult> = stubFailure("search")

    suspend fun updateAccountNote(
        accountId: String,
        note: String,
    ): NetworkResult<Relationship> = stubFailure("updateAccountNote")

    // ---------------------------------------------------------------
    // push subscription
    // ---------------------------------------------------------------

    suspend fun pushNotificationSubscription(
        auth: String,
        domain: String,
    ): NetworkResult<NotificationSubscribeResult> = stubFailure("pushNotificationSubscription")

    suspend fun subscribePushNotifications(
        auth: String,
        domain: String,
        standard: Boolean,
        endpoint: String,
        keysP256DH: String,
        keysAuth: String,
        data: Map<String, Boolean>,
    ): NetworkResult<NotificationSubscribeResult> = stubFailure("subscribePushNotifications")

    suspend fun updatePushNotificationSubscription(
        auth: String,
        domain: String,
        data: Map<String, Boolean>,
    ): NetworkResult<NotificationSubscribeResult> = stubFailure("updatePushNotificationSubscription")

    suspend fun unsubscribePushNotifications(
        auth: String,
        domain: String,
    ): NetworkResult<Unit> = NetworkResult.success(Unit)

    // ---------------------------------------------------------------
    // tags + trends
    // ---------------------------------------------------------------

    suspend fun tag(name: String): NetworkResult<HashTag> = stubFailure("tag")

    suspend fun followedTags(
        minId: String? = null,
        sinceId: String? = null,
        maxId: String? = null,
        limit: Int? = null,
    ): Response<List<HashTag>> = stubList()

    suspend fun followTag(name: String): NetworkResult<HashTag> = stubFailure("followTag")
    suspend fun unfollowTag(name: String): NetworkResult<HashTag> = stubFailure("unfollowTag")

    suspend fun trendingTags(): NetworkResult<List<TrendingTag>> = NetworkResult.success(emptyList())

    suspend fun trendingStatuses(
        limit: Int? = null,
        offset: String? = null,
    ): Response<List<Status>> = stubList()

    suspend fun quotingStatuses(
        statusId: String,
        limit: Int? = null,
        offset: String? = null,
    ): Response<List<Status>> = stubList()

    suspend fun translate(
        statusId: String,
        targetLanguage: String?,
    ): NetworkResult<Translation> = stubFailure("translate")

    // ---------------------------------------------------------------
    // notification policy + requests
    // ---------------------------------------------------------------

    suspend fun notificationPolicy(): NetworkResult<NotificationPolicy> = stubFailure("notificationPolicy")

    suspend fun updateNotificationPolicy(
        forNotFollowing: String?,
        forNotFollowers: String?,
        forNewAccounts: String?,
        forPrivateMentions: String?,
        forLimitedAccounts: String?,
    ): NetworkResult<NotificationPolicy> = stubFailure("updateNotificationPolicy")

    suspend fun getNotificationRequests(
        maxId: String? = null,
        minId: String? = null,
        sinceId: String? = null,
        limit: Int? = null,
    ): Response<List<NotificationRequest>> = stubList()

    suspend fun acceptNotificationRequest(notificationId: String): NetworkResult<Unit> =
        NetworkResult.success(Unit)

    suspend fun dismissNotificationRequest(notificationId: String): NetworkResult<Unit> =
        NetworkResult.success(Unit)

    // ---------------------------------------------------------------
    // quotes
    // ---------------------------------------------------------------

    suspend fun removeQuote(
        id: String,
        quotingStatusId: String,
    ): NetworkResult<Status> = stubFailure("removeQuote")
}
