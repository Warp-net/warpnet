/* Copyright 2025 Tusky Contributors
 *
 * This file is a part of Tusky.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Tusky is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Tusky; if not,
 * see <http://www.gnu.org/licenses>. */

package com.keylesspalace.tusky.components.editimage

import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.net.Uri
import android.os.Parcelable
import androidx.activity.result.contract.ActivityResultContract
import androidx.core.content.IntentCompat
import kotlinx.parcelize.Parcelize

class EditImageContract :
    ActivityResultContract<EditImageOptions, EditImageResult>() {

    override fun createIntent(context: Context, input: EditImageOptions): Intent {
        return EditImageActivity.newIntent(context, input)
    }

    override fun parseResult(
        resultCode: Int,
        intent: Intent?
    ): EditImageResult {
        val result = intent?.let {
            IntentCompat.getParcelableExtra(it, EditImageActivity.EDIT_IMAGE_EXTRA_RESULT, EditImageResult::class.java)
        }

        return result ?: EditImageResult.Cancelled
    }
}

sealed class EditImageResult : Parcelable {
    @Parcelize
    class Success(
        val outputUri: Uri
    ) : EditImageResult()

    @Parcelize
    class Error(
        val exception: Exception
    ) : EditImageResult()

    @Parcelize
    data object Cancelled : EditImageResult()
}

@Parcelize
class EditImageOptions(
    val input: Uri,
    val outputUri: Uri? = null,
    val requiredWidth: Int = 0,
    val requiredHeight: Int = 0,
    val outputCompressFormat: Bitmap.CompressFormat
) : Parcelable
