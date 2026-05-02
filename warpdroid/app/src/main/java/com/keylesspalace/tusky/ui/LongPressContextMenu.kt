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

package com.keylesspalace.tusky.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.indication
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.PressInteraction
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.ripple
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntRect
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupPositionProvider
import androidx.compose.ui.window.PopupProperties

/**
 * A menu that is shown on long press.
 * @param menuContent The content of the menu.
 * @param content The content that makes up the clickable area that shows the menu
 * @param onClick called when the content is clicked
 * @param keys Whenever the keys changes, the menu will be recomposed
 */
@Composable
fun LongPressContextMenu(
    menuContent: @Composable () -> Unit,
    content: @Composable BoxScope.() -> Unit,
    modifier: Modifier = Modifier,
    onClick: () -> Unit = { },
    key1: Any,
    key2: Any
) {
    var showPopup by remember { mutableStateOf(false) }
    var touchPoint: Offset by remember { mutableStateOf(Offset.Zero) }

    val interactionSource = remember { MutableInteractionSource() }

    Box(
        modifier = modifier
            .indication(interactionSource, ripple())
            .pointerInput(key1, key2) {
                detectTapGestures(
                    onPress = { offset ->
                        val press = PressInteraction.Press(offset)
                        interactionSource.emit(press)
                        tryAwaitRelease()
                        interactionSource.emit(PressInteraction.Release(press))
                    },
                    onLongPress = { offset ->
                        touchPoint = offset
                        showPopup = true
                    },
                    onTap = { onClick() }
                )
            }
    ) {
        if (showPopup) {
            // offset the popup so it is not covered by the finger
            val popupOffset = with(LocalDensity.current) {
                24.dp.roundToPx()
            }

            Popup(
                properties = PopupProperties(focusable = true),
                popupPositionProvider = object : PopupPositionProvider {
                    override fun calculatePosition(
                        anchorBounds: IntRect,
                        windowSize: IntSize,
                        layoutDirection: LayoutDirection,
                        popupContentSize: IntSize
                    ): IntOffset {
                        // convert the touch offset, which is relative to the touched element, to window coordinates
                        val touchX = anchorBounds.left + touchPoint.x.toInt()
                        val touchY = anchorBounds.top + touchPoint.y.toInt()

                        return if (touchY > windowSize.height / 2) {
                            // more space at the top - show popup above touch point
                            IntOffset(touchX, touchY - popupContentSize.height - popupOffset)
                        } else {
                            // more space at the bottom - show popup below touch point
                            IntOffset(touchX, touchY + popupOffset)
                        }
                    }
                },
                onDismissRequest = { showPopup = false },
            ) {
                Box(
                    modifier = Modifier
                        .background(colorScheme.surface, RoundedCornerShape(4.dp))
                        .widthIn(max = 380.dp)
                        .padding(horizontal = 12.dp, vertical = 6.dp)
                ) {
                    menuContent()
                }
            }
        }
        content()
    }
}
