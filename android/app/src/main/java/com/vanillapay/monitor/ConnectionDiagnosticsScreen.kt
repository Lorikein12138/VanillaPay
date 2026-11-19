package com.vanillapay.monitor

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay

@Composable
internal fun ConnectionDiagnosticsRoute(
    diagnostics: MonitorDiagnostics,
    access: Boolean,
    monitoring: Boolean,
    online: Boolean,
    busy: Boolean,
    message: String?,
    back: () -> Unit,
    export: () -> Unit,
    share: () -> Unit,
    clear: () -> Unit,
) {
    var snapshot by remember { mutableStateOf<DiagnosticSnapshot?>(null) }
    var loading by remember { mutableStateOf(true) }
    var confirmClear by remember { mutableStateOf(false) }

    LaunchedEffect(diagnostics, busy) {
        if (busy) return@LaunchedEffect
        while (true) {
            snapshot = diagnostics.snapshot()
            loading = false
            delay(5_000)
        }
    }

    ConnectionDiagnosticsScreen(access, monitoring, online, snapshot, loading, busy, message, back,
        export = export,
        share = share,
        clear = { confirmClear = true },
    )

    if (confirmClear) {
        MonitorDialog("清除连接诊断日志？", { confirmClear = false },
            confirmButton = {
                TextAction("清除日志", {
                    confirmClear = false
                    clear()
                }, enabled = !busy, danger = true)
            },
            dismissButton = { TextAction("取消", { confirmClear = false }) },
        ) {
            Paragraph("清除本机已保存的连接诊断日志。之后发生的连接事件仍会记录。已导出的文件和接收方已保存的副本不会被收回。")
        }
    }
}

@Composable
internal fun ConnectionDiagnosticsScreen(
    access: Boolean,
    monitoring: Boolean,
    online: Boolean,
    snapshot: DiagnosticSnapshot?,
    loading: Boolean,
    busy: Boolean,
    message: String?,
    back: () -> Unit,
    export: () -> Unit,
    share: () -> Unit,
    clear: () -> Unit,
) {
    val status = when {
        !monitoring -> "未开始监听"
        !access -> "通知读取权限已关闭"
        online -> "心跳在线"
        else -> "心跳离线"
    }
    val hasLogs = (snapshot?.count ?: 0) > 0
    val enabled = !snapshot?.text.isNullOrEmpty() && !busy
    val button = Modifier.fillMaxWidth().heightIn(min = 48.dp)
    Screen(topBar = { BackHeader("连接诊断", back) }) {
        Paragraph("记录心跳和监听状态，最多保留 24 小时。可手动导出或分享脱敏日志文件。")
        Panel {
            Headline(status)
            when {
                loading -> Caption("正在读取诊断日志…")
                snapshot == null -> Caption("无法读取诊断日志。可尝试清除后重新记录。")
                !hasLogs -> Caption("暂无连接诊断日志")
                else -> {
                    Caption("已保存 ${snapshot.count} 条事件")
                    Caption("最近记录 ${snapshot.latestUtc}")
                }
            }
        }
        if (snapshot?.writeFailed == true) Callout("部分诊断日志未能保存。")
        message?.takeIf { it.isNotBlank() }?.let { Callout(it, warning = false) }
        Action("导出日志文件", export, enabled, button)
        Secondary("分享日志文件", share, button, enabled = enabled)
        Secondary("清除日志", clear, button, enabled = !busy && !loading && (hasLogs || snapshot == null), danger = true)
    }
}
