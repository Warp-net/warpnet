package site.warpnet.warpdroid.components.scheduled

import android.util.Log
import androidx.paging.ExperimentalPagingApi
import androidx.paging.LoadType
import androidx.paging.PagingState
import androidx.paging.RemoteMediator
import site.warpnet.warpdroid.components.timeline.util.ifExpected
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.util.HttpHeaderLink
import retrofit2.HttpException

@OptIn(ExperimentalPagingApi::class)
class ScheduledTweetRemoteMediator(
    private val api: WarpnetApi,
    private val accountManager: AccountManager,
    private val viewModel: ScheduledTweetViewModel
) : RemoteMediator<String, ScheduledTweetViewData>() {

    override suspend fun load(
        loadType: LoadType,
        state: PagingState<String, ScheduledTweetViewData>
    ): MediatorResult {
        val activeAccount = accountManager.activeAccount
        if (activeAccount == null) {
            return MediatorResult.Success(endOfPaginationReached = true)
        }

        try {
            val statusResponse = when (loadType) {
                LoadType.REFRESH -> {
                    viewModel.scheduledStatuses.clear()
                    api.scheduledStatuses(limit = state.config.pageSize)
                }

                LoadType.PREPEND -> {
                    return MediatorResult.Success(endOfPaginationReached = true)
                }

                LoadType.APPEND -> {
                    val nextKey = state.pages.lastOrNull()?.nextKey
                    if (nextKey == null) {
                        return MediatorResult.Success(endOfPaginationReached = true)
                    }
                    api.scheduledStatuses(limit = state.config.pageSize, maxId = nextKey)
                }
            }

            val statuses = statusResponse.body()
            if (!statusResponse.isSuccessful || statuses == null) {
                return MediatorResult.Error(HttpException(statusResponse))
            }

            val data = statuses.map { status ->
                status.toViewData(
                    spoilerExpanded = status.params.spoilerText.isNullOrBlank() || activeAccount.alwaysOpenSpoiler,
                    overflowVisible = false,
                    mediaVisible = activeAccount.alwaysShowSensitiveMedia || !(status.params.sensitive ?: false)
                )
            }

            val linkHeader = statusResponse.headers()["Link"]
            val links = HttpHeaderLink.parse(linkHeader)
            val next = HttpHeaderLink.findByRelationType(links, "next")

            viewModel.scheduledStatuses.addAll(data)

            viewModel.nextKey = next?.uri?.getQueryParameter("max_id")

            viewModel.invalidate()

            return MediatorResult.Success(endOfPaginationReached = false)
        } catch (e: Exception) {
            return ifExpected(e) {
                Log.w("ScheduledStatuses", "Failed to load scheduled statuses", e)
                MediatorResult.Error(e)
            }
        }
    }
}
