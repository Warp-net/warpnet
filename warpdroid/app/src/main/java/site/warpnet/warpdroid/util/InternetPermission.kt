/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

package site.warpnet.warpdroid.util

import android.Manifest
import android.app.Activity
import android.content.Context
import android.content.pm.PackageManager
import androidx.appcompat.app.AlertDialog
import androidx.core.content.ContextCompat
import site.warpnet.warpdroid.R

/**
 * INTERNET is an install-time permission, but privacy ROMs (GrapheneOS,
 * MIUI, LineageOS Privacy Guard) let the user revoke it. Warpdroid is a
 * libp2p client that has nothing to show offline, so refuse to start rather
 * than failing deep inside the transport with a socket error.
 */
object InternetPermission {

    fun isGranted(context: Context): Boolean =
        ContextCompat.checkSelfPermission(context, Manifest.permission.INTERNET) ==
            PackageManager.PERMISSION_GRANTED

    /**
     * Terminal dialog for the revoked case. The caller must stop initialising
     * but must not finish() itself — the dialog needs a live window, and
     * closing it tears the task down.
     */
    fun showRequiredDialog(activity: Activity) {
        AlertDialog.Builder(activity)
            .setTitle(R.string.warpnet_internet_required_title)
            .setMessage(R.string.warpnet_internet_required_body)
            .setCancelable(false)
            .setPositiveButton(R.string.action_close) { _, _ -> activity.finishAffinity() }
            .show()
    }
}
