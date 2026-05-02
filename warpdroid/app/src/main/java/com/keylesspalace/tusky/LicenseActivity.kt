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

package com.keylesspalace.tusky

import android.os.Bundle
import android.util.Log
import androidx.activity.compose.setContent
import androidx.annotation.RawRes
import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement.Absolute.spacedBy
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.windowInsetsBottomHeight
import androidx.compose.foundation.lazy.staggeredgrid.LazyStaggeredGridScope
import androidx.compose.foundation.lazy.staggeredgrid.LazyVerticalStaggeredGrid
import androidx.compose.foundation.lazy.staggeredgrid.StaggeredGridCells
import androidx.compose.foundation.lazy.staggeredgrid.StaggeredGridItemSpan
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.dp
import com.keylesspalace.tusky.ui.TuskyPreviewTheme
import com.keylesspalace.tusky.ui.TuskyTheme
import com.keylesspalace.tusky.util.openLink
import dagger.hilt.android.AndroidEntryPoint
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okio.buffer
import okio.source

@AndroidEntryPoint
class LicenseActivity : BaseActivity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        setContent {
            TuskyTheme {
                var apache2License by remember { mutableStateOf("") }

                LaunchedEffect(Unit) {
                    apache2License = loadFileContent(R.raw.apache)
                }

                LicenseActivityContent(
                    apache2License = apache2License,
                    onBack = { finish() }
                )
            }
        }
    }

    private suspend fun loadFileContent(@RawRes fileId: Int): String = withContext(Dispatchers.IO) {
        try {
            resources.openRawResource(fileId).source().buffer().use { it.readUtf8() }
        } catch (e: IOException) {
            Log.w("LicenseActivity", "failed loading file content", e)
            ""
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun LicenseActivityContent(
    apache2License: String,
    onBack: () -> Unit,
) {
    Scaffold(
        contentWindowInsets = WindowInsets(0, 0, 0, 0),
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.title_licenses)) },
                navigationIcon = {
                    IconButton(
                        onClick = onBack
                    ) {
                        Icon(painterResource(R.drawable.ic_arrow_back_24dp), "back")
                    }
                }
            )
        }
    ) { contentPadding ->
        BoxWithConstraints {
            val columns = if (maxWidth < 480.dp) {
                StaggeredGridCells.Fixed(1)
            } else {
                StaggeredGridCells.FixedSize(320.dp)
            }

            LazyVerticalStaggeredGrid(
                verticalItemSpacing = 8.dp,
                horizontalArrangement = spacedBy(8.dp, Alignment.CenterHorizontally),
                columns = columns,
                contentPadding = PaddingValues(horizontal = 12.dp, vertical = 8.dp),
                modifier = Modifier.padding(contentPadding)
            ) {
                item(
                    key = "text",
                    span = StaggeredGridItemSpan.FullLine
                ) {
                    Text(stringResource(R.string.license_description))
                }
                licenseCard(
                    name = "Kotlin",
                    license = R.string.license_apache_2,
                    link = "https://kotlinlang.org/"
                )
                licenseCard(
                    name = "AndroidX",
                    license = R.string.license_apache_2,
                    link = "https://developer.android.com/jetpack/androidx"
                )
                licenseCard(
                    name = "Material Components for Android",
                    license = R.string.license_apache_2,
                    link = "https://github.com/material-components/material-components-android"
                )
                licenseCard(
                    name = "OkHttp",
                    license = R.string.license_apache_2,
                    link = "https://square.github.io/okhttp/"
                )
                licenseCard(
                    name = "Conscrypt",
                    license = R.string.license_apache_2,
                    link = "https://github.com/google/conscrypt"
                )
                licenseCard(
                    name = "Retrofit",
                    license = R.string.license_apache_2,
                    link = "https://square.github.io/retrofit/"
                )
                licenseCard(
                    name = "Moshi",
                    license = R.string.license_apache_2,
                    link = "https://github.com/square/moshi"
                )
                licenseCard(
                    name = "Glide",
                    license = R.string.license_apache_2,
                    link = "https://bumptech.github.io/glide/"
                )
                licenseCard(
                    name = "Dagger 2",
                    license = R.string.license_apache_2,
                    link = "https://dagger.dev"
                )
                licenseCard(
                    name = "MaterialDrawer",
                    license = R.string.license_apache_2,
                    link = "https://github.com/mikepenz/MaterialDrawer"
                )
                licenseCard(
                    name = "SparkButton",
                    license = R.string.license_apache_2,
                    link = "https://github.com/connyduck/SparkButton"
                )
                licenseCard(
                    name = "TouchImageView",
                    license = R.string.license_apache_2,
                    link = "https://github.com/MikeOrtiz/TouchImageView"
                )
                licenseCard(
                    name = "Android Image Cropper",
                    license = R.string.license_apache_2,
                    link = "https://github.com/CanHub/Android-Image-Cropper"
                )
                licenseCard(
                    name = "APNG4Android",
                    license = R.string.license_apache_2,
                    link = "https://github.com/penfeizhou/APNG4Android"
                )
                licenseCard(
                    name = "FileMojiCompat",
                    license = R.string.license_apache_2,
                    link = "https://github.com/C1710/FileMojiCompat"
                )
                licenseCard(
                    name = "UnifiedPush",
                    license = R.string.license_apache_2,
                    link = "https://unifiedpush.org/"
                )
                licenseCard(
                    name = "Twemoji",
                    license = R.string.license_cc_by_4,
                    link = "https://github.com/twitter/twemoji"
                )
                licenseCard(
                    name = "Blobmoji",
                    license = R.string.license_cc_by_4,
                    link = "https://github.com/c1710/blobmoji"
                )
                licenseCard(
                    name = "Tusky elephant artwork",
                    license = R.string.license_cc_by_sa_4,
                    link = "https://codeberg.org/tusky/artwork"
                )

                if (!apache2License.isEmpty()) {
                    item(
                        key = "apache",
                        span = StaggeredGridItemSpan.FullLine
                    ) {
                        Text(apache2License)
                    }
                }

                item(
                    key = "bottomSpacer",
                    span = StaggeredGridItemSpan.FullLine
                ) {
                    Spacer(Modifier.windowInsetsBottomHeight(WindowInsets.systemBars))
                }
            }
        }
    }
}

private fun LazyStaggeredGridScope.licenseCard(
    name: String,
    @StringRes license: Int,
    link: String
) = item(key = name) {
    val context = LocalContext.current
    Card(
        onClick = {
            context.openLink(link)
        },
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
        ) {
            Text(name, fontWeight = FontWeight.Medium)
            Text(stringResource(license))
            Text(link, color = MaterialTheme.colorScheme.primary)
        }
    }
}

@PreviewLightDark
@Composable
fun LicenseContentPreview() {
    TuskyPreviewTheme {
        LicenseActivityContent(
            apache2License = "some license text",
            onBack = { }
        )
    }
}
