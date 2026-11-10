package com.vanillapay.monitor

import android.content.ContentResolver
import android.graphics.BitmapFactory
import android.net.Uri
import android.util.Rational
import android.view.Surface
import androidx.camera.core.Camera
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.core.UseCaseGroup
import androidx.camera.core.ViewPort
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.LuminanceSource
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.RGBLuminanceSource
import com.google.zxing.ReaderException
import com.google.zxing.common.HybridBinarizer
import com.google.zxing.qrcode.QRCodeReader
import java.util.concurrent.Executors

internal fun decodeQr(source: LuminanceSource): String? = try {
    QRCodeReader().decode(
        BinaryBitmap(HybridBinarizer(source)),
        mapOf(DecodeHintType.TRY_HARDER to true),
    ).text
} catch (_: ReaderException) {
    null
}

internal fun decodePhoto(resolver: ContentResolver, uri: Uri): String? {
    val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
    resolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, bounds) }
    if (bounds.outWidth <= 0 || bounds.outHeight <= 0) return null
    var sample = 1
    while (maxOf(bounds.outWidth, bounds.outHeight) / sample > 2048) sample *= 2
    val options = BitmapFactory.Options().apply { inSampleSize = sample }
    val bitmap = resolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, options) } ?: return null
    try {
        val width = bitmap.width
        val height = bitmap.height
        val pixels = IntArray(width * height)
        bitmap.getPixels(pixels, 0, width, 0, 0, width, height)
        return decodeQr(RGBLuminanceSource(width, height, pixels))
    } finally {
        bitmap.recycle()
    }
}

private fun decodeFrame(image: ImageProxy): String? {
    // The shared CameraX viewport maps the square preview to this crop rectangle.
    val crop = image.cropRect
    val width = crop.width()
    val height = crop.height()
    val plane = image.planes[0]
    val buffer = plane.buffer
    val start = buffer.position()
    val luminance = ByteArray(width * height)
    for (row in 0 until height) {
        buffer.position(start + (crop.top + row) * plane.rowStride + crop.left)
        buffer.get(luminance, row * width, width)
    }
    return decodeQr(PlanarYUVLuminanceSource(luminance, width, height, 0, 0, width, height, false))
}

@Composable
internal fun CameraPreview(
    modifier: Modifier,
    onCamera: (Camera?) -> Unit,
    onQr: (String) -> Unit,
    onUnavailable: () -> Unit,
) {
    val context = LocalContext.current
    val owner = LocalLifecycleOwner.current
    val reportCamera = rememberUpdatedState(onCamera)
    val reportQr = rememberUpdatedState(onQr)
    val reportUnavailable = rememberUpdatedState(onUnavailable)
    val view = remember(context) {
        PreviewView(context).apply { implementationMode = PreviewView.ImplementationMode.COMPATIBLE }
    }

    AndroidView(factory = { view }, modifier = modifier)
    DisposableEffect(view, owner) {
        val rotation = view.display?.rotation ?: Surface.ROTATION_0
        val preview = Preview.Builder().setTargetRotation(rotation).build()
        val analysis = ImageAnalysis.Builder()
            .setTargetRotation(rotation)
            .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
            .build()
        val viewport = ViewPort.Builder(Rational(1, 1), rotation).build()
        val group = UseCaseGroup.Builder().setViewPort(viewport)
            .addUseCase(preview).addUseCase(analysis).build()
        val executor = Executors.newSingleThreadExecutor()
        val main = ContextCompat.getMainExecutor(context)
        val provider = ProcessCameraProvider.getInstance(context)
        var boundProvider: ProcessCameraProvider? = null
        var active = true
        var delivered = false
        analysis.setAnalyzer(executor) { frame ->
            val text = try { decodeFrame(frame) } finally { frame.close() }
            if (text != null) main.execute {
                if (active && !delivered) {
                    delivered = true
                    reportQr.value(text)
                }
            }
        }
        provider.addListener({
            if (active) {
                try {
                    val cameras = provider.get()
                    preview.setSurfaceProvider(view.surfaceProvider)
                    val camera = cameras.bindToLifecycle(owner, CameraSelector.DEFAULT_BACK_CAMERA, group)
                    boundProvider = cameras
                    reportCamera.value(camera)
                } catch (_: Exception) {
                    reportUnavailable.value()
                }
            }
        }, main)
        onDispose {
            active = false
            analysis.clearAnalyzer()
            boundProvider?.unbind(preview, analysis)
            executor.shutdown()
            reportCamera.value(null)
        }
    }
}
