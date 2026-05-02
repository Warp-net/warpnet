package com.keylesspalace.tusky.util

import android.content.Context
import android.util.DisplayMetrics
import kotlin.math.roundToInt

fun dpToPx(context: Context, dp: Int): Int {
    val displayMetrics = context.resources.displayMetrics
    return (dp * (displayMetrics.xdpi / DisplayMetrics.DENSITY_DEFAULT)).roundToInt()
}
