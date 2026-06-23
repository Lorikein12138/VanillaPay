package com.vanillapay.monitor.ui

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Bundle
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.camera.core.CameraSelector
import androidx.camera.core.ExperimentalGetImage
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import com.google.mlkit.vision.barcode.BarcodeScannerOptions
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.common.InputImage
import com.vanillapay.monitor.R
import com.vanillapay.monitor.bind.BindingPayload
import com.vanillapay.monitor.util.AppLog
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

class ScanActivity : AppCompatActivity() {
    private val executor = Executors.newSingleThreadExecutor()
    private val completed = AtomicBoolean(false)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        AppLog.i(TAG, "ScanActivity onCreate")
        setContentView(R.layout.activity_scan)
        val back = findViewById<android.widget.ImageButton>(R.id.btnBack)
        back.setOnClickListener { finish() }
        val baseTopMargin = (back.layoutParams as android.widget.FrameLayout.LayoutParams).topMargin
        androidx.core.view.ViewCompat.setOnApplyWindowInsetsListener(back) { v, insets ->
            val bars = insets.getInsets(androidx.core.view.WindowInsetsCompat.Type.systemBars())
            val lp = v.layoutParams as android.widget.FrameLayout.LayoutParams
            lp.topMargin = baseTopMargin + bars.top
            v.layoutParams = lp
            insets
        }
        if (checkSelfPermission(Manifest.permission.CAMERA) != PackageManager.PERMISSION_GRANTED) {
            AppLog.i(TAG, "camera permission not granted, requesting")
            ActivityCompat.requestPermissions(this, arrayOf(Manifest.permission.CAMERA), REQUEST_CAMERA)
        } else {
            startCamera()
        }
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == REQUEST_CAMERA && grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED) {
            AppLog.i(TAG, "camera permission granted")
            startCamera()
        } else {
            AppLog.w(TAG, "camera permission denied, finishing")
            Toast.makeText(this, "需要相机权限", Toast.LENGTH_SHORT).show()
            finish()
        }
    }

    @androidx.annotation.OptIn(ExperimentalGetImage::class)
    private fun startCamera() {
        AppLog.i(TAG, "startCamera: requesting ProcessCameraProvider")
        val providerFuture = ProcessCameraProvider.getInstance(this)
        providerFuture.addListener(
            {
                try {
                    val cameraProvider = providerFuture.get()
                    AppLog.i(TAG, "camera provider obtained")
                    val previewView = findViewById<PreviewView>(R.id.previewView)
                    val preview = Preview.Builder().build().also {
                        it.setSurfaceProvider(previewView.surfaceProvider)
                    }
                    val analysis = ImageAnalysis.Builder()
                        .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                        .build()
                    val scanner = BarcodeScanning.getClient(
                        BarcodeScannerOptions.Builder()
                            .setBarcodeFormats(Barcode.FORMAT_QR_CODE)
                            .build(),
                    )
                    analysis.setAnalyzer(executor) { imageProxy ->
                        val mediaImage = imageProxy.image
                        if (mediaImage == null) {
                            imageProxy.close()
                            return@setAnalyzer
                        }
                        val image = InputImage.fromMediaImage(mediaImage, imageProxy.imageInfo.rotationDegrees)
                        scanner.process(image)
                            .addOnSuccessListener { codes ->
                                val payload = codes.asSequence()
                                    .mapNotNull { it.rawValue }
                                    .firstOrNull { BindingPayload.parse(it) != null }
                                if (payload != null && completed.compareAndSet(false, true)) {
                                    AppLog.i(TAG, "binding QR recognized")
                                    setResult(Activity.RESULT_OK, Intent().putExtra(EXTRA_PAYLOAD, payload))
                                    cameraProvider.unbindAll()
                                    finish()
                                }
                            }
                            .addOnFailureListener { AppLog.w(TAG, "barcode process failed", it) }
                            .addOnCompleteListener {
                                imageProxy.close()
                            }
                    }
                    cameraProvider.unbindAll()
                    cameraProvider.bindToLifecycle(
                        this,
                        CameraSelector.DEFAULT_BACK_CAMERA,
                        preview,
                        analysis,
                    )
                    AppLog.i(TAG, "camera bound to lifecycle")
                } catch (e: Exception) {
                    AppLog.e(TAG, "startCamera failed", e)
                    Toast.makeText(this, getString(R.string.scan_camera_error, e.javaClass.simpleName), Toast.LENGTH_LONG).show()
                    finish()
                }
            },
            ContextCompat.getMainExecutor(this),
        )
    }

    override fun onDestroy() {
        executor.shutdown()
        super.onDestroy()
    }

    companion object {
        const val EXTRA_PAYLOAD = "payload"
        private const val REQUEST_CAMERA = 300
        private const val TAG = "Scan"
    }
}
