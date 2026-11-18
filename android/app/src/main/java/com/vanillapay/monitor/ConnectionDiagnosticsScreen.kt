package com.vanillapay.monitor

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

@Composable
internal fun ConnectionDiagnosticsRoute(
    diagnostics: MonitorDiagnostics,
    access: Boolean,
    monitoring: Boolean,
    online: Boolean,
    back: () -> Unit,
    copy: (String) -> Unit,
    share: (String) -> Boolean,
) {
    val scope = rememberCoroutineScope()
    var snapshot by remember { mutableStateOf<DiagnosticSnapshot?>(null) }
    var loading by remember { mutableStateOf(true) }
    var busy by remember { mutableStateOf(false) }
    var message by remember { mutableStateOf<String?>(null) }
    var confirmClear by remember { mutableStateOf(false) }

    LaunchedEffect(diagnostics, busy) {
        if (busy) return@LaunchedEffect
        while (true) {
            snapshot = diagnostics.snapshot()
            loading = false
            delay(5_000)
        }
    }

    fun act(operation: suspend () -> String) {
        if (busy) return
        busy = true
        scope.launch {
            try {
                message = operation()
            } finally {
                busy = false
            }
        }
    }

    fun export(send: (String) -> String) = act {
        snapshot = diagnostics.snapshot()
        val text = snapshot?.text
        when {
            text == null -> "无法读取诊断日志，请重试。"
            text.isEmpty() -> "暂无可导出的诊断日志。"
            else -> send(text)
        }
    }

    ConnectionDiagnosticsScreen(access, monitoring, online, snapshot, loading, busy, message, back,
        copy = {
            export { text ->
                if (runCatching { copy(text) }.isSuccess) "已复制脱敏日志。" else "无法复制日志，请重试。"
            }
        },
        share = { export { if (share(it)) "" else "无法打开系统分享，请重试。" } },
        clear = { confirmClear = true },
    )

    if (confirmClear) {
        MonitorDialog("清除连接诊断日志？", { confirmClear = false },
            confirmButton = {
                TextAction("清除日志", {
                    confirmClear = false
                    act {
                        if (diagnostics.clear()) {
                            snapshot = diagnostics.snapshot()
                            "已清除连接诊断日志。"
                        } else "无法清除诊断日志，请重试。"
                    }
                }, danger = true)
            },
            dismissButton = { TextAction("取消", { confirmClear = false }) },
        ) {
            Paragraph("清除本机已保存的连接诊断日志。之后发生的连接事件仍会记录。")
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
    copy: () -> Unit,
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
    val enabled = hasLogs && !busy
    val button = Modifier.fillMaxWidth().heightIn(min = 48.dp)
    Screen(topBar = { BackHeader("连接诊断", back) }) {
        Paragraph("记录心跳和监听状态，最多保留 24 小时。日志仅存本机，需手动复制或分享。")
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
        Action("复制脱敏日志", copy, enabled, button)
        Secondary("系统分享", share, button, enabled = enabled)
        Secondary("清除日志", clear, button, enabled = !busy && !loading && (hasLogs || snapshot == null), danger = true)
    }
}
