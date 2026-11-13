/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.chats

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.CancellationException

/** Green when the user's node answers for them, grey otherwise; nothing until known. */
@Composable
internal fun UserStatusDot(userId: String, isOnline: suspend (String) -> Boolean?) {
    val online by produceState<Boolean?>(null, userId) {
        value = try {
            isOnline(userId)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        }
    }
    val known = online ?: return
    Box(
        modifier = Modifier
            .padding(horizontal = 6.dp)
            .size(10.dp)
            .background(if (known) Color(0xFF22C55E) else Color.Gray, CircleShape),
    )
}
