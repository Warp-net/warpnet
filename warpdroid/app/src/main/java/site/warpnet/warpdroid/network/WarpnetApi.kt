/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Drop-in stand-in for Warpdroid's Retrofit WarpnetApi interface.
 *
 * Warpnet is a libp2p P2P network, not an HTTP Warpnet instance, so this
 * class is NOT backed by Retrofit. It keeps the same method signatures and
 * return types as the original so the ~50 view models / use cases that call
 * it keep compiling. Behaviour per method is one of two things:
 *
 *   - **Stubbed** — returns a predefined `NetworkResult.failure` or
 *     `Response.error(501, ...)`. Used for surfaces with no Warpnet
 *     equivalent (filters, lists, announcements, domain blocks, reports,
 *     scheduled statuses, trending, translate, polls, push subscriptions,
 *     custom emojis, OAuth, instance metadata, bookmarks, markers,
 *     conversations — the fediverse/instance-admin half of the Warpnet
 *     API). Chunk 1 stubs everything; subsequent chunks migrate tractable
 *     methods to [WarpnetRepository].
 *   - **Warpnet-backed** — delegates to [WarpnetRepository]. None in this
 *     chunk; chunks 2-5 wire these in one feature group at a time.
 */
package site.warpnet.warpdroid.network

import android.net.Uri
import android.util.Log
import at.connyduck.calladapter.networkresult.NetworkResult
import site.warpnet.warpdroid.components.filters.FilterExpiration
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.User
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.entity.DeletedTweet
import site.warpnet.warpdroid.entity.Emoji
import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.entity.FilterKeyword
import site.warpnet.warpdroid.entity.MediaUploadResult
import site.warpnet.warpdroid.entity.NewTweet
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.Relationship
import site.warpnet.warpdroid.entity.SearchResult
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.entity.TweetContext
import site.warpnet.warpdroid.entity.TweetSource
import site.warpnet.warpdroid.entity.TimelineUser
import site.warpnet.warpdroid.warpnet.WarpnetMapper
import site.warpnet.warpdroid.warpnet.WarpnetRepository
import java.util.Date
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.CancellationException
import okhttp3.Headers
import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.MultipartBody
import okhttp3.RequestBody
import okhttp3.ResponseBody.Companion.toResponseBody
import retrofit2.Response

@Singleton
class WarpnetApi @Inject constructor(
    @Suppress("unused") private val warpnet: WarpnetRepository,
    @Suppress("unused") private val accountManager: AccountManager,
) {

    companion object {
        private const val TAG = "WarpnetApi"

        // Matches database/local-store/db.go EndCursor: the sentinel the
        // store returns once a prefix scan is exhausted.
        private const val END_CURSOR = "end"

        private val STUB_BODY = "".toResponseBody("text/plain".toMediaTypeOrNull())
        private fun unsupported(name: String) =
            UnsupportedOperationException("WarpnetApi.$name has no Warpnet equivalent")
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
     * synthesise a Warpnet-style `Link: <url?max_id=CURSOR>; rel="next"`
     * header when a follow-up cursor exists. NetworkTimelineRemoteMediator
     * (and peers) extract `max_id` from that header to drive pagination.
     */
    private suspend fun <T> paginated(block: suspend () -> Pair<List<T>, String>): Response<List<T>> = try {
        val (items, nextCursor) = block()
        // The Warpnet store signals "no more pages" with the sentinel
        // "end" (and "" when a node returns nothing). Don't synthesise a
        // next link for either, or the RemoteMediator fires one more
        // pointless APPEND that re-queries an exhausted cursor. The cursor
        // is an opaque DB key that can contain URL-reserved characters
        // ('/', '=', ...), so percent-encode it; the mediators read it back
        // with Uri.getQueryParameter, which decodes automatically.
        val headers = if (nextCursor.isNotEmpty() && nextCursor != END_CURSOR) {
            Headers.headersOf(
                "Link",
                "<${WarpnetMapper.FAKE_BASE_URL}/?max_id=${Uri.encode(nextCursor)}>; rel=\"next\"",
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
    // filters
    // ---------------------------------------------------------------

    suspend fun getFilter(filterId: String): NetworkResult<Filter> {
        val active = accountManager.activeAccount ?: return stubFailure("getFilter")
        return result {
            val wf = warpnet.getFilter(userId = active.accountId, filterId = filterId)
                ?: throw NoSuchElementException("filter $filterId")
            wf.toWarpnetFilter()
        }
    }

    suspend fun getFilters(): NetworkResult<List<Filter>> {
        val active = accountManager.activeAccount ?: return NetworkResult.success(emptyList())
        return result {
            val (fs, _) = warpnet.getFilters(userId = active.accountId)
            fs.map { it.toWarpnetFilter() }
        }
    }

    suspend fun createFilter(
        title: String,
        context: List<Filter.Kind>,
        filterAction: Filter.Action,
        expiresIn: FilterExpiration?,
    ): NetworkResult<Filter> {
        val active = accountManager.activeAccount ?: return stubFailure("createFilter")
        return result {
            val wf = warpnet.createFilter(
                site.warpnet.transport.dto.WarpnetFilter(
                    userId = active.accountId,
                    title = title,
                    context = context.map { it.kind },
                    action = filterAction.action,
                ),
            )
            wf.toWarpnetFilter()
        }
    }

    suspend fun updateFilter(
        id: String,
        title: String? = null,
        context: List<Filter.Kind>? = null,
        filterAction: Filter.Action? = null,
        expires: FilterExpiration? = null,
    ): NetworkResult<Filter> {
        val active = accountManager.activeAccount ?: return stubFailure("updateFilter")
        return result {
            val wf = warpnet.updateFilter(
                site.warpnet.transport.dto.WarpnetFilter(
                    id = id,
                    userId = active.accountId,
                    title = title.orEmpty(),
                    context = context?.map { it.kind } ?: emptyList(),
                    action = filterAction?.action.orEmpty(),
                ),
            )
            wf.toWarpnetFilter()
        }
    }

    suspend fun deleteFilter(id: String): NetworkResult<Unit> {
        val active = accountManager.activeAccount ?: return NetworkResult.success(Unit)
        return result {
            warpnet.deleteFilter(userId = active.accountId, filterId = id)
        }
    }

    suspend fun addFilterKeyword(
        filterId: String,
        keyword: String,
        wholeWord: Boolean,
    ): NetworkResult<FilterKeyword> {
        val active = accountManager.activeAccount ?: return stubFailure("addFilterKeyword")
        return result {
            val kw = warpnet.addFilterKeyword(
                userId = active.accountId,
                filterId = filterId,
                keyword = keyword,
                wholeWord = wholeWord,
            )
            FilterKeyword(id = kw.id, keyword = kw.keyword, wholeWord = kw.wholeWord)
        }
    }

    suspend fun updateFilterKeyword(
        keywordId: String,
        keyword: String,
        wholeWord: Boolean,
    ): NetworkResult<FilterKeyword> {
        val active = accountManager.activeAccount ?: return stubFailure("updateFilterKeyword")
        return result {
            val kw = warpnet.updateFilterKeyword(
                userId = active.accountId,
                keywordId = keywordId,
                keyword = keyword,
                wholeWord = wholeWord,
            )
            FilterKeyword(id = kw.id, keyword = kw.keyword, wholeWord = kw.wholeWord)
        }
    }

    suspend fun deleteFilterKeyword(keywordId: String): NetworkResult<Unit> {
        val active = accountManager.activeAccount ?: return NetworkResult.success(Unit)
        return result {
            warpnet.deleteFilterKeyword(userId = active.accountId, keywordId = keywordId)
        }
    }

    private fun site.warpnet.transport.dto.WarpnetFilter.toWarpnetFilter(): Filter = Filter(
        id = id,
        title = title,
        context = context.map { Filter.Kind.from(it) },
        expiresAt = expiresAt?.let { runCatching { java.util.Date.from(java.time.Instant.parse(it)) }.getOrNull() },
        action = Filter.Action.from(action),
        keywords = keywords.map { FilterKeyword(id = it.id, keyword = it.keyword, wholeWord = it.wholeWord) },
    )

    // ---------------------------------------------------------------
    // timeline
    //
    // Warpnet has a single timeline: the local user's own tweets merged
    // with their followings' tweets (plus recommendations). The
    // `homeTimeline` name is kept only because the UI layer above
    // expects this entry point — it maps 1:1 to PRIVATE_GET_TIMELINE.
    // ---------------------------------------------------------------

    suspend fun homeTimeline(
        maxId: String? = null,
        minId: String? = null,
        sinceId: String? = null,
        limit: Int? = null,
    ): Response<List<Tweet>> = paginated {
        warpnet.getHomeTimeline(cursor = maxId.orEmpty(), limit = limit ?: 20)
    }

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
            warpnet.getNotifications(userId = userId, cursor = maxId.orEmpty(), limit = limit ?: 20)
        }
    }

    suspend fun notification(id: String): Response<Notification> = response {
        warpnet.getNotification(notificationId = id)
            ?: throw NoSuchElementException("notification $id not found")
    }

    suspend fun notificationsPage(
        cursor: String,
    ): Response<List<Notification>> {
        val userId = accountManager.activeAccount?.accountId.orEmpty()
        if (userId.isEmpty()) return stubList()
        return paginated {
            warpnet.getPushes(cursor = cursor, limit = 40)
        }
    }

    // ---------------------------------------------------------------
    // media
    // ---------------------------------------------------------------

    suspend fun updateMedia(
        mediaId: String,
        description: String?,
        focus: String?,
    ): NetworkResult<Attachment> {
        val active = accountManager.activeAccount ?: return stubFailure("updateMedia")
        val (fx, fy) = parseFocus(focus)
        return result {
            warpnet.updateMediaMeta(
                userId = active.accountId,
                key = mediaId,
                description = description.orEmpty(),
                focusX = fx,
                focusY = fy,
            )
            // Warpnet's stored attachment isn't surfaced as a separate
            // record — the next status fetch reads description / focus
            // alongside the tweet. Return a minimal Attachment so the
            // compose screen can echo the edit back.
            Attachment(
                id = mediaId,
                url = "",
                description = description,
                meta = Attachment.MetaData(
                    focus = Attachment.Focus(x = fx, y = fy),
                ),
                type = Attachment.Type.IMAGE,
            )
        }
    }

    suspend fun getMedia(mediaId: String): Response<MediaUploadResult> {
        val active = accountManager.activeAccount ?: return stubError()
        return response {
            val meta = warpnet.getMediaMeta(userId = active.accountId, key = mediaId)
            // MediaUploadResult intentionally only carries the id — see
            // entity/MediaUploadResult.kt. The descriptive metadata flows
            // back via the Attachment surface on the next status fetch.
            MediaUploadResult(id = meta.key)
        }
    }

    private fun parseFocus(focus: String?): Pair<Float, Float> {
        if (focus.isNullOrBlank()) return 0f to 0f
        val parts = focus.split(',', limit = 2)
        val x = parts.getOrNull(0)?.toFloatOrNull() ?: 0f
        val y = parts.getOrNull(1)?.toFloatOrNull() ?: 0f
        return x to y
    }

    // ---------------------------------------------------------------
    // statuses (CRUD + interactions)
    // ---------------------------------------------------------------

    suspend fun createStatus(
        idempotencyKey: String,
        status: NewTweet,
    ): NetworkResult<Tweet> {
        val active = accountManager.activeAccount ?: return stubFailure("createStatus")
        // The Warpnet `Tweet.Username` field is the human-readable display
        // name (e.g. "Vadim") — desktop renders it verbatim as the author
        // line. WarpnetMapper.toAccount maps WarpnetUser.id → User.username
        // (the @-handle, peer-derived ULID) and WarpnetUser.username →
        // User.displayName (the real name). So the tweet's authorUsername
        // has to be sourced from displayName, not username, otherwise the
        // post shows up authored by the ULID. Fall back to the @-handle if
        // displayName isn't populated yet.
        val authorName = active.displayName.ifBlank { active.username }
        return result {
            warpnet.postStatus(
                text = status.status,
                authorUserId = active.accountId,
                authorUsername = authorName,
                parentId = status.inReplyToId,
            )
        }
    }

    /**
     * Fetch a single status. Warpnet's wire requires the tweet's author, since
     * the canonical record lives on the author's node — every call site must
     * supply [authorId]; passing the active account's id for someone else's
     * status will fail.
     */
    suspend fun status(statusId: String, authorId: String): NetworkResult<Tweet> {
        if (authorId.isEmpty()) {
            return NetworkResult.failure(IllegalArgumentException("status($statusId): authorId is required"))
        }
        return result {
            warpnet.getStatus(tweetId = statusId, userId = authorId)
        }
    }

    suspend fun editStatus(
        statusId: String,
        idempotencyKey: String,
        editedStatus: NewTweet,
    ): NetworkResult<Tweet> {
        val active = accountManager.activeAccount ?: return stubFailure("editStatus")
        return result {
            warpnet.editTweet(
                tweetId = statusId,
                userId = active.accountId,
                text = editedStatus.status,
            )
            warpnet.getStatus(tweetId = statusId, userId = active.accountId)
        }
    }

    /**
     * Warpnet has no rendering step — domain.Tweet.text already *is* the
     * plaintext source — so this is synthesised client-side from the
     * current status fetch. No wire call.
     */
    suspend fun statusSource(statusId: String): NetworkResult<TweetSource> {
        val active = accountManager.activeAccount ?: return stubFailure("statusSource")
        return result {
            val t = warpnet.getStatus(tweetId = statusId, userId = active.accountId)
            TweetSource(id = t.id, text = t.content.toString(), spoilerText = t.spoilerText)
        }
    }

    /**
     * Ancestors + descendants for a single status. Like [status], the author
     * is required because the ancestor walk lives on the author's node.
     */
    suspend fun statusContext(statusId: String, authorId: String): NetworkResult<TweetContext> {
        if (authorId.isEmpty()) {
            return NetworkResult.failure(IllegalArgumentException("statusContext($statusId): authorId is required"))
        }
        return result {
            TweetContext(
                ancestors = warpnet.getAncestors(tweetId = statusId, userId = authorId),
                descendants = warpnet.getReplies(rootId = statusId),
            )
        }
    }

    suspend fun statusRetweetedBy(
        statusId: String,
        maxId: String?,
    ): Response<List<TimelineUser>> {
        // ownerUserId is unavailable at the AccountList mediator level —
        // fall back to the active account so the handler returns the
        // local record (correct for self-engagement, the common case).
        val active = accountManager.activeAccount ?: return stubList()
        return paginated {
            warpnet.getTweetRetweeters(
                tweetId = statusId,
                ownerUserId = active.accountId,
                cursor = maxId.orEmpty(),
            )
        }
    }

    suspend fun statusLikedBy(
        statusId: String,
        maxId: String?,
    ): Response<List<TimelineUser>> {
        val active = accountManager.activeAccount ?: return stubList()
        return paginated {
            warpnet.getTweetLikers(
                tweetId = statusId,
                ownerUserId = active.accountId,
                cursor = maxId.orEmpty(),
            )
        }
    }

    suspend fun deleteStatus(
        statusId: String,
        deleteMedia: Boolean? = null,
    ): NetworkResult<DeletedTweet> {
        val userId = accountManager.activeAccount?.accountId.orEmpty()
        if (userId.isEmpty()) return stubFailure("deleteStatus")
        return result {
            warpnet.deleteStatus(tweetId = statusId, userId = userId)
            // Warpnet's delete returns no body; the Warpnet shape expects a
            // draftable DeletedTweet. Hand back an empty one — callers use
            // `DeletedTweet.isEmpty` to skip reopening a draft.
            DeletedTweet(
                text = null,
                inReplyToId = null,
                spoilerText = "",
                visibility = Tweet.Visibility.PUBLIC,
                sensitive = false,
                attachments = emptyList(),
                createdAt = java.util.Date(),
                language = null,
            )
        }
    }

    suspend fun retweetStatus(
        statusId: String,
        visibility: String?,
        sourceAuthorId: String? = null,
        comment: String? = null,
    ): NetworkResult<Tweet> {
        val active = accountManager.activeAccount ?: return stubFailure("retweetStatus")
        // Same reasoning as createStatus: the wire-level username field is
        // the display name, not the @-handle.
        val retweeterName = active.displayName.ifBlank { active.username }
        return result {
            warpnet.retweetStatus(
                tweetId = statusId,
                retweeterId = active.accountId,
                retweeterUsername = retweeterName,
                sourceAuthorId = sourceAuthorId,
                comment = comment,
            )
            warpnet.getStatus(tweetId = statusId, userId = active.accountId)
        }
    }

    suspend fun unretweetStatus(statusId: String): NetworkResult<Tweet> {
        val active = accountManager.activeAccount ?: return stubFailure("unretweetStatus")
        return result {
            warpnet.unretweetStatus(tweetId = statusId, retweeterId = active.accountId)
            warpnet.getStatus(tweetId = statusId, userId = active.accountId)
        }
    }

    // Warpnet's LikeEvent semantics: user_id = tweet author, owner_id = liker
    // (core/handler/like.go:79-87). [authorId] is the actionable status'
    // author id (TweetViewData.actionableAccountId); the liker is the
    // locally active account.
    suspend fun likeStatus(statusId: String, authorId: String): NetworkResult<Tweet> {
        val active = accountManager.activeAccount ?: return stubFailure("likeStatus")
        return result {
            warpnet.likeStatus(tweetId = statusId, userId = authorId, ownerId = active.accountId)
            warpnet.getStatus(tweetId = statusId, userId = active.accountId)
        }
    }

    suspend fun unlikeStatus(statusId: String, authorId: String): NetworkResult<Tweet> {
        val active = accountManager.activeAccount ?: return stubFailure("unlikeStatus")
        return result {
            warpnet.unlikeStatus(tweetId = statusId, userId = authorId, ownerId = active.accountId)
            warpnet.getStatus(tweetId = statusId, userId = active.accountId)
        }
    }

    /**
     * Record a tweet-view event when a status comes on screen.
     *
     * Side-effect-only: the post-increment count is dropped because
     * existing [Tweet] view models don't have a slot for it, and the
     * timeline already shows the count via [Tweet.viewsCount] from
     * the periodic stats refresh. Failures are logged at WARN level
     * (so offline / misconfigured nodes are diagnosable) but never
     * propagate to the UI.
     */
    suspend fun recordView(statusId: String, authorId: String) {
        val active = accountManager.activeAccount ?: return
        if (statusId.isBlank() || authorId.isBlank()) return
        runCatching {
            warpnet.recordView(
                tweetId = statusId,
                authorId = authorId,
                viewerId = active.accountId,
            )
        }.onFailure { e ->
            Log.w(TAG, "recordView failed for $statusId", e)
        }
    }

    suspend fun bookmarkStatus(statusId: String, authorId: String): NetworkResult<Tweet> {
        val active = accountManager.activeAccount ?: return stubFailure("bookmarkStatus")
        if (authorId.isEmpty()) {
            return NetworkResult.failure(IllegalArgumentException("bookmarkStatus: authorId required"))
        }
        return result {
            warpnet.bookmarkTweet(userId = active.accountId, tweetId = statusId, ownerUserId = authorId)
            warpnet.getStatus(tweetId = statusId, userId = authorId)
        }
    }

    suspend fun unbookmarkStatus(statusId: String, authorId: String): NetworkResult<Tweet> {
        val active = accountManager.activeAccount ?: return stubFailure("unbookmarkStatus")
        if (authorId.isEmpty()) {
            return NetworkResult.failure(IllegalArgumentException("unbookmarkStatus: authorId required"))
        }
        return result {
            warpnet.unbookmarkTweet(userId = active.accountId, tweetId = statusId)
            warpnet.getStatus(tweetId = statusId, userId = authorId)
        }
    }

    suspend fun pinStatus(statusId: String): NetworkResult<Tweet> {
        val active = accountManager.activeAccount ?: return stubFailure("pinStatus")
        return result {
            warpnet.pinTweet(userId = active.accountId, tweetId = statusId)
            warpnet.getStatus(tweetId = statusId, userId = active.accountId)
        }
    }

    suspend fun unpinStatus(statusId: String): NetworkResult<Tweet> {
        val active = accountManager.activeAccount ?: return stubFailure("unpinStatus")
        return result {
            warpnet.unpinTweet(userId = active.accountId, tweetId = statusId)
            warpnet.getStatus(tweetId = statusId, userId = active.accountId)
        }
    }


    // ---------------------------------------------------------------
    // accounts
    // ---------------------------------------------------------------

    // Warpdroid has no login flow — the stub account stands in for what
    // OAuth login would normally populate. Resolve from Warpnet so the
    // username/displayName/avatar reflect the paired identity instead of
    // the AccountManager stub ("me"); MainActivity only syncs accountId
    // from PairedNodeStore and would otherwise leave username at "me",
    // which would be sent verbatim by createStatus and stored in the
    // tweet author field on the fat node. If the lookup fails (offline,
    // not yet paired) fall back to the local stub so callers still get
    // a non-null User.
    suspend fun accountVerifyCredentials(
        domain: String? = null,
        auth: String? = null,
    ): NetworkResult<User> {
        val active = accountManager.activeAccount
            ?: return stubFailure("accountVerifyCredentials")
        if (active.accountId.isNotEmpty() && active.accountId != AccountManager.STUB_USERNAME) {
            try {
                return NetworkResult.success(warpnet.getAccount(active.accountId))
            } catch (ce: CancellationException) {
                throw ce
            } catch (t: Throwable) {
                Log.w(TAG, "accountVerifyCredentials: getAccount(${active.accountId}) failed, falling back to stub", t)
            }
        }
        return NetworkResult.success(
            User(
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
    ): NetworkResult<User> = stubFailure("accountUpdateSource")

    suspend fun accountUpdateCredentials(
        displayName: RequestBody?,
        note: RequestBody?,
        locked: RequestBody?,
        avatar: MultipartBody.Part?,
        header: MultipartBody.Part?,
        fields: Map<String, RequestBody>,
    ): NetworkResult<User> = stubFailure("accountUpdateCredentials")

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
    ): NetworkResult<List<TimelineUser>> {
        if (query.isBlank()) return NetworkResult.success(emptyList())
        return result {
            val (hits, _) = warpnet.searchAccounts(
                query = query,
                limit = (limit ?: 40).coerceAtLeast(1),
            )
            hits
        }
    }

    suspend fun account(accountId: String): NetworkResult<User> = result {
        warpnet.getAccount(accountId)
    }

    suspend fun accountStatuses(
        accountId: String,
        maxId: String? = null,
        minId: String? = null,
        sinceId: String? = null,
        limit: Int? = null,
        excludeReplies: Boolean? = null,
        excludeRetweets: Boolean? = null,
        onlyMedia: Boolean? = null,
        pinned: Boolean? = null,
    ): Response<List<Tweet>> = paginated {
        warpnet.getUserTimeline(userId = accountId, cursor = maxId.orEmpty(), limit = limit ?: 40)
    }

    suspend fun accountFollowers(
        accountId: String,
        maxId: String?,
    ): Response<List<TimelineUser>> = paginated {
        warpnet.getFollowers(userId = accountId, cursor = maxId.orEmpty(), limit = 40)
    }

    suspend fun accountFollowing(
        accountId: String,
        maxId: String?,
    ): Response<List<TimelineUser>> = paginated {
        warpnet.getFollowings(userId = accountId, cursor = maxId.orEmpty(), limit = 40)
    }

    suspend fun followAccount(
        accountId: String,
        showRetweets: Boolean? = null,
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

    suspend fun blockAccount(accountId: String): NetworkResult<Relationship> {
        val active = accountManager.activeAccount ?: return stubFailure("blockAccount")
        return result {
            warpnet.blockUser(blockerId = active.accountId, blockeeId = accountId)
            warpnet.relationshipFor(accountId).copy(blocking = true)
        }
    }

    suspend fun unblockAccount(accountId: String): NetworkResult<Relationship> {
        val active = accountManager.activeAccount ?: return stubFailure("unblockAccount")
        return result {
            warpnet.unblockUser(blockerId = active.accountId, blockeeId = accountId)
            warpnet.relationshipFor(accountId).copy(blocking = false)
        }
    }

    suspend fun muteAccount(
        accountId: String,
        notifications: Boolean? = null,
        duration: Int? = null,
    ): NetworkResult<Relationship> {
        val active = accountManager.activeAccount ?: return stubFailure("muteAccount")
        return result {
            warpnet.muteUser(muterId = active.accountId, muteeId = accountId)
            warpnet.relationshipFor(accountId).copy(muting = true)
        }
    }

    suspend fun unmuteAccount(accountId: String): NetworkResult<Relationship> {
        val active = accountManager.activeAccount ?: return stubFailure("unmuteAccount")
        return result {
            warpnet.unmuteUser(muterId = active.accountId, muteeId = accountId)
            warpnet.relationshipFor(accountId).copy(muting = false)
        }
    }

    // Publish a moderation report to the network's reports topic. [type]
    // mirrors the fat node's ModerationObjectType (0 = user, 1 = tweet);
    // [reason] is free-form text capped at 256 chars by the backend.
    suspend fun reportContent(
        type: Int,
        objectId: String,
        targetUserId: String,
        targetNodeId: String,
        reason: String,
    ): NetworkResult<Unit> = result {
        warpnet.reportContent(
            type = type,
            objectId = objectId,
            targetUserId = targetUserId,
            targetNodeId = targetNodeId,
            reason = reason,
        )
    }

    suspend fun relationships(accountIds: List<String>): NetworkResult<List<Relationship>> = result {
        accountIds.map { warpnet.relationshipFor(it) }
    }

    suspend fun subscribeAccount(accountId: String): NetworkResult<Relationship> {
        val active = accountManager.activeAccount ?: return stubFailure("subscribeAccount")
        return result {
            warpnet.subscribeUser(selfId = active.accountId, targetId = accountId)
            warpnet.relationshipFor(accountId).copy(subscribing = true)
        }
    }

    suspend fun unsubscribeAccount(accountId: String): NetworkResult<Relationship> {
        val active = accountManager.activeAccount ?: return stubFailure("unsubscribeAccount")
        return result {
            warpnet.unsubscribeUser(selfId = active.accountId, targetId = accountId)
            warpnet.relationshipFor(accountId).copy(subscribing = false)
        }
    }

    suspend fun blocks(maxId: String? = null): Response<List<TimelineUser>> {
        val active = accountManager.activeAccount ?: return stubList()
        return response {
            val (ids, _) = warpnet.getBlocks(userId = active.accountId, cursor = maxId.orEmpty())
            ids.mapNotNull { id -> runCatching { warpnet.getTimelineUser(id) }.getOrNull() }
        }
    }

    suspend fun mutes(maxId: String? = null): Response<List<TimelineUser>> {
        val active = accountManager.activeAccount ?: return stubList()
        return response {
            val (ids, _) = warpnet.getMutes(userId = active.accountId, cursor = maxId.orEmpty())
            ids.mapNotNull { id -> runCatching { warpnet.getTimelineUser(id) }.getOrNull() }
        }
    }


    suspend fun likes(
        maxId: String?,
        minId: String? = null,
        sinceId: String?,
        limit: Int?,
    ): Response<List<Tweet>> = stubList()

    suspend fun bookmarks(
        maxId: String?,
        minId: String? = null,
        sinceId: String?,
        limit: Int?,
    ): Response<List<Tweet>> {
        val active = accountManager.activeAccount ?: return stubList()
        return paginated {
            warpnet.getBookmarks(
                userId = active.accountId,
                cursor = maxId.orEmpty(),
                limit = limit ?: 40,
            )
        }
    }

    suspend fun followRequests(maxId: String?): Response<List<TimelineUser>> {
        val active = accountManager.activeAccount ?: return stubList()
        return paginated {
            warpnet.getFollowRequests(
                userId = active.accountId,
                cursor = maxId.orEmpty(),
            )
        }
    }

    suspend fun authorizeFollowRequest(accountId: String): NetworkResult<Relationship> {
        val active = accountManager.activeAccount ?: return stubFailure("authorizeFollowRequest")
        return result {
            warpnet.authorizeFollowRequest(userId = active.accountId, followerId = accountId)
            warpnet.relationshipFor(accountId).copy(followedBy = true)
        }
    }

    suspend fun rejectFollowRequest(accountId: String): NetworkResult<Relationship> {
        val active = accountManager.activeAccount ?: return stubFailure("rejectFollowRequest")
        return result {
            warpnet.rejectFollowRequest(userId = active.accountId, followerId = accountId)
            warpnet.relationshipFor(accountId)
        }
    }

    // ---------------------------------------------------------------
    // polls, announcements, reports, search
    // ---------------------------------------------------------------


    suspend fun search(
        query: String?,
        type: String? = null,
        resolve: Boolean? = null,
        limit: Int? = null,
        offset: Int? = null,
        following: Boolean? = null,
    ): NetworkResult<SearchResult> = stubFailure("search")

    // ---------------------------------------------------------------
    // quotes — backend route was folded into retweet-with-comment;
    // these are kept as stubs only to satisfy the warpdroid UI surface
    // that still calls them. Quote-as-retweet runs through the normal
    // retweet path with a non-empty comment.
    // ---------------------------------------------------------------

    suspend fun quotingStatuses(
        statusId: String,
        limit: Int? = null,
        offset: String? = null,
    ): Response<List<Tweet>> = stubList()

    suspend fun removeQuote(
        id: String,
        quotingStatusId: String,
    ): NetworkResult<Tweet> = stubFailure("removeQuote")
}
