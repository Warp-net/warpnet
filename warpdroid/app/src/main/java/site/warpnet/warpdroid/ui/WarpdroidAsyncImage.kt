/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Glide-backed drop-in replacement for Coil3's AsyncImage. Coil3 ran its
 * SizeResolver during composition on the main thread and silently failed
 * on warpnet:// URLs (Glide had the ModelLoader; Coil never got one),
 * which together caused most of the transition jank on screens with
 * avatars or media. This wrapper loads the bitmap via Glide's background
 * pipeline and hands a stable Painter to a plain Compose Image — no
 * recomposition cascade, no main-thread layout work for image requests.
 */
package site.warpnet.warpdroid.ui

import android.graphics.Bitmap
import android.graphics.drawable.Drawable
import androidx.compose.foundation.Image
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.painter.BitmapPainter
import androidx.compose.ui.graphics.painter.ColorPainter
import androidx.compose.ui.graphics.painter.Painter
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import com.bumptech.glide.Glide
import com.bumptech.glide.request.target.CustomTarget
import com.bumptech.glide.request.transition.Transition

@Composable
fun WarpdroidAsyncImage(
    model: Any?,
    contentDescription: String?,
    modifier: Modifier = Modifier,
    placeholder: Painter? = null,
    error: Painter? = placeholder,
    contentScale: ContentScale = ContentScale.Fit,
    alignment: Alignment = Alignment.Center,
) {
    val context = LocalContext.current
    var bitmap by remember(model) { mutableStateOf<Bitmap?>(null) }
    var failed by remember(model) { mutableStateOf(model == null) }
    // Layout dimensions are captured once the Image lays out and reused as
    // the override() size in Glide so it can downsample large media. Until
    // the first layout pass arrives we keep SIZE_ORIGINAL so a synchronous
    // first frame stays correct on tiny placeholders (placeholder/error).
    var layoutWidth by remember(model) { mutableStateOf(0) }
    var layoutHeight by remember(model) { mutableStateOf(0) }

    DisposableEffect(model, layoutWidth, layoutHeight) {
        if (model == null || layoutWidth == 0 || layoutHeight == 0) {
            return@DisposableEffect onDispose { }
        }
        val target = object : CustomTarget<Bitmap>() {
            override fun onResourceReady(resource: Bitmap, transition: Transition<in Bitmap>?) {
                bitmap = resource
                failed = false
            }
            override fun onLoadCleared(placeholder: Drawable?) {
                bitmap = null
            }
            override fun onLoadFailed(errorDrawable: Drawable?) {
                bitmap = null
                failed = true
            }
        }
        Glide.with(context)
            .asBitmap()
            .load(model)
            .override(layoutWidth, layoutHeight)
            .into(target)
        onDispose {
            Glide.with(context).clear(target)
        }
    }

    val painter: Painter = bitmap?.let { BitmapPainter(it.asImageBitmap()) }
        ?: (if (failed) error else placeholder)
        ?: ColorPainter(Color.Transparent)

    Image(
        painter = painter,
        contentDescription = contentDescription,
        modifier = modifier.onSizeChanged { size ->
            if (size.width > 0 && size.height > 0) {
                layoutWidth = size.width
                layoutHeight = size.height
            }
        },
        contentScale = contentScale,
        alignment = alignment,
    )
}
