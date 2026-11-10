package com.vanillapay.monitor

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AlertDialogDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
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
    Screen {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Image(painterResource(R.drawable.vanilla_club), null, Modifier.size(24.dp))
            Text("VanillaPay Monitor", color = LocalPalette.current.accent, fontSize = 13.sp, fontWeight = FontWeight.Medium)
        }
        Headline("绑定这台手机")
        Paragraph("在商户后台「监控设备」页生成二维码或绑定码，5 分钟内有效。每个商户只绑定一台手机。")
        Action("扫码绑定", scan, !busy, Modifier.fillMaxWidth().height(48.dp), R.drawable.monitor_scan)
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Box(Modifier.weight(1f)) { Rule() }
            Caption("或输入绑定码")
            Box(Modifier.weight(1f)) { Rule() }
        }
        Input(code, onCode, "绑定码", !busy, KeyboardOptions(capitalization = KeyboardCapitalization.Characters), monospace = true)
        Input(name, onName, "设备名称", !busy, hint = "显示在商户后台，方便认出是哪台手机。")
        Input(base, onBase, "站点地址", !busy, KeyboardOptions(keyboardType = KeyboardType.Uri),
            hint = "默认是构建时指定的站点；扫码绑定会自动填写")
        error?.let { Callout(it, warning = false) }
        Secondary(if (busy) "正在绑定…" else "用绑定码绑定", bind, Modifier.fillMaxWidth().height(48.dp), enabled = !busy)
        TextAction("通知识别测试", diagnostics, !busy)
    }
}

@Composable
private fun Input(
    value: String, change: (String) -> Unit, label: String, enabled: Boolean,
    keyboard: KeyboardOptions = KeyboardOptions.Default, hint: String? = null, monospace: Boolean = false,
) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(label, color = LocalPalette.current.secondary, fontSize = 13.sp, fontWeight = FontWeight.Medium)
        val style = MaterialTheme.typography.bodyLarge
        val surface = MaterialTheme.colorScheme.surface
        OutlinedTextField(value, change, Modifier.fillMaxWidth().semantics { contentDescription = label }, enabled = enabled,
            textStyle = if (monospace) style.copy(fontFamily = mono, letterSpacing = 1.5.sp) else style,
            colors = OutlinedTextFieldDefaults.colors(focusedContainerColor = surface, unfocusedContainerColor = surface),
            singleLine = true, keyboardOptions = keyboard, shape = RoundedCornerShape(8.dp))
        hint?.let { Caption(it) }
    }
}

internal data class SetupChecks(
    val access: Boolean, val posting: Boolean, val battery: Boolean, val startup: Boolean, val sources: Boolean,
)

internal data class StartupGuide(val startup: String, val background: String? = null)

internal fun startupGuide(manufacturer: String): StartupGuide = if (isVivoManufacturer(manufacturer)) {
    StartupGuide(
        startup = "i 管家 → 应用管理 → 自启动，开启 VanillaPay Monitor。",
        background = "未开启时，切到后台约 1 分钟后可能停止监听，到账要等打开 app 才上报。\n" +
            "设置 → 电池 → 后台耗电管理 → VanillaPay Monitor → 允许后台高耗电",
    )
} else {
    StartupGuide(
        "小米：手机管家 → 应用管理 → 自启动。\n" +
            "华为：设置 → 应用启动管理，手动允许后台运行。\n" +
            "OPPO：手机管家 → 权限隐私 → 自启动管理。\n" +
            "vivo：i 管家 → 应用管理 → 自启动。另需在设置 → 电池 → 后台耗电管理 → " +
            "VanillaPay Monitor 中选择「允许后台高耗电」。",
    )
}

@Composable
internal fun StartupGuideDialog(
    guide: StartupGuide, openStartup: () -> Unit, openBackground: () -> Unit,
    onConfirm: () -> Unit, onDismiss: () -> Unit,
) {
    val background = guide.background
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = if (background == null) AlertDialogDefaults.containerColor else MaterialTheme.colorScheme.surface,
        title = { Text("自启动与后台运行") },
        text = {
            if (background == null) {
                Text(guide.startup)
            } else {
                Column(
                    Modifier.verticalScroll(rememberScrollState()),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text("第一步：自启动", style = MaterialTheme.typography.titleMedium)
                    Paragraph(guide.startup)
                    TextAction("打开自启动设置", openStartup)
                    Rule()
                    Text("第二步：允许后台高耗电", style = MaterialTheme.typography.titleMedium)
                    Paragraph(background)
                    TextAction("打开后台耗电设置", openBackground)
                }
            }
        },
        confirmButton = {
            if (background == null) TextAction("去设置", openStartup)
            else TextAction("我已确认", onConfirm)
        },
        dismissButton = {
            if (background == null) TextAction("我已确认", onConfirm)
        },
    )
}

@Composable
internal fun SetupScreen(
    checks: SetupChecks, openAccess: () -> Unit, openPosting: () -> Unit, openBattery: () -> Unit,
    openStartup: () -> Unit, confirmSources: () -> Unit, start: () -> Unit,
    busy: Boolean = false, error: String? = null,
) {
    Screen {
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
        val indicator = Modifier.size(22.dp)
        Box(if (done) indicator.background(palette.good, CircleShape)
            else indicator.border(1.dp, MaterialTheme.colorScheme.outline, CircleShape),
            contentAlignment = Alignment.Center) {
            if (done) Icon(painterResource(R.drawable.monitor_check), null, Modifier.size(15.dp),
                tint = MaterialTheme.colorScheme.background)
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
    Screen(topBar = { HomeHeader(hide, settings) }) {
        val status = when {
            !access -> "已暂停"
            online -> "在线 · 心跳 ${((now - (state.lastHeartbeat ?: now)) / 1000).coerceAtLeast(0)} 秒前"
            else -> "离线"
        }
        Panel(PaddingValues(20.dp), gap = 14) {
            StatusLabel(status, if (online) palette.good else palette.wait, halo = online)
            Headline(if (access) "正在监听到账" else "通知监听已暂停")
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Metric("今日上报", reported, Modifier.weight(1f), MaterialTheme.colorScheme.onSurface)
                Metric("待上传", pending, Modifier.weight(1f),
                    pendingUploadColor(pending, palette.accent, MaterialTheme.colorScheme.onSurface))
            }
            if (!access) TextAction("重新开启通知读取权限", restoreAccess)
        }
        if (state.banned) Callout("商户已被暂停收款，到账仍会上报留档，但不会自动匹配订单")
        Panel(PaddingValues(horizontal = 16.dp, vertical = 6.dp), gap = 0) {
            Channel.entries.forEachIndexed { index, channel ->
                if (index > 0) Rule()
                Row(Modifier.height(48.dp), verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    val enabled = channel in channels && access
                    ChannelIcon(channel, size = 28.dp, tone = ChannelTone.Brand, enabled = enabled)
                    Text(channel.display, Modifier.weight(1f))
                    val label = when {
                        channel !in channels -> "未安装"
                        access -> "通知可读取"
                        else -> "读取已暂停"
                    }
                    StatusLabel(label, if (enabled) palette.good else palette.subtle)
                }
            }
        }
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(Modifier.fillMaxWidth().padding(horizontal = 4.dp), horizontalArrangement = Arrangement.SpaceBetween) {
                Text("最近到账", style = MaterialTheme.typography.titleMedium.copy(fontSize = 15.sp))
                Caption("只显示本机最近 50 条")
            }
            Panel(PaddingValues(horizontal = 16.dp, vertical = 4.dp), gap = 0) {
                if (receipts.isEmpty()) Box(Modifier.padding(vertical = 10.dp)) { Caption("暂无到账记录") }
                receipts.forEachIndexed { index, receipt ->
                    if (index > 0) Rule()
                    ReceiptRow(receipt)
                }
            }
        }
    }
}

@Composable
private fun HomeHeader(hide: () -> Unit, settings: () -> Unit) {
    Row(Modifier.fillMaxWidth().padding(start = 20.dp, end = 12.dp, top = 12.dp),
        horizontalArrangement = Arrangement.spacedBy(4.dp), verticalAlignment = Alignment.CenterVertically) {
        Row(Modifier.weight(1f), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
            Image(painterResource(R.drawable.vanilla_club), null, Modifier.size(28.dp))
            Text("VanillaPay", fontFamily = serif, fontSize = 20.sp, maxLines = 1)
            Text("Monitor", color = LocalPalette.current.accent, fontSize = 12.sp, fontWeight = FontWeight.Medium, maxLines = 1)
        }
        Secondary("隐藏", hide, Modifier.height(36.dp), compact = true, icon = R.drawable.monitor_eye_off)
        HeaderIcon(R.drawable.monitor_settings, "设置", settings)
    }
}

@Composable
private fun StatusLabel(label: String, dot: Color, halo: Boolean = false) {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        Box(Modifier.size(if (halo) 18.dp else 7.dp).background(dot.copy(alpha = if (halo) 0.18f else 1f), CircleShape),
            contentAlignment = Alignment.Center) {
            if (halo) Box(Modifier.size(10.dp).background(dot, CircleShape))
        }
        Text(label, color = LocalPalette.current.secondary, fontSize = 13.sp)
    }
}

@Composable
private fun Metric(label: String, value: Int, modifier: Modifier, color: Color) {
    Column(modifier, verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Caption(label)
        Text("$value 笔", color = color, fontFamily = serif, fontSize = 20.sp)
    }
}

@Composable
internal fun ReceiptRow(receipt: Receipt) {
    val palette = LocalPalette.current
    val channel = Channel.entries.first { it.wire == receipt.channel }
    val (status, color, detail) = when (receipt.status) {
        MATCHED -> Triple("已匹配", palette.good, receipt.detail?.let { "订单 $it" })
        QUEUED -> Triple("待上传", palette.wait, "网络恢复后自动重试")
        UNMATCHED -> Triple("未匹配", palette.secondary, "到账未自动匹配订单")
        else -> Triple("上报失败", palette.danger, receiptFailureDescription(receipt.detail, receipt.cents))
    }
    Column(Modifier.fillMaxWidth().padding(vertical = 10.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(yuan(receipt.cents), Modifier.alignByBaseline(), fontFamily = mono, fontSize = 16.sp, maxLines = 1)
            Row(
                Modifier.weight(1f).alignByBaseline(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                ChannelIcon(channel, size = 14.dp, tone = ChannelTone.Plain)
                Text(channel.display, color = palette.secondary, fontSize = 13.sp,
                    maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            Text(localTime(receipt.occurredAt, "HH:mm:ss"), Modifier.alignByBaseline(), color = palette.subtle,
                fontFamily = mono, fontSize = 12.sp, maxLines = 1)
        }
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Pill(status, color)
            detail?.let {
                Text(it, color = palette.secondary, style = MaterialTheme.typography.bodySmall,
                    maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

internal fun receiptFailureDescription(code: String?, cents: Long) = when {
    code == "EVENT_CONFLICT" -> "与已上报记录冲突"
    code == "VALIDATION_ERROR" && cents !in 1..999_999_999 -> "金额超出范围，服务器未接收"
    else -> "服务器拒绝了这条记录"
}

@Composable
internal fun SettingsScreen(
    session: Credential, back: () -> Unit, checkUpdate: () -> Unit, diagnostics: () -> Unit, unbind: () -> Unit,
    updateBusy: Boolean = false, message: String? = null,
) {
    Screen(topBar = { BackHeader("设置", back) }, footer = {
        Column(Modifier.fillMaxWidth().padding(start = 20.dp, end = 20.dp, bottom = 20.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            Secondary("解绑这台手机", unbind, Modifier.fillMaxWidth().height(48.dp), danger = true)
            Text("解绑后本机停止上报，需要在商户后台重新生成绑定码。", color = LocalPalette.current.subtle,
                style = MaterialTheme.typography.bodySmall, textAlign = TextAlign.Center)
        }
    }) {
        SettingsGroup("本机", listOf(
            "设备名称" to session.deviceName, "设备 ID" to session.deviceId,
            "绑定时间" to localTime(session.boundAt, "yyyy-MM-dd HH:mm"),
        ))
        SettingsGroup("连接", listOf("站点" to (siteUrl(session.base)?.host ?: session.base), "心跳间隔" to "${session.interval} 秒"))
        SettingsGroup("应用", listOf("名称" to "VanillaPay Monitor", "版本" to BuildConfig.VERSION_NAME)) {
            Rule()
            SettingsRow("检查更新", if (updateBusy) "检查中…" else "打开下载页", click = checkUpdate, enabled = !updateBusy)
            Rule()
            SettingsRow("通知识别测试", click = diagnostics)
            message?.let { Box(Modifier.padding(bottom = 8.dp)) { Caption(it) } }
        }
    }
}

@Composable
private fun SettingsGroup(title: String, values: List<Pair<String, String>>, actions: (@Composable () -> Unit)? = null) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(title, Modifier.padding(horizontal = 4.dp), color = LocalPalette.current.subtle, fontSize = 13.sp,
            fontWeight = FontWeight.Medium)
        Panel(PaddingValues(horizontal = 16.dp, vertical = 2.dp), gap = 0) {
            values.forEachIndexed { index, (label, value) ->
                if (index > 0) Rule()
                SettingsRow(label, value, monospace = label == "设备 ID" || label == "版本")
            }
            actions?.invoke()
        }
    }
}

@Composable
private fun SettingsRow(label: String, value: String? = null, monospace: Boolean = false,
    click: (() -> Unit)? = null, enabled: Boolean = true) {
    val action = if (click == null) Modifier else Modifier.clickable(enabled = enabled, role = Role.Button, onClick = click)
    val secondary = LocalPalette.current.secondary
    Row(Modifier.fillMaxWidth().height(48.dp).then(action), verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(label, fontSize = 15.sp)
        if (value != null) {
            Text(value, Modifier.weight(1f), color = secondary, fontSize = 14.sp, fontFamily = if (monospace) mono else null,
                maxLines = 1, overflow = TextOverflow.Ellipsis, textAlign = TextAlign.End)
        } else {
            Box(Modifier.weight(1f), contentAlignment = Alignment.CenterEnd) {
                Icon(painterResource(R.drawable.monitor_forward), null, Modifier.size(16.dp), tint = secondary)
            }
        }
    }
}

@Composable
internal fun DiagnosticsScreen(
    access: Boolean, entries: List<Diagnostic>, back: () -> Unit, enable: () -> Unit, copy: (String) -> Unit,
) {
    Screen(topBar = { BackHeader("通知识别测试", back) }) {
        Paragraph("显示本机最近 50 条相关通知及识别结果，仅保留 7 天。这些测试记录只存本机。")
        if (!access) {
            Callout("请先开启通知读取权限，再让收款 app 发一条到账通知。")
            TextAction("去开启通知读取", enable)
        }
        if (entries.isEmpty()) Panel { Caption("暂无可测试的通知") }
        entries.forEach { entry ->
            val channel = Channel.entries.first { it.wire == entry.channel }
            Panel {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    ChannelIcon(channel, size = 14.dp, tone = ChannelTone.Plain)
                    Caption("${channel.display} · ${localTime(entry.postTime, "MM-dd HH:mm:ss")}")
                }
                val cents = entry.cents
                Pill(if (cents == null) "未识别" else "识别为 ${channel.display} · ${yuan(cents)}",
                    if (cents == null) LocalPalette.current.secondary else LocalPalette.current.good)
                Column {
                    Text(entry.raw)
                    TextAction("复制原文", { copy(entry.raw) })
                }
            }
        }
    }
}
