/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package com.keylesspalace.tusky.components.pairing

import android.annotation.SuppressLint
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import com.google.mlkit.vision.barcode.BarcodeScanner
import com.google.mlkit.vision.barcode.BarcodeScannerOptions
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.common.InputImage
import java.util.concurrent.atomic.AtomicBoolean

/**
 * CameraX ImageAnalysis delegate that decodes a single QR code and fires
 * [onResult] once. Subsequent frames are discarded so the UI can transition
 * to the confirmation screen without fighting a scanner that keeps re-firing.
 *
 * Uses the on-device ML Kit model (`com.google.mlkit:barcode-scanning`)
 * rather than the Play Services variant so the scanner works on phones
 * without Google Play Services.
 */
class QrCodeAnalyzer(
    private val onResult: (String) -> Unit,
) : ImageAnalysis.Analyzer {

    private val scanner: BarcodeScanner = BarcodeScanning.getClient(
        BarcodeScannerOptions.Builder()
            .setBarcodeFormats(Barcode.FORMAT_QR_CODE)
            .build(),
    )
    private val fired = AtomicBoolean(false)

    // Gate so only one ML Kit decode is in flight at a time. Without this,
    // a fast camera feed can queue many scanner.process() Tasks in parallel
    // even though we only need one successful result.
    private val processing = AtomicBoolean(false)

    @SuppressLint("UnsafeOptInUsageError")
    override fun analyze(image: ImageProxy) {
        if (fired.get() || !processing.compareAndSet(false, true)) {
            image.close()
            return
        }
        val media = image.image
        if (media == null) {
            processing.set(false)
            image.close()
            return
        }
        val input = InputImage.fromMediaImage(media, image.imageInfo.rotationDegrees)
        scanner.process(input)
            .addOnSuccessListener { barcodes ->
                val payload = barcodes.firstNotNullOfOrNull { it.rawValue }
                if (!payload.isNullOrBlank() && fired.compareAndSet(false, true)) {
                    onResult(payload)
                }
            }
            .addOnCompleteListener {
                processing.set(false)
                image.close()
            }
    }

    fun close() {
        scanner.close()
    }
}
