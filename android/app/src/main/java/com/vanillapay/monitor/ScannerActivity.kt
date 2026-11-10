package com.vanillapay.monitor

import android.Manifest
import android.content.ActivityNotFoundException
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.view.HapticFeedbackConstants
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.camera.core.Camera
import androidx.camera.core.TorchState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.core.content.ContextCompat
import androidx.lifecycle.Observer
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.IOException

internal const val PHOTO_QR_ERROR = "未能从这张图片识别二维码，请选择清晰的二维码图片。"

class ScannerActivity : ComponentActivity() {
    private var granted by mutableStateOf(false)
    private var permissionAnswered by mutableStateOf(false)
    private var cameraUnavailable by mutableStateOf(false)
    private var picking by mutableStateOf(false)
    private var reading by mutableStateOf(false)
    private var photoError by mutableStateOf<String?>(null)
    private var camera by mutableStateOf<Camera?>(null)
    private var torchOn by mutableStateOf(false)
    private var completed = false
    private val torchObserver = Observer<Int> { torchOn = it == TorchState.ON }

    private val cameraPermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) {
        granted = it
        permissionAnswered = true
    }

    private val photoPicker = registerForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri ->
        picking = false
        if (uri != null) readPhoto(uri)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        permissionAnswered = savedInstanceState?.getBoolean("permissionAnswered") ?: false
        picking = savedInstanceState?.getBoolean("picking") ?: false
        granted = hasCameraPermission()
        enableEdgeToEdge()
        setContent {
            MonitorTheme {
                val canPreview = granted && !picking && !reading && !cameraUnavailable
                ScannerScreen(
                    permissionDenied = !granted && permissionAnswered,
                    cameraUnavailable = cameraUnavailable,
                    previewActive = camera != null,
                    torchAvailable = camera?.cameraInfo?.hasFlashUnit() == true,
                    torchOn = torchOn,
                    reading = reading,
                    photoError = photoError,
                    back = ::finish,
                    toggleTorch = { camera?.cameraControl?.enableTorch(!torchOn) },
                    choosePhoto = ::choosePhoto,
                    openSettings = {
                        startActivity(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.parse("package:$packageName")))
                    },
                ) { modifier ->
                    if (canPreview) CameraPreview(modifier, ::observeCamera, ::complete) { cameraUnavailable = true }
                }
            }
        }
        if (savedInstanceState == null && !granted) cameraPermission.launch(Manifest.permission.CAMERA)
    }

    override fun onResume() {
        super.onResume()
        if (granted && !hasCameraPermission()) permissionAnswered = true
        granted = hasCameraPermission()
    }

    override fun onSaveInstanceState(outState: Bundle) {
        outState.putBoolean("permissionAnswered", permissionAnswered)
        outState.putBoolean("picking", picking)
        super.onSaveInstanceState(outState)
    }

    private fun hasCameraPermission(): Boolean =
        ContextCompat.checkSelfPermission(this, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED

    private fun observeCamera(next: Camera?) {
        camera?.cameraInfo?.torchState?.removeObserver(torchObserver)
        camera = next
        torchOn = false
        next?.cameraInfo?.torchState?.observe(this, torchObserver)
    }

    private fun choosePhoto() {
        if (picking || reading || completed) return
        picking = true
        photoError = null
        try {
            photoPicker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
        } catch (_: ActivityNotFoundException) {
            picking = false
            photoError = "无法打开相册。"
        }
    }

    private fun readPhoto(uri: Uri) {
        reading = true
        lifecycleScope.launch {
            val qr = withContext(Dispatchers.IO) {
                try { decodePhoto(contentResolver, uri) }
                catch (_: IOException) { null }
                catch (_: SecurityException) { null }
            }
            reading = false
            if (qr == null) photoError = PHOTO_QR_ERROR else complete(qr)
        }
    }

    private fun complete(qr: String) {
        if (completed || isFinishing || picking || reading) return
        completed = true
        val feedback = if (Build.VERSION.SDK_INT >= 30) HapticFeedbackConstants.CONFIRM
            else HapticFeedbackConstants.VIRTUAL_KEY
        window.decorView.performHapticFeedback(feedback)
        setResult(RESULT_OK, Intent().putExtra(RESULT_QR, qr))
        finish()
    }

    companion object {
        internal const val RESULT_QR = "com.vanillapay.monitor.SCANNED_QR"
    }
}
