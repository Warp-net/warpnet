package site.warpnet.warpdroid.components.account

import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import at.connyduck.calladapter.networkresult.fold
import site.warpnet.warpdroid.appstore.BlockEvent
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.MuteEvent
import site.warpnet.warpdroid.appstore.ProfileEditedEvent
import site.warpnet.warpdroid.appstore.UnfollowEvent
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.User
import site.warpnet.warpdroid.entity.Relationship
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.util.Error
import site.warpnet.warpdroid.util.Loading
import site.warpnet.warpdroid.util.Resource
import site.warpnet.warpdroid.util.Success
import site.warpnet.warpdroid.util.getDomain
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

@HiltViewModel
class AccountViewModel @Inject constructor(
    private val warpnetApi: WarpnetApi,
    private val eventHub: EventHub,
    accountManager: AccountManager
) : ViewModel() {

    private val _accountData = MutableStateFlow(null as Resource<User>?)
    val accountData: StateFlow<Resource<User>?> = _accountData.asStateFlow()

    private val _relationshipData = MutableStateFlow(null as Resource<Relationship>?)
    val relationshipData: StateFlow<Resource<Relationship>?> = _relationshipData.asStateFlow()


    private val _isRefreshing = MutableStateFlow(false)
    val isRefreshing: StateFlow<Boolean> = _isRefreshing.asStateFlow()

    lateinit var accountId: String
    var isSelf = false

    /** the domain of the viewed account **/
    var domain = ""

    /** True if the viewed account has the same domain as the active account */
    var isFromOwnDomain = false

    private val activeAccount = accountManager.activeAccount!!

    init {
        viewModelScope.launch {
            eventHub.events.collect { event ->
                if (event is ProfileEditedEvent && event.newProfileData.id == _accountData.value?.data?.id) {
                    _accountData.value = Success(event.newProfileData)
                }
            }
        }
    }

    private fun obtainAccount(reload: Boolean = false) {
        if (_accountData.value == null || reload) {
            if (reload) {
                _isRefreshing.value = true
            }
            _accountData.value = Loading()

            viewModelScope.launch {
                warpnetApi.account(accountId)
                    .fold(
                        { account ->
                            domain = getDomain(account.url)
                            isFromOwnDomain = domain == activeAccount.domain

                            _accountData.value = Success(account)
                            _isRefreshing.value = false
                        },
                        { t ->
                            Log.w(TAG, "failed obtaining account", t)
                            _accountData.value = Error(cause = t)
                            _isRefreshing.value = false
                        }
                    )
            }
        }
    }

    private fun obtainRelationship(reload: Boolean = false) {
        if (_relationshipData.value == null || reload) {
            _relationshipData.value = Loading()

            viewModelScope.launch {
                warpnetApi.relationships(listOf(accountId))
                    .fold(
                        { relationships ->
                            _relationshipData.value =
                                if (relationships.isNotEmpty()) {
                                    Success(
                                        relationships[0]
                                    )
                                } else {
                                    Error()
                                }
                        },
                        { t ->
                            Log.w(TAG, "failed obtaining relationships", t)
                            _relationshipData.value = Error(cause = t)
                        }
                    )
            }
        }
    }

    fun changeFollowState() {
        val relationship = _relationshipData.value?.data
        if (relationship?.following == true || relationship?.requested == true) {
            changeRelationship(RelationShipAction.UNFOLLOW)
        } else {
            changeRelationship(RelationShipAction.FOLLOW)
        }
    }

    fun changeBlockState() {
        if (_relationshipData.value?.data?.blocking == true) {
            changeRelationship(RelationShipAction.UNBLOCK)
        } else {
            changeRelationship(RelationShipAction.BLOCK)
        }
    }

    fun muteAccount(notifications: Boolean, duration: Int?) {
        changeRelationship(RelationShipAction.MUTE, notifications, duration)
    }

    fun unmuteAccount() {
        changeRelationship(RelationShipAction.UNMUTE)
    }

    fun changeSubscribingState() {
        val relationship = _relationshipData.value?.data
        if (relationship?.notifying == true ||
            // Warpnet 3.3.0rc1
            relationship?.subscribing == true // Pleroma
        ) {
            changeRelationship(RelationShipAction.UNSUBSCRIBE)
        } else {
            changeRelationship(RelationShipAction.SUBSCRIBE)
        }
    }

    fun changeShowRetweetsState() {
        if (_relationshipData.value?.data?.showingRetweets == true) {
            changeRelationship(RelationShipAction.FOLLOW, false)
        } else {
            changeRelationship(RelationShipAction.FOLLOW, true)
        }
    }

    /**
     * @param parameter showRetweets if RelationShipAction.FOLLOW, notifications if MUTE
     */
    private fun changeRelationship(
        relationshipAction: RelationShipAction,
        parameter: Boolean? = null,
        duration: Int? = null
    ) = viewModelScope.launch {
        val relation = _relationshipData.value?.data
        val account = _accountData.value?.data
        val isWarpnet = _relationshipData.value?.data?.notifying != null

        if (relation != null && account != null) {
            // optimistically post new state for faster response

            val newRelation = when (relationshipAction) {
                RelationShipAction.FOLLOW -> {
                    if (account.locked) {
                        relation.copy(requested = true)
                    } else {
                        relation.copy(following = true)
                    }
                }
                RelationShipAction.UNFOLLOW -> relation.copy(following = false)
                RelationShipAction.BLOCK -> relation.copy(blocking = true)
                RelationShipAction.UNBLOCK -> relation.copy(blocking = false)
                RelationShipAction.MUTE -> relation.copy(muting = true)
                RelationShipAction.UNMUTE -> relation.copy(muting = false)
                RelationShipAction.SUBSCRIBE -> {
                    if (isWarpnet) {
                        relation.copy(notifying = true)
                    } else {
                        relation.copy(subscribing = true)
                    }
                }
                RelationShipAction.UNSUBSCRIBE -> {
                    if (isWarpnet) {
                        relation.copy(notifying = false)
                    } else {
                        relation.copy(subscribing = false)
                    }
                }
            }
            _relationshipData.value = Loading(newRelation)
        }

        val relationshipCall = when (relationshipAction) {
            RelationShipAction.FOLLOW -> warpnetApi.followAccount(
                accountId,
                showRetweets = parameter ?: true
            )
            RelationShipAction.UNFOLLOW -> warpnetApi.unfollowAccount(accountId)
            RelationShipAction.BLOCK -> warpnetApi.blockAccount(accountId)
            RelationShipAction.UNBLOCK -> warpnetApi.unblockAccount(accountId)
            RelationShipAction.MUTE -> warpnetApi.muteAccount(
                accountId,
                parameter ?: true,
                duration
            )
            RelationShipAction.UNMUTE -> warpnetApi.unmuteAccount(accountId)
            RelationShipAction.SUBSCRIBE -> {
                if (isWarpnet) {
                    warpnetApi.followAccount(accountId, notify = true)
                } else {
                    warpnetApi.subscribeAccount(accountId)
                }
            }
            RelationShipAction.UNSUBSCRIBE -> {
                if (isWarpnet) {
                    warpnetApi.followAccount(accountId, notify = false)
                } else {
                    warpnetApi.unsubscribeAccount(accountId)
                }
            }
        }

        relationshipCall.fold(
            { relationship ->
                _relationshipData.value = Success(relationship)

                when (relationshipAction) {
                    RelationShipAction.UNFOLLOW -> eventHub.dispatch(UnfollowEvent(accountId))
                    RelationShipAction.BLOCK -> eventHub.dispatch(BlockEvent(accountId))
                    RelationShipAction.MUTE -> eventHub.dispatch(MuteEvent(accountId))
                    else -> { }
                }
            },
            { t ->
                Log.w(TAG, "failed loading relationship", t)
                _relationshipData.value = Error(relation, cause = t)
            }
        )
    }

    fun refresh() {
        reload(true)
    }

    private fun reload(isReload: Boolean = false) {
        if (_isRefreshing.value) {
            return
        }
        accountId.let {
            obtainAccount(isReload)
            if (!isSelf) {
                obtainRelationship(isReload)
            }
        }
    }

    fun setAccountInfo(accountId: String) {
        this.accountId = accountId
        this.isSelf = activeAccount.accountId == accountId
        reload(false)
    }

    enum class RelationShipAction {
        FOLLOW,
        UNFOLLOW,
        BLOCK,
        UNBLOCK,
        MUTE,
        UNMUTE,
        SUBSCRIBE,
        UNSUBSCRIBE
    }

    companion object {
        const val TAG = "AccountViewModel"
    }
}
