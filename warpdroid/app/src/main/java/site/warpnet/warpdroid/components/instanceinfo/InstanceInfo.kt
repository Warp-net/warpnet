/* Copyright 2022 Warpdroid contributors
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

package site.warpnet.warpdroid.components.instanceinfo

data class InstanceInfo(
    val maxChars: Int,
    val pollMaxOptions: Int,
    val pollMaxLength: Int,
    val pollMinDuration: Int,
    val pollMaxDuration: Int,
    val charactersReservedPerUrl: Int,
    val videoSizeLimit: Int,
    val imageSizeLimit: Int,
    val imageMatrixLimit: Int,
    val maxMediaAttachments: Int,
    val mediaDescriptionLimit: Int,
    val maxFields: Int,
    val maxFieldNameLength: Int?,
    val maxFieldValueLength: Int?,
    val version: String?,
    val translationEnabled: Boolean,
    val warpnetApiVersion: Int?,
    val vapidKey: String?
)
