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

package site.warpnet.warpdroid.components.notifications.requests.details

import android.content.Context
import android.content.Intent
import android.os.Bundle
import androidx.activity.viewModels
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat.Type.systemBars
import androidx.core.view.updatePadding
import androidx.lifecycle.lifecycleScope
import site.warpnet.warpdroid.BottomSheetActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ActivityNotificationRequestDetailsBinding
import site.warpnet.warpdroid.entity.Emoji
import site.warpnet.warpdroid.settings.PrefKeys
import site.warpnet.warpdroid.util.emojify
import site.warpnet.warpdroid.util.getParcelableArrayListExtraCompat
import site.warpnet.warpdroid.util.viewBinding
import dagger.hilt.android.AndroidEntryPoint
import dagger.hilt.android.lifecycle.withCreationCallback
import kotlin.getValue
import kotlinx.coroutines.launch

@AndroidEntryPoint
class NotificationRequestDetailsActivity : BottomSheetActivity() {

    private val viewModel: NotificationRequestDetailsViewModel by viewModels(
        extrasProducer = {
            defaultViewModelCreationExtras.withCreationCallback<NotificationRequestDetailsViewModel.Factory> { factory ->
                factory.create(
                    notificationRequestId = intent.getStringExtra(EXTRA_NOTIFICATION_REQUEST_ID)!!,
                    accountId = intent.getStringExtra(EXTRA_ACCOUNT_ID)!!
                )
            }
        }
    )

    private val binding by viewBinding(ActivityNotificationRequestDetailsBinding::inflate)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        setContentView(binding.root)

        setSupportActionBar(binding.includedToolbar.toolbar)

        val animateEmojis = preferences.getBoolean(PrefKeys.ANIMATE_CUSTOM_EMOJIS, false)

        val emojis: List<Emoji> = intent.getParcelableArrayListExtraCompat(EXTRA_ACCOUNT_EMOJIS)!!

        val title = getString(R.string.notifications_from, intent.getStringExtra(EXTRA_ACCOUNT_NAME))
            .emojify(emojis, binding.includedToolbar.toolbar, animateEmojis)

        supportActionBar?.run {
            setTitle(title)
            setDisplayHomeAsUpEnabled(true)
            setDisplayShowHomeEnabled(true)
        }

        ViewCompat.setOnApplyWindowInsetsListener(binding.bottomBar) { view, insets ->
            val bottomInsets = insets.getInsets(systemBars()).bottom
            view.updatePadding(bottom = bottomInsets)
            insets.inset(0, 0, 0, bottomInsets)
        }

        lifecycleScope.launch {
            viewModel.finish.collect { finishMode ->
                setResult(
                    RESULT_OK,
                    Intent().apply { putExtra(EXTRA_NOTIFICATION_REQUEST_ID, intent.getStringExtra(EXTRA_NOTIFICATION_REQUEST_ID)!!) }
                )
                finish()
            }
        }

        binding.acceptButton.setOnClickListener {
            viewModel.acceptNotificationRequest()
        }
        binding.dismissButton.setOnClickListener {
            viewModel.dismissNotificationRequest()
        }
    }

    companion object {
        const val EXTRA_NOTIFICATION_REQUEST_ID = "notificationRequestId"
        private const val EXTRA_ACCOUNT_ID = "accountId"
        private const val EXTRA_ACCOUNT_NAME = "accountName"
        private const val EXTRA_ACCOUNT_EMOJIS = "accountEmojis"
        fun newIntent(
            notificationRequestId: String,
            accountId: String,
            accountName: String,
            accountEmojis: List<Emoji>,
            context: Context
        ) = Intent(context, NotificationRequestDetailsActivity::class.java).apply {
            putExtra(EXTRA_NOTIFICATION_REQUEST_ID, notificationRequestId)
            putExtra(EXTRA_ACCOUNT_ID, accountId)
            putExtra(EXTRA_ACCOUNT_NAME, accountName)
            putExtra(EXTRA_ACCOUNT_EMOJIS, ArrayList(accountEmojis))
        }
    }
}
