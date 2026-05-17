package site.warpnet.warpdroid.util

import android.content.Context
import site.warpnet.warpdroid.TweetListActivity
import site.warpnet.warpdroid.ViewMediaActivity
import site.warpnet.warpdroid.components.account.AccountActivity
import site.warpnet.warpdroid.components.accountlist.AccountListActivity
import site.warpnet.warpdroid.components.compose.ComposeActivity
import site.warpnet.warpdroid.components.compose.ComposeActivity.ComposeOptions
import site.warpnet.warpdroid.components.viewthread.ViewThreadActivity
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.viewdata.AttachmentViewData
import site.warpnet.warpdroid.viewdata.TweetViewData
import kotlin.collections.map

fun Context.viewThread(viewData: TweetViewData.Concrete) {
    // the url of a actionable status is never null
    val intent = ViewThreadActivity.newIntent(
        this,
        viewData.actionableId,
        viewData.actionable.url!!,
        viewData.actionable.account.id,
    )
    startActivityWithSlideInAnimation(intent)
}

/**
 * Warpnet has no hashtag timeline (§5 B2 Tier B item without backend),
 * so any onViewTag arriving here is a deliberate no-op. Hashtag spans
 * still render in tweet text — they just aren't navigable.
 */
@Suppress("UnusedReceiverParameter", "UNUSED_PARAMETER")
fun Context.viewTag(tag: String) = Unit

fun Context.viewAccount(accountId: String) {
    val intent = AccountActivity.newIntent(this, accountId)
    startActivityWithSlideInAnimation(intent)
}

fun Context.reply(viewData: TweetViewData.Concrete, activeAccount: AccountEntity) {
    val actionableStatus = viewData.actionable

    val mentionedUsernames = buildSet {
        add(actionableStatus.account.username)
        addAll(
            actionableStatus.mentions
                .map { it.username }
        )
        remove(activeAccount.username)
    }

    val intent = ComposeActivity.newIntent(
        this,
        ComposeOptions(
            inReplyToId = actionableStatus.id,
            replyVisibility = actionableStatus.visibility,
            contentWarning = actionableStatus.spoilerText,
            mentionedUsernames = mentionedUsernames,
            replyingStatusAuthor = actionableStatus.account.localUsername,
            replyingTweetContent = actionableStatus.content.parseAsWarpnetHtml().toString(),
            language = actionableStatus.language,
            kind = ComposeActivity.ComposeKind.NEW
        )
    )
    startActivityWithSlideInAnimation(intent)
}

fun Context.viewMedia(index: Int, attachments: List<AttachmentViewData>) {
    val (attachment) = attachments[index]
    when (attachment.type) {
        Attachment.Type.GIFV, Attachment.Type.VIDEO, Attachment.Type.IMAGE, Attachment.Type.AUDIO -> {
            val intent = ViewMediaActivity.newIntent(this, attachments, index)
            startActivity(intent)
        }

        Attachment.Type.UNKNOWN -> {
            openLink(attachment.unknownUrl)
        }
    }
}

fun Context.showFavs(viewData: TweetViewData.Concrete) {
    val intent = AccountListActivity.newIntent(this, AccountListActivity.Type.LIKED, viewData.actionableId)
    startActivityWithSlideInAnimation(intent)
}

fun Context.showRetweets(viewData: TweetViewData.Concrete) {
    val intent = AccountListActivity.newIntent(this, AccountListActivity.Type.RETWEETED, viewData.actionableId)
    startActivityWithSlideInAnimation(intent)
}

fun Context.showQuotes(viewData: TweetViewData.Concrete) {
    val intent = TweetListActivity.newQuotesIntent(this, viewData.actionableId)
    startActivityWithSlideInAnimation(intent)
}
