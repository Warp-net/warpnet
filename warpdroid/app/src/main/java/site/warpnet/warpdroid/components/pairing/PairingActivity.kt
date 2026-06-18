/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.pairing

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Bundle
import android.text.InputType
import android.view.View
import android.widget.EditText
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.core.content.ContextCompat
import androidx.lifecycle.lifecycleScope
import com.google.android.material.button.MaterialButton
import site.warpnet.warpdroid.MainActivity
import site.warpnet.warpdroid.R
import com.squareup.moshi.Moshi
import dagger.hilt.android.AndroidEntryPoint
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import javax.inject.Inject
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import site.warpnet.transport.dto.AuthNodeInfo

/**
 * Single-screen pairing flow: camera preview → confirm → connect → hand off
 * to [MainActivity]. Errors return the user to the scanner (camera stays on)
 * so they can retry without backing out of the activity.
 */
@AndroidEntryPoint
class PairingActivity : AppCompatActivity() {

    @Inject lateinit var moshi: Moshi

    @Inject lateinit var pairingCoordinator: PairingCoordinator

    @Inject lateinit var pairedNodeStore: PairedNodeStore

    private lateinit var previewView: PreviewView
    private lateinit var progress: View
    private lateinit var progressBar: com.google.android.material.progressindicator.LinearProgressIndicator
    private lateinit var progressCaption: android.widget.TextView
    private lateinit var messagePanel: View
    private lateinit var messageTitle: android.widget.TextView
    private lateinit var messageBody: android.widget.TextView
    private lateinit var connectButton: MaterialButton
    private lateinit var cancelButton: MaterialButton
    private lateinit var scanPrompt: View
    private lateinit var manualEntryLink: View

    private val validator by lazy { AuthNodeInfoValidator(moshi) }
    private val startupProgress by lazy {
        StartupProgressController(progressBar, progressCaption, STARTUP_STAGES)
    }
    private val cameraExecutor: ExecutorService = Executors.newSingleThreadExecutor()
    private var analyzer: QrCodeAnalyzer? = null
    private var cameraProvider: ProcessCameraProvider? = null

    private val cameraPermissionLauncher = registerForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { granted ->
        if (granted) {
            startCamera()
        } else {
            showManualInput()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_pairing)
        previewView = findViewById(R.id.previewView)
        progress = findViewById(R.id.progress)
        progressBar = findViewById(R.id.progressBar)
        progressCaption = findViewById(R.id.progressCaption)
        messagePanel = findViewById(R.id.messagePanel)
        messageTitle = findViewById(R.id.messageTitle)
        messageBody = findViewById(R.id.messageBody)
        connectButton = findViewById(R.id.connectButton)
        cancelButton = findViewById(R.id.cancelButton)
        scanPrompt = findViewById(R.id.scanPrompt)
        manualEntryLink = findViewById(R.id.manualEntryLink)
        manualEntryLink.setOnClickListener { showManualInput() }

        // Try to re-authenticate using the keystore-backed QR payload from
        // the last successful pair. On success we hand straight off to
        // MainActivity without showing the camera. On auth-level failure
        // (server rejection or peer-id mismatch) the stored QR is wiped
        // so the user lands on a fresh scan. The lazy EncryptedSharedPreferences
        // open happens on Dispatchers.IO so the keystore init + file I/O
        // never blocks the first frame.
        scanPrompt.visibility = View.GONE
        manualEntryLink.visibility = View.GONE
        previewView.visibility = View.GONE
        progress.visibility = View.VISIBLE
        lifecycleScope.launch {
            val storedQr = withContext(Dispatchers.IO) { pairedNodeStore.loadRawQr() }
            if (storedQr != null) {
                tryAutoPair(storedQr)
            } else {
                progress.visibility = View.GONE
                startCameraOrManual()
            }
        }
    }

    private suspend fun tryAutoPair(rawJson: String) {
        when (val result = validator.validate(rawJson)) {
            is ValidationResult.Valid -> {
                startupProgress.start(lifecycleScope)
                val outcome = pairingCoordinator.pair(result.authNodeInfo, result.rawJson)
                finishStartupProgress(outcome)
                progress.visibility = View.GONE
                handleAutoPairOutcome(outcome)
            }
            is ValidationResult.Invalid -> {
                // Stored payload no longer parses; treat as a hard failure
                // and drop it so the next launch starts clean.
                withContext(Dispatchers.IO) { pairedNodeStore.clear() }
                progress.visibility = View.GONE
                startCameraOrManual()
            }
        }
    }

    private fun handleAutoPairOutcome(outcome: PairingOutcome) {
        when (outcome) {
            is PairingOutcome.Success -> {
                startActivity(
                    Intent(this, MainActivity::class.java)
                        .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_NEW_TASK),
                )
                finish()
            }
            is PairingOutcome.Rejected,
            is PairingOutcome.PeerIdMismatch -> {
                // Auth was refused by the fat node, so the stored credentials
                // will not work again. Wipe them and fall back to a fresh scan.
                pairedNodeStore.clear()
                Toast.makeText(
                    this,
                    R.string.warpnet_pair_auto_failed,
                    Toast.LENGTH_LONG,
                ).show()
                startCameraOrManual()
            }
            is PairingOutcome.TransportError -> {
                // Likely network / firewall; keep the stored QR so the next
                // launch can retry, but still let the user scan a new one now.
                Toast.makeText(
                    this,
                    getString(R.string.warpnet_pair_error_transport, outcome.message),
                    Toast.LENGTH_LONG,
                ).show()
                startCameraOrManual()
            }
        }
    }

    private fun startCameraOrManual() {
        scanPrompt.visibility = View.VISIBLE
        manualEntryLink.visibility = View.VISIBLE
        previewView.visibility = View.VISIBLE
        if (!packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY)) {
            showManualInput()
            return
        }
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.CAMERA) ==
            PackageManager.PERMISSION_GRANTED
        ) {
            startCamera()
        } else {
            cameraPermissionLauncher.launch(Manifest.permission.CAMERA)
        }
    }

    override fun onDestroy() {
        super.onDestroy()
        analyzer?.close()
        cameraProvider?.unbindAll()
        cameraExecutor.shutdown()
    }

    private fun startCamera() {
        val providerFuture = ProcessCameraProvider.getInstance(this)
        providerFuture.addListener({
            val provider = try {
                providerFuture.get()
            } catch (e: Exception) {
                // Camera service unavailable (emulators, cameraless devices
                // where the feature check didn't catch it). Fall back to
                // manual input so the pairing flow isn't a dead-end.
                showManualInput()
                return@addListener
            }
            cameraProvider = provider

            val preview = Preview.Builder().build().also {
                it.surfaceProvider = previewView.surfaceProvider
            }

            val imageAnalysis = ImageAnalysis.Builder()
                .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                .build()
            val qrAnalyzer = QrCodeAnalyzer { payload -> onQrScanned(payload) }
            analyzer = qrAnalyzer
            imageAnalysis.setAnalyzer(cameraExecutor, qrAnalyzer)

            try {
                provider.unbindAll()
                provider.bindToLifecycle(
                    this,
                    CameraSelector.DEFAULT_BACK_CAMERA,
                    preview,
                    imageAnalysis,
                )
            } catch (e: IllegalArgumentException) {
                // No back camera to bind to (front-only devices).
                showManualInput()
            } catch (e: IllegalStateException) {
                showManualInput()
            }
        }, ContextCompat.getMainExecutor(this))
    }

    private fun showManualInput() {
        scanPrompt.visibility = View.GONE
        manualEntryLink.visibility = View.GONE
        previewView.visibility = View.GONE
        val input = EditText(this).apply {
            setHint(R.string.warpnet_pair_manual_hint)
            isSingleLine = false
            maxLines = 10
            inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_FLAG_MULTI_LINE
        }
        AlertDialog.Builder(this)
            .setTitle(R.string.warpnet_pair_manual_title)
            .setMessage(R.string.warpnet_pair_manual_body)
            .setView(input)
            .setCancelable(false)
            .setPositiveButton(R.string.warpnet_pair_manual_submit) { _, _ ->
                handleManualInput(input.text.toString())
            }
            .setNegativeButton(R.string.action_cancel) { _, _ -> finish() }
            .show()
    }

    private fun handleManualInput(raw: String) {
        when (val result = validator.validate(raw)) {
            is ValidationResult.Valid -> showConfirmation(result.authNodeInfo, result.rawJson)
            is ValidationResult.Invalid -> {
                Toast.makeText(
                    this,
                    getString(R.string.warpnet_pair_invalid_qr, result.reason),
                    Toast.LENGTH_LONG,
                ).show()
                showManualInput()
            }
        }
    }

    private fun onQrScanned(raw: String) {
        // Analyzer fires off a camera thread; hop back onto the main thread
        // before touching views.
        runOnUiThread {
            when (val result = validator.validate(raw)) {
                is ValidationResult.Valid -> showConfirmation(result.authNodeInfo, result.rawJson)
                is ValidationResult.Invalid -> {
                    Toast.makeText(
                        this,
                        getString(R.string.warpnet_pair_invalid_qr, result.reason),
                        Toast.LENGTH_LONG,
                    ).show()
                    // Re-arm the analyzer so the user can scan again without
                    // leaving the activity. startCamera() builds a fresh
                    // QrCodeAnalyzer and rebinds inside the same try/catch
                    // fallback used on first start.
                    analyzer?.close()
                    startCamera()
                }
            }
        }
    }

    private fun showConfirmation(info: AuthNodeInfo, rawJson: String) {
        scanPrompt.visibility = View.GONE
        manualEntryLink.visibility = View.GONE
        messagePanel.visibility = View.VISIBLE
        messageTitle.text = getString(R.string.warpnet_pair_title)
        val shortNodeId = info.nodeId.let { if (it.length > 16) "${it.take(8)}…${it.takeLast(6)}" else it }
        val firstAddr = info.addresses.firstOrNull().orEmpty()
        // The flat AuthNodeInfo no longer carries a username; show userId in
        // the "Owner" slot of the confirmation string instead.
        messageBody.text = getString(
            R.string.warpnet_pair_confirm_body,
            info.userId,
            shortNodeId,
            firstAddr,
        )
        connectButton.setOnClickListener { runPairing(info, rawJson) }
        cancelButton.setOnClickListener { finish() }
    }

    private fun runPairing(info: AuthNodeInfo, rawJson: String) {
        messagePanel.visibility = View.GONE
        progress.visibility = View.VISIBLE
        lifecycleScope.launch {
            startupProgress.start(lifecycleScope)
            val outcome = pairingCoordinator.pair(info, rawJson)
            finishStartupProgress(outcome)
            progress.visibility = View.GONE
            handleOutcome(outcome)
        }
    }

    /**
     * Snap the bar to 100% on a real successful pair; otherwise just stop the
     * timed fill so the error panel replaces it. Completion is gated on the
     * actual [PairingOutcome] so the UI never reaches 100% before the node is
     * connected.
     */
    private suspend fun finishStartupProgress(outcome: PairingOutcome) {
        if (outcome is PairingOutcome.Success) {
            startupProgress.complete(R.string.warpnet_startup_stage_ready)
        } else {
            startupProgress.cancel()
        }
    }

    private fun handleOutcome(outcome: PairingOutcome) {
        when (outcome) {
            is PairingOutcome.Success -> {
                startActivity(
                    Intent(this, MainActivity::class.java)
                        .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_NEW_TASK),
                )
                finish()
            }
            is PairingOutcome.Rejected -> showFatalMessage(
                getString(R.string.warpnet_pair_error_rejected, outcome.code, outcome.message),
            )
            is PairingOutcome.PeerIdMismatch -> showFatalMessage(
                getString(R.string.warpnet_pair_error_peer_mismatch),
            )
            is PairingOutcome.TransportError -> showFatalMessage(
                getString(R.string.warpnet_pair_error_transport, outcome.message),
            )
        }
    }

    private fun showFatalMessage(text: String) {
        scanPrompt.visibility = View.GONE
        manualEntryLink.visibility = View.GONE
        progress.visibility = View.GONE
        messagePanel.visibility = View.VISIBLE
        messageTitle.text = getString(R.string.warpnet_pair_title)
        messageBody.text = text
        connectButton.visibility = View.GONE
        cancelButton.text = getString(R.string.action_cancel)
        cancelButton.setOnClickListener { finish() }
    }

    private companion object {
        // Caption sequence for the cold-start progress bar, ordered to mirror
        // the libp2p node lifecycle (start → discover → handshake → pair →
        // connected) followed by the first tweet-load cycle (profile →
        // timeline) that runs once MainActivity takes over.
        val STARTUP_STAGES = listOf(
            R.string.warpnet_startup_stage_node,
            R.string.warpnet_startup_stage_peers,
            R.string.warpnet_startup_stage_handshake,
            R.string.warpnet_startup_stage_pairing,
            R.string.warpnet_startup_stage_connected,
            R.string.warpnet_startup_stage_account,
            R.string.warpnet_startup_stage_timeline,
        )
    }
}
