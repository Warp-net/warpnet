/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.pairing

import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.MultiFormatReader
import com.google.zxing.NotFoundException
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.common.HybridBinarizer
import java.util.concurrent.atomic.AtomicBoolean

/**
 * CameraX ImageAnalysis delegate that decodes a single QR code and fires
 * [onResult] once. Subsequent frames are discarded so the UI can transition
 * to the confirmation screen without fighting a scanner that keeps re-firing.
 *
 * Uses the fully FOSS ZXing core decoder (`com.google.zxing:core`) rather than
 * ML Kit / Google Play Services, so the app stays F-Droid-buildable and the
 * scanner works on any device.
 */
class QrCodeAnalyzer(
    private val onResult: (String) -> Unit,
) : ImageAnalysis.Analyzer {

    private val reader = MultiFormatReader().apply {
        setHints(
            mapOf(
                DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE),
            ),
        )
    }
    private val fired = AtomicBoolean(false)

    override fun analyze(image: ImageProxy) {
        if (fired.get()) {
            image.close()
            return
        }
        try {
            val payload = decode(image)
            if (!payload.isNullOrBlank() && fired.compareAndSet(false, true)) {
                onResult(payload)
            }
        } finally {
            image.close()
        }
    }

    /**
     * Decode the Y (luminance) plane of the CameraX frame with ZXing. CameraX
     * delivers YUV_420_888; the first plane is full-resolution luminance, which
     * is all a QR decode needs. Returns null when no code is present in-frame.
     */
    private fun decode(image: ImageProxy): String? {
        val plane = image.planes.firstOrNull() ?: return null
        val buffer = plane.buffer
        val data = ByteArray(buffer.remaining())
        buffer.get(data)

        val rowStride = plane.rowStride
        val width = image.width
        val height = image.height
        val source = PlanarYUVLuminanceSource(
            data,
            rowStride,
            height,
            0,
            0,
            width,
            height,
            false,
        )
        val bitmap = BinaryBitmap(HybridBinarizer(source))
        return try {
            reader.decodeWithState(bitmap).text
        } catch (e: NotFoundException) {
            null
        } finally {
            reader.reset()
        }
    }

    fun close() {
        // ZXing MultiFormatReader holds no native resources; nothing to release.
    }
}
