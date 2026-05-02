package com.keylesspalace.tusky.util

import android.content.Context
import com.keylesspalace.tusky.StatusListActivity
import com.keylesspalace.tusky.ViewMediaActivity
import com.keylesspalace.tusky.components.account.AccountActivity
import com.keylesspalace.tusky.components.accountlist.AccountListActivity
import com.keylesspalace.tusky.components.compose.ComposeActivity
import com.keylesspalace.tusky.components.compose.ComposeActivity.ComposeOptions
import com.keylesspalace.tusky.components.report.ReportActivity
import com.keylesspalace.tusky.components.viewthread.ViewThreadActivity
import com.keylesspalace.tusky.db.entity.AccountEntity
import com.keylesspalace.tusky.entity.Attachment
import com.keylesspalace.tusky.viewdata.AttachmentViewData
import com.keylesspalace.tusky.viewdata.StatusViewData
import kotlin.collections.map

fun Context.viewThread(viewData: StatusViewData.Concrete) {
    // the url of a actionable status is never null
    val intent = ViewThreadActivity.newIntent(this, viewData.actionableId, viewData.actionable.url!!)
    startActivityWithSlideInAnimation(intent)
}

fun Context.viewTag(tag: String) {
    val intent = StatusListActivity.newHashtagIntent(this, tag)
    startActivityWithSlideInAnimation(intent)
}

fun Context.viewAccount(accountId: String) {
    val intent = AccountActivity.newIntent(this, accountId)
    startActivityWithSlideInAnimation(intent)
}

fun Context.reply(viewData: StatusViewData.Concrete, activeAccount: AccountEntity) {
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
            replyingStatusContent = actionableStatus.content.parseAsMastodonHtml().toString(),
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

fun Context.showFavs(viewData: StatusViewData.Concrete) {
    val intent = AccountListActivity.newIntent(this, AccountListActivity.Type.FAVOURITED, viewData.actionableId)
    startActivityWithSlideInAnimation(intent)
}

fun Context.showReblogs(viewData: StatusViewData.Concrete) {
    val intent = AccountListActivity.newIntent(this, AccountListActivity.Type.REBLOGGED, viewData.actionableId)
    startActivityWithSlideInAnimation(intent)
}

fun Context.showQuotes(viewData: StatusViewData.Concrete) {
    val intent = StatusListActivity.newQuotesIntent(this, viewData.actionableId)
    startActivityWithSlideInAnimation(intent)
}

fun Context.report(viewData: StatusViewData.Concrete) {
    val account = viewData.actionable.account
    val intent = ReportActivity.getIntent(this, account.id, account.username, viewData.id)
    startActivityWithSlideInAnimation(intent)
}
