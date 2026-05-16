package site.warpnet.warpdroid.appstore

import site.warpnet.warpdroid.entity.Account
import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.Tweet

data class TweetChangedEvent(val status: Tweet) : Event
data class UnfollowEvent(val accountId: String) : Event
data class BlockEvent(val accountId: String) : Event
data class MuteEvent(val accountId: String) : Event
data class TweetDeletedEvent(val statusId: String) : Event
data class TweetComposedEvent(val status: Tweet) : Event
data class TweetScheduledEvent(val scheduledStatusId: String) : Event
data class ProfileEditedEvent(val newProfileData: Account) : Event
data class PreferenceChangedEvent(val preferenceKey: String) : Event
data class FilterUpdatedEvent(val filterContext: List<Filter.Kind>) : Event
data class NewNotificationsEvent(
    val accountId: String,
    val notifications: List<Notification>
) : Event
data class ConversationsLoadingEvent(val accountId: String) : Event
data class NotificationsLoadingEvent(val accountId: String) : Event
