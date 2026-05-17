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

package site.warpnet.warpdroid.ui.tweetcomponents

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.requiredSize
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.painter.BitmapPainter
import androidx.compose.ui.graphics.painter.ColorPainter
import androidx.compose.ui.graphics.painter.Painter
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.dimensionResource
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.dp
import androidx.core.net.toUri
import coil3.compose.AsyncImage
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.ViewMediaActivity
import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.entity.PreviewCard
import site.warpnet.warpdroid.interfaces.TweetActionListener
import site.warpnet.warpdroid.ui.WarpdroidPreviewTheme
import site.warpnet.warpdroid.ui.WarpdroidTextButton
import site.warpnet.warpdroid.ui.preferences.LocalAccount
import site.warpnet.warpdroid.ui.preferences.LocalPreferences
import site.warpnet.warpdroid.ui.tweetcomponents.fake.fakeTweetViewData
import site.warpnet.warpdroid.ui.tweetcomponents.fake.noopListener
import site.warpnet.warpdroid.ui.warpdroidColors
import site.warpnet.warpdroid.ui.warpdroidDefaultCornerShape
import site.warpnet.warpdroid.util.BlurHashDecoder
import site.warpnet.warpdroid.util.getRelativeTimeSpanString
import site.warpnet.warpdroid.viewdata.TweetViewData
import kotlin.math.roundToInt
import kotlin.math.sqrt

@Composable
fun LinkPreviewCard(
    statusViewData: TweetViewData.Concrete,
    isExpanded: Boolean,
    listener: TweetActionListener,
) {
    val status = statusViewData.actionable
    val card = status.card

    if (card == null ||
        (status.spoilerText.isNotEmpty() && !isExpanded) ||
        status.attachments.isNotEmpty() ||
        (!LocalPreferences.current.showLinkPreviews && !statusViewData.isDetailed)
    ) {
        return // no card shown
    }

    val cardModifier = Modifier
        .fillMaxWidth()
        .padding(top = 6.dp)
        .clip(warpdroidDefaultCornerShape)
        .background(colorScheme.surface)
        .clickable {
            listener.onViewUrl(card.url)
        }

    if (card.width <= card.height || card.image == null) {
        Row(
            modifier = cardModifier
                .height(IntrinsicSize.Min)
                .run {
                    if (card.image != null) {
                        heightIn(dimensionResource(R.dimen.card_image_horizontal_width))
                    } else {
                        this
                    }
                },
            verticalAlignment = Alignment.CenterVertically
        ) {
            Box(
                modifier = Modifier
                    .width(dimensionResource(R.dimen.card_image_horizontal_width))
                    .fillMaxHeight()
            ) {
                LinkPreviewImage(
                    imageUrl = card.image,
                    embedUrl = card.embedUrl?.takeIf { card.type == PreviewCard.TYPE_PHOTO },
                    width = card.width,
                    height = card.height,
                    blurhash = card.blurhash,
                    blurMedia = statusViewData.filter?.action == Filter.Action.BLUR,
                    sensitive = statusViewData.actionable.sensitive,
                    modifier = Modifier.matchParentSize()
                )
            }
            LinkPreviewDescription(
                card = card,
                listener = listener
            )
        }
    } else {
        val imageAspectRatio = (card.width.toFloat() / card.height.toFloat()).coerceAtMost(4f)

        Column(
            modifier = cardModifier
        ) {
            LinkPreviewImage(
                imageUrl = card.image,
                embedUrl = card.embedUrl?.takeIf { card.type == PreviewCard.TYPE_PHOTO },
                width = card.width,
                height = card.height,
                blurhash = card.blurhash,
                blurMedia = statusViewData.filter?.action == Filter.Action.BLUR,
                sensitive = statusViewData.actionable.sensitive,
                modifier = Modifier
                    .aspectRatio(imageAspectRatio)
                    .fillMaxWidth()
            )
            LinkPreviewDescription(
                card = card,
                listener = listener
            )
        }
    }
}

@Composable
private fun LinkPreviewImage(
    imageUrl: String?,
    embedUrl: String?,
    width: Int,
    height: Int,
    blurhash: String?,
    blurMedia: Boolean,
    sensitive: Boolean,
    modifier: Modifier = Modifier
) {
    if (imageUrl.isNullOrEmpty()) {
        Icon(
            painter = painterResource(R.drawable.card_image_placeholder),
            tint = warpdroidColors.tertiaryTextColor,
            contentDescription = null,
            modifier = modifier.requiredSize(72.dp).padding(12.dp)
        )
    } else {
        val aspectRatio = if (width != 0 && height != 0) {
            width.toFloat() / height.toFloat()
        } else {
            1f
        }
        val showBlurhash = LocalPreferences.current.useBlurhash
        val backgroundAccent = warpdroidColors.backgroundAccent
        val context = LocalContext.current
        val showMedia = LocalAccount.current?.mediaPreviewEnabled != false

        val placeholder: Painter = remember(blurhash) {
            if (showBlurhash && blurhash != null) {
                val height = sqrt(128 / aspectRatio).roundToInt().coerceIn(16, 256)
                val width = (height * aspectRatio).roundToInt().coerceIn(16, 256)
                BlurHashDecoder.decode(blurhash, width, height, 1f)?.let { blurhashBitmap ->
                    BitmapPainter(blurhashBitmap.asImageBitmap())
                } ?: ColorPainter(backgroundAccent)
            } else {
                ColorPainter(backgroundAccent)
            }
        }

        AsyncImage(
            model = if (sensitive || blurMedia || !showMedia) null else imageUrl,
            contentDescription = null,
            contentScale = ContentScale.Crop,
            placeholder = placeholder,
            error = placeholder,
            modifier = modifier
                .run {
                    if (embedUrl != null) {
                        clickable {
                            ViewMediaActivity.newSingleImageIntent(context, embedUrl)
                        }
                    } else {
                        this
                    }
                }
        )
    }
}

@Composable
private fun LinkPreviewDescription(
    card: PreviewCard,
    listener: TweetActionListener
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp, vertical = 8.dp)
    ) {
        val providerName = if (card.providerName.isNullOrEmpty()) {
            card.url.toUri().host
        } else {
            card.providerName
        }
        val cardMetadata = if (card.publishedAt == null) {
            providerName
        } else {
            val metadataJoiner = stringResource(R.string.metadata_joiner)
            providerName + metadataJoiner + getRelativeTimeSpanString(LocalContext.current, card.publishedAt.time, System.currentTimeMillis())
        }
        if (cardMetadata != null) {
            Text(
                text = cardMetadata,
                color = warpdroidColors.secondaryTextColor,
                style = LocalPreferences.current.statusTextStyles.medium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(bottom = 4.dp)
            )
        }

        Text(
            text = card.title,
            color = warpdroidColors.secondaryTextColor,
            style = LocalPreferences.current.statusTextStyles.medium,
            fontWeight = FontWeight.Medium,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(bottom = 4.dp)
        )

        val cardAuthor = card.authors.firstOrNull()
        val cardAuthorName = cardAuthor?.name ?: card.authorName
        val cardDescription = cardAuthorName.takeIf { !it.isNullOrEmpty() } ?: card.description

        if (cardAuthor?.account != null) {
            WarpdroidTextButton(
                text = stringResource(R.string.preview_card_more_by_author, cardAuthor.account.name),
                onClick = {
                    listener.onViewAccount(cardAuthor.account.id)
                },
                modifier = Modifier.align(Alignment.CenterHorizontally)
            )
        } else if (cardDescription.isNotEmpty()) {
            Text(
                text = cardDescription,
                color = warpdroidColors.tertiaryTextColor,
                style = LocalPreferences.current.statusTextStyles.medium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
        }
    }
}

@PreviewLightDark
@Composable
fun LinkPreviewCardPreview() {
    val viewData = fakeTweetViewData()
    WarpdroidPreviewTheme {
        LinkPreviewCard(
            statusViewData = viewData
                .copy(
                    status = viewData.status.copy(
                        card = PreviewCard(
                            url = "https://tusky.app",
                            title = "Warpdroid - Warpnet client for Android",
                            description = "Warpdroid is a lightweight Android client for Warpnet, a free and open-source social network server.",
                            authors = emptyList(),
                            authorName = "",
                            providerName = "warpdroid.app",
                            publishedAt = null,
                            image = "https://files.mastodon.social/cache/preview_cards/images/008/190/651/original/ba3ee53a6bca3f88.png",
                            type = "link",
                            width = 686,
                            height = 335,
                            blurhash = "UC8;x[yGIs%MWZb0oht88^n#xuIoROoct7Rl",
                            embedUrl = ""
                        ),
                        attachments = emptyList(),
                    ),
                    isDetailed = true
                ),
            isExpanded = true,
            listener = noopListener
        )
    }
}
