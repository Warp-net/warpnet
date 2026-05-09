/* Copyright 2021 Warpdroid Contributors
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

import android.content.Context
import android.text.TextUtils
import androidx.recyclerview.widget.RecyclerView
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ItemReportNotificationBinding
import site.warpnet.warpdroid.interfaces.AccountActionListener
import site.warpnet.warpdroid.util.TweetDisplayOptions
import site.warpnet.warpdroid.util.getRelativeTimeSpanString
import site.warpnet.warpdroid.util.loadAvatar
import site.warpnet.warpdroid.util.unicodeWrap
import site.warpnet.warpdroid.util.updateEmojiTargets
import site.warpnet.warpdroid.viewdata.NotificationViewData

class ReportNotificationViewHolder(
    private val binding: ItemReportNotificationBinding,
    private val listener: NotificationActionListener,
    private val accountActionListener: AccountActionListener
) : RecyclerView.ViewHolder(binding.root), NotificationsViewHolder {

    override fun bind(
        viewData: NotificationViewData.Concrete,
        payloads: List<*>,
        statusDisplayOptions: TweetDisplayOptions
    ) {
        if (payloads.isNotEmpty()) {
            return
        }
        val report = viewData.report!!
        val reporter = viewData.account

        binding.notificationTopText.updateEmojiTargets {
            val reporterName = reporter.name.unicodeWrap().emojify(reporter.emojis, statusDisplayOptions.animateEmojis)
            val reporteeName = report.targetAccount.name.unicodeWrap().emojify(report.targetAccount.emojis, statusDisplayOptions.animateEmojis)

            // Context.getString() returns a String and doesn't support Spannable.
            // Convert the placeholders to the format used by TextUtils.expandTemplate which does.
            val topText =
                view.context.getString(R.string.notification_header_report_format, "^1", "^2")
            view.text = TextUtils.expandTemplate(topText, reporterName, reporteeName)
        }
        binding.notificationSummary.text = itemView.context.getString(
            R.string.notification_summary_report_format,
            getRelativeTimeSpanString(itemView.context, report.createdAt.time, System.currentTimeMillis()),
            report.statusIds?.size ?: 0
        )
        binding.notificationCategory.text = getTranslatedCategory(itemView.context, report.category)

        loadAvatar(
            report.targetAccount.avatar,
            binding.notificationReporteeAvatar,
            itemView.context.resources.getDimensionPixelSize(R.dimen.avatar_radius_36dp),
            statusDisplayOptions.animateAvatars,
        )
        loadAvatar(
            reporter.avatar,
            binding.notificationReporterAvatar,
            itemView.context.resources.getDimensionPixelSize(R.dimen.avatar_radius_24dp),
            statusDisplayOptions.animateAvatars,
        )

        binding.notificationReporteeAvatar.setOnClickListener {
            accountActionListener.onViewAccount(report.targetAccount.id)
        }
        binding.notificationReporterAvatar.setOnClickListener {
            accountActionListener.onViewAccount(reporter.id)
        }

        itemView.setOnClickListener { listener.onViewReport(report.id) }
    }

    private fun getTranslatedCategory(context: Context, rawCategory: String): String {
        return when (rawCategory) {
            "violation" -> context.getString(R.string.report_category_violation)
            "spam" -> context.getString(R.string.report_category_spam)
            "legal" -> context.getString(R.string.report_category_legal)
            "other" -> context.getString(R.string.report_category_other)
            else -> rawCategory
        }
    }
}
