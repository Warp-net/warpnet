package com.keylesspalace.tusky.util

import android.content.Context
import android.content.Intent
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.viewdata.StatusViewData

fun Context.shareStatusContent(viewData: StatusViewData.Concrete) {
    val statusToShare = viewData.actionable
    val sendIntent = Intent().apply {
        action = Intent.ACTION_SEND
        type = "text/plain"
        putExtra(
            Intent.EXTRA_TEXT,
            "${statusToShare.account.username} - ${statusToShare.content.parseAsMastodonHtml()}"
        )
        putExtra(Intent.EXTRA_SUBJECT, statusToShare.url)
    }
    startActivity(
        Intent.createChooser(
            sendIntent,
            resources.getText(R.string.send_post_content_to)
        )
    )
}

fun Context.shareStatusLink(viewData: StatusViewData.Concrete) {
    val urlToShare = viewData.actionable.url
    val sendIntent = Intent().apply {
        action = Intent.ACTION_SEND
        putExtra(Intent.EXTRA_TEXT, urlToShare)
        type = "text/plain"
    }
    startActivity(
        Intent.createChooser(
            sendIntent,
            resources.getText(R.string.send_post_link_to)
        )
    )
}
