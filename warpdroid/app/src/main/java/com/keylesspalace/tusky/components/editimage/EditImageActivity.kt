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
import android.os.Bundle
import android.view.Menu
import android.view.MenuInflater
import android.view.MenuItem
import android.view.ViewGroup
import androidx.appcompat.R as appcompatR
import androidx.core.content.IntentCompat
import androidx.core.view.MenuProvider
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsCompat.Type.displayCutout
import androidx.core.view.WindowInsetsCompat.Type.statusBars
import androidx.core.view.updateLayoutParams
import com.canhub.cropper.CropImageOptions
import com.canhub.cropper.CropImageView
import com.canhub.cropper.CropImageView.CropResult
import com.google.android.material.color.MaterialColors
import com.keylesspalace.tusky.BaseActivity
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.databinding.ActivityEditImageBinding
import dagger.hilt.android.AndroidEntryPoint

/**
 * Activity for rotating, flipping and cropping images.
 * Use [EditImageContract] to launch it.
 */
@AndroidEntryPoint
class EditImageActivity :
    BaseActivity(),
    MenuProvider,
    CropImageView.OnCropImageCompleteListener {

    private lateinit var binding: ActivityEditImageBinding

    private lateinit var options: EditImageOptions

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        binding = ActivityEditImageBinding.inflate(layoutInflater)
        setContentView(binding.root)

        setSupportActionBar(binding.includedToolbar.toolbar)
        supportActionBar?.apply {
            setDisplayShowTitleEnabled(false)
            setDisplayHomeAsUpEnabled(true)
            setDisplayShowHomeEnabled(true)
        }

        addMenuProvider(this)

        options = requireNotNull(IntentCompat.getParcelableExtra(intent, EDIT_IMAGE_EXTRA_OPTIONS, EditImageOptions::class.java))

        binding.cropImageView.setImageCropOptions(
            CropImageOptions(
                progressBarColor = MaterialColors.getColor(binding.root, appcompatR.attr.colorPrimary),
                initialCropWindowPaddingRatio = 0.075f,
            )
        )

        if (options.requiredWidth > 0 && options.requiredHeight > 0) {
            binding.cropImageView.setAspectRatio(options.requiredWidth, options.requiredHeight)
        }

        if (savedInstanceState == null) {
            binding.cropImageView.setImageUriAsync(options.input)
        }

        binding.cropImageView.setOnCropImageCompleteListener(this)

        ViewCompat.setOnApplyWindowInsetsListener(binding.cropImageView) { view, insets ->
            val safeDrawingInsets = insets.getInsets(statusBars() or displayCutout())
            view.updateLayoutParams<ViewGroup.MarginLayoutParams> {
                // top is handled by the toolbar
                leftMargin = safeDrawingInsets.left
                rightMargin = safeDrawingInsets.right
                bottomMargin = safeDrawingInsets.bottom
            }
            WindowInsetsCompat.CONSUMED
        }
    }

    override fun onCreateMenu(menu: Menu, menuInflater: MenuInflater) {
        menuInflater.inflate(R.menu.activity_edit_image, menu)
    }

    override fun onMenuItemSelected(item: MenuItem): Boolean {
        return when (item.itemId) {
            R.id.editImageDone -> {
                cropImage()
                true
            }
            R.id.editImageRotate -> {
                binding.cropImageView.rotateImage(90)
                true
            }
            R.id.editImageFlipHorizontally -> {
                binding.cropImageView.flipImageHorizontally()
                true
            }
            R.id.editImageFlipVertically -> {
                binding.cropImageView.flipImageVertically()
                true
            }
            android.R.id.home -> {
                cancel()
                true
            }
            else -> super.onOptionsItemSelected(item)
        }
    }

    override fun onCropImageComplete(view: CropImageView, result: CropResult) {
        val uri = result.uriContent

        setResult(
            if (uri != null) {
                RESULT_OK
            } else {
                RESULT_ERROR
            },
            Intent().apply {
                putExtra(
                    EDIT_IMAGE_EXTRA_RESULT,
                    if (uri != null) {
                        EditImageResult.Success(uri)
                    } else {
                        EditImageResult.Error(result.error ?: Exception("Unknown error"))
                    }
                )
            }
        )
        finish()
    }

    private fun cropImage() {
        binding.cropImageView.croppedImageAsync(
            saveCompressFormat = options.outputCompressFormat,
            reqWidth = options.requiredWidth,
            reqHeight = options.requiredHeight,
            customOutputUri = options.outputUri,
        )
    }

    private fun cancel() {
        setResult(
            RESULT_CANCELED,
            Intent().apply {
                putExtra(EDIT_IMAGE_EXTRA_RESULT, EditImageResult.Cancelled)
            }
        )
        finish()
    }

    companion object {
        const val RESULT_ERROR = 2
        const val EDIT_IMAGE_EXTRA_RESULT = "result"
        private const val EDIT_IMAGE_EXTRA_OPTIONS = "options"

        fun newIntent(context: Context, options: EditImageOptions) =
            Intent(context, EditImageActivity::class.java).apply {
                putExtra(EDIT_IMAGE_EXTRA_OPTIONS, options)
            }
    }
}
