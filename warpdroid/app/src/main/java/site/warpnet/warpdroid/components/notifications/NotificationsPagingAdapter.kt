/* Copyright 2024 Warpdroid Contributors
 *
 * This file is a part of Warpdroid.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Warpdroid is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Warpdroid; if not,
 * see <http://www.gnu.org/licenses>. */

package site.warpnet.warpdroid.components.notifications

import android.view.LayoutInflater
import android.view.ViewGroup
import androidx.compose.runtime.getValue
import androidx.compose.ui.platform.ComposeView
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.findViewTreeViewModelStoreOwner
import androidx.lifecycle.setViewTreeViewModelStoreOwner
import androidx.paging.PagingDataAdapter
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.RecyclerView
import site.warpnet.warpdroid.adapter.FilteredNotificationViewHolder
import site.warpnet.warpdroid.adapter.FollowRequestViewHolder
import site.warpnet.warpdroid.adapter.LoadMoreViewHolder
import site.warpnet.warpdroid.adapter.PlaceholderViewHolder
import site.warpnet.warpdroid.databinding.ItemFollowBinding
import site.warpnet.warpdroid.databinding.ItemFollowRequestBinding
import site.warpnet.warpdroid.databinding.ItemLoadMoreBinding
import site.warpnet.warpdroid.databinding.ItemModerationWarningNotificationBinding
import site.warpnet.warpdroid.databinding.ItemPlaceholderBinding
import site.warpnet.warpdroid.databinding.ItemReportNotificationBinding
import site.warpnet.warpdroid.databinding.ItemSeveredRelationshipNotificationBinding
import site.warpnet.warpdroid.databinding.ItemTweetFilteredBinding
import site.warpnet.warpdroid.databinding.ItemUnknownNotificationBinding
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.interfaces.AccountActionListener
import site.warpnet.warpdroid.interfaces.LoadMoreActionListener
import site.warpnet.warpdroid.interfaces.TweetActionListener
import site.warpnet.warpdroid.ui.WarpdroidTheme
import site.warpnet.warpdroid.ui.tweetcomponents.NotificationInfo
import site.warpnet.warpdroid.ui.tweetcomponents.TweetCard
import site.warpnet.warpdroid.ui.tweetcomponents.TweetNotification
import site.warpnet.warpdroid.util.TweetDisplayOptions
import site.warpnet.warpdroid.viewdata.NotificationViewData

interface NotificationActionListener {
    fun onViewReport(reportId: String)
}

interface NotificationsViewHolder {
    fun bind(
        viewData: NotificationViewData.Concrete,
        payloads: List<*>,
        statusDisplayOptions: TweetDisplayOptions
    )
}

class NotificationsPagingAdapter(
    private var statusDisplayOptions: TweetDisplayOptions,
    private val statusListener: TweetActionListener,
    private val loadMoreListener: LoadMoreActionListener<NotificationViewData.LoadMore>,
    private val notificationActionListener: NotificationActionListener,
    private val accountActionListener: AccountActionListener,
    private val instanceName: String,
    private val accountManager: AccountManager
) : PagingDataAdapter<NotificationViewData, RecyclerView.ViewHolder>(NotificationsDifferCallback) {

    var mediaPreviewEnabled: Boolean
        get() = statusDisplayOptions.mediaPreviewEnabled
        set(mediaPreviewEnabled) {
            statusDisplayOptions = statusDisplayOptions.copy(
                mediaPreviewEnabled = mediaPreviewEnabled
            )
            notifyItemRangeChanged(0, itemCount)
        }

    init {
        stateRestorationPolicy = StateRestorationPolicy.PREVENT_WHEN_EMPTY
    }

    override fun getItemViewType(position: Int): Int {
        return when (val notification = getItem(position)) {
            is NotificationViewData.LoadMore -> VIEW_TYPE_LOAD_MORE
            is NotificationViewData.Concrete -> {
                when (notification.type) {
                    Notification.Type.Mention,
                    Notification.Type.Status,
                    Notification.Type.Update,
                    Notification.Type.Quote,
                    Notification.Type.QuotedUpdate -> if (notification.statusViewData?.isFilterWarn == true) {
                        VIEW_TYPE_STATUS_FILTERED
                    } else {
                        VIEW_TYPE_STATUS
                    }
                    Notification.Type.PleromaEmojiReaction,
                    Notification.Type.Like,
                    Notification.Type.Retweet -> VIEW_TYPE_STATUS_NOTIFICATION
                    Notification.Type.Follow,
                    Notification.Type.SignUp -> VIEW_TYPE_FOLLOW
                    Notification.Type.FollowRequest -> VIEW_TYPE_FOLLOW_REQUEST
                    Notification.Type.Report -> VIEW_TYPE_REPORT
                    Notification.Type.SeveredRelationship -> VIEW_TYPE_SEVERED_RELATIONSHIP
                    Notification.Type.ModerationWarning -> VIEW_TYPE_MODERATION_WARNING
                    else -> VIEW_TYPE_UNKNOWN
                }
            }
            null -> VIEW_TYPE_PLACEHOLDER
        }
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RecyclerView.ViewHolder {
        val inflater = LayoutInflater.from(parent.context)
        return when (viewType) {
            VIEW_TYPE_PLACEHOLDER -> PlaceholderViewHolder(
                ItemPlaceholderBinding.inflate(inflater, parent, false),
                mode = PlaceholderViewHolder.Mode.NOTIFICATION
            )
            VIEW_TYPE_STATUS -> ComposeViewHolder(
                ComposeView(parent.context).apply {
                    setViewTreeViewModelStoreOwner(parent.findViewTreeViewModelStoreOwner())
                }
            )
            VIEW_TYPE_STATUS_FILTERED -> FilteredNotificationViewHolder(
                ItemTweetFilteredBinding.inflate(inflater, parent, false),
                statusListener
            )
            VIEW_TYPE_STATUS_NOTIFICATION -> ComposeViewHolder(
                ComposeView(parent.context).apply {
                    setViewTreeViewModelStoreOwner(parent.findViewTreeViewModelStoreOwner())
                }
            )
            VIEW_TYPE_FOLLOW -> FollowViewHolder(
                ItemFollowBinding.inflate(inflater, parent, false),
                accountActionListener,
                statusListener
            )
            VIEW_TYPE_FOLLOW_REQUEST -> FollowRequestViewHolder(
                ItemFollowRequestBinding.inflate(inflater, parent, false),
                accountActionListener,
                statusListener,
                true
            )
            VIEW_TYPE_LOAD_MORE -> LoadMoreViewHolder(
                ItemLoadMoreBinding.inflate(inflater, parent, false),
                loadMoreListener
            )
            VIEW_TYPE_REPORT -> ReportNotificationViewHolder(
                ItemReportNotificationBinding.inflate(inflater, parent, false),
                notificationActionListener,
                accountActionListener
            )
            VIEW_TYPE_SEVERED_RELATIONSHIP -> SeveredRelationshipNotificationViewHolder(
                ItemSeveredRelationshipNotificationBinding.inflate(inflater, parent, false),
                instanceName
            )
            VIEW_TYPE_MODERATION_WARNING -> ModerationWarningViewHolder(
                ItemModerationWarningNotificationBinding.inflate(inflater, parent, false),
                instanceName
            )
            else -> UnknownNotificationViewHolder(
                ItemUnknownNotificationBinding.inflate(inflater, parent, false)
            )
        }
    }

    override fun onBindViewHolder(viewHolder: RecyclerView.ViewHolder, position: Int) {
        onBindViewHolder(viewHolder, position, emptyList())
    }

    override fun onBindViewHolder(viewHolder: RecyclerView.ViewHolder, position: Int, payloads: List<Any>) {
        getItem(position)?.let { notification ->
            when (notification) {
                is NotificationViewData.Concrete -> {
                    val viewType = getItemViewType(position)
                    when (viewType) {
                        VIEW_TYPE_STATUS -> {
                            (viewHolder as ComposeViewHolder).composeView.setContent {
                                WarpdroidTheme {
                                    // unfortunately servers sometimes send null statuses for notification types where they should not
                                    if (notification.statusViewData != null) {
                                        val accounts by accountManager.accountsFlow.collectAsStateWithLifecycle()
                                        TweetCard(
                                            notification.statusViewData,
                                            statusListener,
                                            statusInfo = {
                                                NotificationInfo(
                                                    notificationViewData = notification,
                                                    listener = statusListener
                                                )
                                            },
                                            accounts = accounts,
                                            showDivider = false
                                        )
                                    }
                                }
                            }
                        }
                        VIEW_TYPE_STATUS_NOTIFICATION -> {
                            (viewHolder as ComposeViewHolder).composeView.setContent {
                                WarpdroidTheme {
                                    TweetNotification(
                                        notificationViewData = notification,
                                        listener = statusListener
                                    )
                                }
                            }
                        }
                        else -> (viewHolder as NotificationsViewHolder).bind(notification, payloads, statusDisplayOptions)
                    }
                }
                is NotificationViewData.LoadMore -> {
                    (viewHolder as LoadMoreViewHolder<NotificationViewData.LoadMore>).setup(notification)
                }
            }
        }
    }

    companion object {
        private const val VIEW_TYPE_PLACEHOLDER = 0
        private const val VIEW_TYPE_STATUS = 1
        private const val VIEW_TYPE_STATUS_FILTERED = 2
        private const val VIEW_TYPE_STATUS_NOTIFICATION = 3
        private const val VIEW_TYPE_FOLLOW = 4
        private const val VIEW_TYPE_FOLLOW_REQUEST = 5
        private const val VIEW_TYPE_LOAD_MORE = 6
        private const val VIEW_TYPE_REPORT = 7
        private const val VIEW_TYPE_SEVERED_RELATIONSHIP = 8
        private const val VIEW_TYPE_MODERATION_WARNING = 9
        private const val VIEW_TYPE_UNKNOWN = 10

        val NotificationsDifferCallback = object : DiffUtil.ItemCallback<NotificationViewData>() {
            override fun areItemsTheSame(
                oldItem: NotificationViewData,
                newItem: NotificationViewData
            ): Boolean {
                return oldItem.id == newItem.id
            }

            override fun areContentsTheSame(
                oldItem: NotificationViewData,
                newItem: NotificationViewData
            ): Boolean {
                return oldItem == newItem
            }
        }
    }

    class ComposeViewHolder(
        val composeView: ComposeView
    ) : RecyclerView.ViewHolder(composeView)
}
