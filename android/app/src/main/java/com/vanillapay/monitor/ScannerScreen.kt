package com.vanillapay.monitor

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp

@Composable
internal fun ScannerScreen(
    permissionDenied: Boolean,
    cameraUnavailable: Boolean,
    previewActive: Boolean,
    torchAvailable: Boolean,
    torchOn: Boolean,
    reading: Boolean,
    photoError: String?,
    back: () -> Unit,
    toggleTorch: () -> Unit,
    choosePhoto: () -> Unit,
    openSettings: () -> Unit,
    preview: @Composable (Modifier) -> Unit,
) {
    Column(Modifier.fillMaxSize().safeDrawingPadding()) {
        BackHeader("扫一扫", back)
        BoxWithConstraints(Modifier.fillMaxWidth().weight(1f), contentAlignment = Alignment.Center) {
            val frameSize = minOf(maxWidth - 40.dp, (maxHeight - 120.dp).coerceAtLeast(140.dp), 360.dp)
            Column(
                Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 20.dp, vertical = 16.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                if (permissionDenied || cameraUnavailable) {
                    Panel {
                        Headline(if (permissionDenied) "相机权限未开启" else "相机暂不可用")
                        Paragraph(
                            if (permissionDenied) "开启相机权限即可现场扫码，也可以从相册选择绑定二维码。"
                            else "可以从相册选择绑定二维码，或返回后手动输入绑定码。",
                        )
                        if (permissionDenied) TextAction("去设置", openSettings)
                    }
                } else {
                    Headline("扫描绑定二维码")
                    Paragraph("将商户后台的绑定二维码放入取景框。")
                    Box(
                        Modifier.size(frameSize).clip(RoundedCornerShape(14.dp)).background(LocalPalette.current.muted),
                        contentAlignment = Alignment.Center,
                    ) {
                        Icon(painterResource(R.drawable.monitor_scan), null, Modifier.size(42.dp), LocalPalette.current.subtle)
                        preview(Modifier.fillMaxSize())
                        ScanGuide(animate = previewActive && !reading, lineVisible = !reading)
                    }
                }
            }
        }
        Column(
            Modifier.fillMaxWidth().padding(horizontal = 20.dp, vertical = 16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            photoError?.let { Callout(it, warning = false) }
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Secondary(
                    if (torchOn) "关闭手电筒" else "手电筒",
                    toggleTorch,
                    Modifier.weight(1f).semantics { stateDescription = if (torchOn) "已开启" else "已关闭" },
                    enabled = torchAvailable && !reading,
                    icon = R.drawable.monitor_flashlight,
                )
                Action(
                    if (reading) "正在识别…" else "从相册选择",
                    choosePhoto,
                    enabled = !reading,
                    modifier = Modifier.weight(1.45f),
                    icon = R.drawable.monitor_photo,
                )
            }
        }
    }
}

@Composable
private fun ScanGuide(animate: Boolean, lineVisible: Boolean) {
    val accent = LocalPalette.current.accent
    val progress = if (animate) {
        val transition = rememberInfiniteTransition(label = "Scan guide")
        val fraction by transition.animateFloat(
            0.22f, 0.78f, infiniteRepeatable(tween(2400), RepeatMode.Reverse), label = "Scan line",
        )
        fraction
    } else 0.5f
    Canvas(Modifier.fillMaxSize()) {
        val inset = 4.dp.toPx()
        val length = 28.dp.toPx()
        val width = 3.dp.toPx()
        for (x in listOf(inset, size.width - inset)) {
            for (y in listOf(inset, size.height - inset)) {
                val corner = Offset(x, y)
                val endX = x + if (x == inset) length else -length
                val endY = y + if (y == inset) length else -length
                drawLine(accent, corner, Offset(endX, y), width, StrokeCap.Round)
                drawLine(accent, corner, Offset(x, endY), width, StrokeCap.Round)
            }
        }
        if (lineVisible) {
            val y = size.height * progress
            drawLine(accent.copy(alpha = 0.5f), Offset(length, y), Offset(size.width - length, y), 1.dp.toPx())
        }
    }
}
