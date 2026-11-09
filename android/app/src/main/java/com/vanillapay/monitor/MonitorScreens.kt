package com.vanillapay.monitor

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vanillapay.monitor.parse.Channel

@Composable
internal fun BindScreen(
    code: String, name: String, base: String, busy: Boolean, error: String?,
    onCode: (String) -> Unit, onName: (String) -> Unit, onBase: (String) -> Unit,
    scan: () -> Unit, bind: () -> Unit, diagnostics: () -> Unit,
) {
    Screen("绑定设备") {
        Image(painterResource(R.drawable.vanilla_club), null, Modifier.size(50.dp))
        Headline("绑定这台手机")
        Paragraph("在商户后台「监控设备」页生成二维码或绑定码，5 分钟内有效。每个商户只绑定一台手机。")
        Action("扫码绑定", scan, !busy, Modifier.fillMaxWidth())
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Box(Modifier.weight(1f)) { Rule() }
            Caption("或输入绑定码")
            Box(Modifier.weight(1f)) { Rule() }
        }
        Input(code, onCode, "绑定码", !busy, KeyboardOptions(capitalization = KeyboardCapitalization.Characters))
        Column {
            Input(name, onName, "设备名称", !busy)
            Caption("显示在商户后台，方便认出是哪台手机。")
        }
        Column {
            Input(base, onBase, "站点地址", !busy, KeyboardOptions(keyboardType = KeyboardType.Uri))
            Caption("默认是构建时指定的站点；扫码绑定会自动填写")
        }
        error?.let { Callout(it, warning = false) }
        Action(if (busy) "正在绑定…" else "用绑定码绑定", bind, !busy, Modifier.fillMaxWidth())
        TextAction("通知识别测试", diagnostics, !busy)
    }
}

@Composable
private fun Input(
    value: String, change: (String) -> Unit, label: String, enabled: Boolean,
    keyboard: KeyboardOptions = KeyboardOptions.Default,
) {
    OutlinedTextField(value, change, Modifier.fillMaxWidth(), enabled = enabled, label = { Text(label) },
        singleLine = true, keyboardOptions = keyboard, shape = RoundedCornerShape(8.dp))
}

internal data class SetupChecks(
    val access: Boolean, val posting: Boolean, val battery: Boolean, val startup: Boolean, val sources: Boolean,
)

@Composable
internal fun SetupScreen(
    checks: SetupChecks, openAccess: () -> Unit, openPosting: () -> Unit, openBattery: () -> Unit,
    openStartup: () -> Unit, confirmSources: () -> Unit, start: () -> Unit,
    busy: Boolean = false, error: String? = null,
) {
    Screen("权限检查") {
        Caption("第 2 步，共 2 步")
        Headline("开始监听之前")
        Paragraph("这些设置决定手机锁屏后还能不能收到到账通知。建议用一台常年插电的备用机。")
        Panel {
            CheckRow("读取通知", "识别支付宝、微信、QQ 的到账通知。必须开启。", checks.access, "去开启", openAccess)
            Rule()
            CheckRow("允许发送通知", "通知栏常驻「正在监听」，点开回到 app。", checks.posting, "去开启", openPosting)
            Rule()
            CheckRow("忽略电池优化", "避免系统在锁屏后停掉监听。", checks.battery, "去设置", openBattery)
            Rule()
            CheckRow("自启动与后台运行", "小米、华为、OPPO、vivo 等需要在手机管家里手动允许。", checks.startup, "查看方法", openStartup)
            Rule()
            CheckRow("收款 app 的到账通知", "在支付宝、微信里打开到账提醒，并允许它们发通知。", checks.sources, "我已确认", confirmSources)
        }
        error?.let { Callout(it, warning = false) }
        Action("开始监听", start, checks.access && !busy, Modifier.fillMaxWidth())
        Caption("读取通知是必须的，其余几项没开也能用，但锁屏后可能漏单。开始后 app 会从最近任务里隐藏，只在通知栏常驻。")
    }
}

@Composable
private fun CheckRow(title: String, description: String, done: Boolean, action: String, onClick: () -> Unit) {
    val palette = LocalPalette.current
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(9.dp)) {
        Box(Modifier.size(25.dp).background((if (done) palette.good else palette.accent).copy(alpha = 0.13f), CircleShape),
            contentAlignment = Alignment.Center) {
            Text(if (done) "✓" else "·", color = if (done) palette.good else palette.accent)
        }
        Column(Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.titleMedium)
            Caption(description)
        }
        if (done) Caption("已完成") else TextAction(action, onClick)
    }
}

@Composable
internal fun HomeScreen(
    state: MonitorState, access: Boolean, channels: Set<Channel>, receipts: List<Receipt>,
    reported: Int, pending: Int, now: Long, hide: () -> Unit, settings: () -> Unit, restoreAccess: () -> Unit,
) {
    val palette = LocalPalette.current
    val online = access && state.online
    Screen("运行状态", right = {
        TextAction("设置", settings)
        TextAction("隐藏", hide)
    }) {
        val status = when {
            !access -> "已暂停"
            online -> "在线 · 心跳 ${((now - (state.lastHeartbeat ?: now)) / 1000).coerceAtLeast(0)} 秒前"
            else -> "离线"
        }
        Column(verticalArrangement = Arrangement.spacedBy(5.dp)) {
            Text("●  $status", color = if (online) palette.good else palette.wait, style = MaterialTheme.typography.bodySmall)
            Headline(if (access) "正在监听到账" else "通知监听已暂停")
            if (!access) TextAction("重新开启通知读取权限", restoreAccess)
        }
        if (state.banned) Callout("商户已被暂停收款，到账仍会上报留档，但不会自动匹配订单")
        Panel {
            Channel.entries.forEachIndexed { index, channel ->
                if (index > 0) Rule()
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    Box(Modifier.size(32.dp).background(palette.muted, RoundedCornerShape(8.dp)), contentAlignment = Alignment.Center) {
                        Text(channel.mark, color = palette.secondary)
                    }
                    Text(channel.display, Modifier.weight(1f))
                    val label = when {
                        channel !in channels -> "未安装"
                        access -> "通知可读取"
                        else -> "读取已暂停"
                    }
                    Text("●  $label", color = if (channel in channels && access) palette.good else palette.subtle,
                        style = MaterialTheme.typography.bodySmall)
                }
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Metric("今日上报", reported, Modifier.weight(1f))
            Metric("待上传", pending, Modifier.weight(1f))
        }
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text("最近到账", style = MaterialTheme.typography.titleLarge)
            Caption("只显示本机最近 50 条")
        }
        Panel {
            if (receipts.isEmpty()) Caption("暂无到账记录")
            receipts.forEachIndexed { index, receipt ->
                if (index > 0) Rule()
                ReceiptRow(receipt)
            }
        }
    }
}

@Composable
private fun Metric(label: String, value: Int, modifier: Modifier) {
    Column(modifier.background(LocalPalette.current.muted, RoundedCornerShape(10.dp)).padding(13.dp)) {
        Caption(label)
        Text("$value 笔", style = MaterialTheme.typography.titleMedium)
    }
}

@Composable
private fun ReceiptRow(receipt: Receipt) {
    val palette = LocalPalette.current
    val channel = Channel.entries.first { it.wire == receipt.channel }
    val (status, color, detail) = when (receipt.status) {
        MATCHED -> Triple("已匹配", palette.good, receipt.detail?.let { "订单 $it" })
        QUEUED -> Triple("待上传", palette.wait, "网络恢复后自动重试")
        UNMATCHED -> Triple("未匹配", palette.secondary, "到账未自动匹配订单")
        else -> Triple("上报失败", palette.danger, receipt.detail)
    }
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.Top, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Text(yuan(receipt.cents), fontFamily = mono, fontSize = 17.sp)
            Caption("${channel.display} · ${localTime(receipt.occurredAt, "HH:mm:ss")}")
            detail?.let { Caption(it) }
        }
        Pill(status, color)
    }
}

@Composable
internal fun SettingsScreen(
    session: Credential, back: () -> Unit, checkUpdate: () -> Unit, diagnostics: () -> Unit, unbind: () -> Unit,
    updateBusy: Boolean = false, message: String? = null,
) {
    Screen("设置与解绑", left = { TextAction("返回", back) }) {
        Headline("设置")
        SettingsGroup("本机", listOf(
            "设备名称" to session.deviceName, "设备 ID" to session.deviceId,
            "绑定时间" to localTime(session.boundAt, "yyyy-MM-dd HH:mm"),
        ))
        SettingsGroup("连接", listOf("站点" to (siteUrl(session.base)?.host ?: session.base), "心跳间隔" to "${session.interval} 秒"))
        SettingsGroup("应用", listOf("名称" to "VanillaPay Monitor", "版本" to BuildConfig.VERSION_NAME)) {
            Rule()
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text("检查更新", Modifier.weight(1f))
                TextAction(if (updateBusy) "检查中…" else "打开下载页", checkUpdate, !updateBusy)
            }
            Rule()
            TextAction("通知识别测试", diagnostics)
            message?.let { Caption(it) }
        }
        Callout("解绑后本机停止上报，需要在商户后台重新生成绑定码。")
        Secondary("解绑这台手机", unbind, Modifier.fillMaxWidth(), danger = true)
    }
}

@Composable
private fun SettingsGroup(title: String, values: List<Pair<String, String>>, actions: (@Composable () -> Unit)? = null) {
    Column(verticalArrangement = Arrangement.spacedBy(5.dp)) {
        Text(title, style = MaterialTheme.typography.titleMedium)
        Panel {
            values.forEachIndexed { index, (label, value) ->
                if (index > 0) Rule()
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(14.dp)) {
                    Paragraph(label)
                    Text(value, Modifier.weight(1f), fontFamily = if (label == "设备 ID" || label == "版本") mono else null)
                }
            }
            actions?.invoke()
        }
    }
}

@Composable
internal fun DiagnosticsScreen(
    access: Boolean, entries: List<Diagnostic>, back: () -> Unit, enable: () -> Unit, copy: (String) -> Unit,
) {
    Screen("通知识别测试", left = { TextAction("返回", back) }) {
        Headline("通知识别测试")
        Paragraph("显示本机最近 50 条相关通知及识别结果，仅保留 7 天。这些测试记录只存本机。")
        if (!access) {
            Callout("请先开启通知读取权限，再让收款 app 发一条到账通知。")
            TextAction("去开启通知读取", enable)
        }
        if (entries.isEmpty()) Panel { Caption("暂无可测试的通知") }
        entries.forEach { entry ->
            val channel = Channel.entries.first { it.wire == entry.channel }
            Panel {
                Caption("${channel.display} · ${localTime(entry.postTime, "MM-dd HH:mm:ss")}")
                val cents = entry.cents
                Pill(if (cents == null) "未识别" else "识别为 ${channel.display} · ${yuan(cents)}",
                    if (cents == null) LocalPalette.current.secondary else LocalPalette.current.good)
                Text(entry.raw)
                TextAction("复制原文", { copy(entry.raw) })
            }
        }
    }
}
