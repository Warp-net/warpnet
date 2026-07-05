/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

package site.warpnet.warpdroid.components.dontkillme

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.view.ViewGroup
import android.widget.LinearLayout
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat.Type.systemBars
import androidx.core.view.updatePadding
import com.google.android.material.button.MaterialButton
import org.briarproject.android.dontkillmelib.HuaweiUtils
import org.briarproject.android.dontkillmelib.XiaomiUtils
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.ActivityDontKillMeBinding
import timber.log.Timber

// "Keep Warpnet running" screen: uses dont-kill-me-lib to detect OEM
// app-killer settings (Huawei/Xiaomi) and deep-link the user to fix them.
// Shown once, only when isNeeded().
class DontKillMeActivity : AppCompatActivity() {

    private data class Step(
        val descriptionRes: Int,
        val buttonRes: Int?,
        val intents: List<Intent>,
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val binding = ActivityDontKillMeBinding.inflate(layoutInflater)
        setContentView(binding.root)
        setTitle(R.string.dont_kill_me_title)

        ViewCompat.setOnApplyWindowInsetsListener(binding.scrollView) { v, insets ->
            val bars = insets.getInsets(systemBars())
            v.updatePadding(top = bars.top, bottom = bars.bottom)
            insets
        }

        steps(this).forEach { step -> addStep(binding.stepContainer, step) }

        binding.doneButton.setOnClickListener { finish() }
    }

    private fun addStep(container: LinearLayout, step: Step) {
        val description = TextView(this).apply {
            setText(step.descriptionRes)
            textSize = 16f
        }
        container.addView(
            description,
            LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
            ).apply { topMargin = dp(16) },
        )

        if (step.buttonRes != null && step.intents.isNotEmpty()) {
            val button = MaterialButton(this).apply {
                setText(step.buttonRes)
                setOnClickListener { launchFirstResolvable(step.intents) }
            }
            container.addView(
                button,
                LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT,
                    ViewGroup.LayoutParams.WRAP_CONTENT,
                ).apply { topMargin = dp(8) },
            )
        }
    }

    private fun launchFirstResolvable(intents: List<Intent>) {
        for (intent in intents) {
            try {
                startActivity(intent)
                return
            } catch (e: Exception) {
                Timber.tag(TAG).w(e, "could not start OEM settings intent")
            }
        }
    }

    private fun dp(value: Int): Int =
        (value * resources.displayMetrics.density).toInt()

    companion object {
        private const val TAG = "DontKillMeActivity"

        /** True when any OEM-specific app-killer setting needs the user's attention. */
        fun isNeeded(context: Context): Boolean =
            HuaweiUtils.protectedAppsNeedsToBeShown(context) ||
                HuaweiUtils.appLaunchNeedsToBeShown(context) ||
                XiaomiUtils.xiaomiRecentAppsNeedsToBeShown() ||
                XiaomiUtils.xiaomiLockAppsNeedsToBeShown(context)

        private fun steps(context: Context): List<Step> = buildList {
            if (HuaweiUtils.protectedAppsNeedsToBeShown(context)) {
                add(
                    Step(
                        R.string.dont_kill_me_huawei_protected,
                        R.string.dont_kill_me_open_settings,
                        listOf(HuaweiUtils.huaweiProtectedAppsIntent),
                    ),
                )
            }
            if (HuaweiUtils.appLaunchNeedsToBeShown(context)) {
                add(
                    Step(
                        R.string.dont_kill_me_huawei_app_launch,
                        R.string.dont_kill_me_open_settings,
                        HuaweiUtils.huaweiAppLaunchIntents,
                    ),
                )
            }
            if (XiaomiUtils.xiaomiRecentAppsNeedsToBeShown()) {
                // Manual gesture, no settings screen — instructions only.
                add(Step(R.string.dont_kill_me_xiaomi_recents, null, emptyList()))
            }
            if (XiaomiUtils.xiaomiLockAppsNeedsToBeShown(context)) {
                add(
                    Step(
                        R.string.dont_kill_me_xiaomi_lock,
                        R.string.dont_kill_me_open_settings,
                        listOf(XiaomiUtils.xiaomiLockAppsIntent),
                    ),
                )
            }
        }
    }
}
