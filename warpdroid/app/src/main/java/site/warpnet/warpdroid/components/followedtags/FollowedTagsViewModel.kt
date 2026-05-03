package site.warpnet.warpdroid.components.followedtags

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.paging.ExperimentalPagingApi
import androidx.paging.Pager
import androidx.paging.PagingConfig
import androidx.paging.cachedIn
import site.warpnet.warpdroid.entity.HashTag
import site.warpnet.warpdroid.network.WarpnetApi
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject

@HiltViewModel
class FollowedTagsViewModel @Inject constructor(
    val api: WarpnetApi
) : ViewModel() {
    val tags: MutableList<HashTag> = mutableListOf()
    var nextKey: String? = null
    var currentSource: FollowedTagsPagingSource? = null

    @OptIn(ExperimentalPagingApi::class)
    val pager = Pager(
        config = PagingConfig(
            pageSize = 100
        ),
        remoteMediator = FollowedTagsRemoteMediator(api, this),
        pagingSourceFactory = {
            FollowedTagsPagingSource(
                viewModel = this
            ).also { source ->
                currentSource = source
            }
        }
    ).flow.cachedIn(viewModelScope)

    companion object {
        private const val TAG = "FollowedTagsViewModel"
    }
}
