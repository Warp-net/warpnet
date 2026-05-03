/* Copyright 2025 Warpdroid Contributors
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

package site.warpnet.warpdroid.ui.preferences

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ProvidableCompositionLocal
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.lifecycle.viewmodel.compose.viewModel
import site.warpnet.warpdroid.db.entity.AccountEntity

val LocalPreferences: ProvidableCompositionLocal<WarpdroidPreferences> = compositionLocalOf { WarpdroidPreferences() }
val LocalAccount: ProvidableCompositionLocal<AccountEntity?> = compositionLocalOf { null }

@Composable
fun PreferencesProvider(
    viewModel: PreferencesProviderViewModel = viewModel(),
    content: @Composable () -> Unit
) {
    val preferences by viewModel.warpdroidPreferences.collectAsState()
    val account by viewModel.activeAccount.collectAsState()

    CompositionLocalProvider(LocalPreferences provides preferences) {
        CompositionLocalProvider(LocalAccount provides account, content)
    }
}
