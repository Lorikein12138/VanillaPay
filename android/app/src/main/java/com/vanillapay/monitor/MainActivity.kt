package com.vanillapay.monitor

import android.Manifest
import android.content.ClipData
import android.content.ClipboardManager
import android.content.ComponentName
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.PersistableBundle
import android.os.PowerManager
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import com.vanillapay.monitor.parse.Channel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import org.json.JSONException
import java.io.IOException

class MainActivity : ComponentActivity() {
    private val app get() = application as MonitorApp
    private var refresh by mutableIntStateOf(0)
    private var reopen by mutableIntStateOf(0)
    private var scanResult by mutableStateOf<String?>(null)
    private var cameraDenied by mutableStateOf(false)
    private val scanner = registerForActivityResult(ScanContract()) { result -> scanResult = result.contents }
    private val camera = registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (granted) openScanner() else cameraDenied = true
    }
    private val notifications = registerForActivityResult(ActivityResultContracts.RequestPermission()) { refresh++ }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent { MonitorTheme { Content() } }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        reopen++
    }

    override fun onResume() {
        super.onResume()
        refresh++
    }

    private fun openScanner() {
        scanner.launch(ScanOptions().setDesiredBarcodeFormats(ScanOptions.QR_CODE)
            .setPrompt("扫描商户后台的绑定二维码").setBeepEnabled(false).setOrientationLocked(false))
    }

    private fun scan() {
        cameraDenied = false
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) openScanner()
        else camera.launch(Manifest.permission.CAMERA)
    }

    private fun appDetails() = Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.parse("package:$packageName"))

    private fun open(intent: Intent): Boolean = runCatching { startActivity(intent) }.isSuccess

    private fun openAccess() {
        if (!open(Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS))) open(appDetails())
    }

    private fun openPosting() {
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED &&
            !requestedPosting) {
            requestedPosting = true
            notifications.launch(Manifest.permission.POST_NOTIFICATIONS)
        } else {
            open(Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, packageName))
        }
    }

    private var requestedPosting = false

    private fun openBattery() {
        if (!open(Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS, Uri.parse("package:$packageName")))) {
            open(appDetails())
        }
    }

    private fun openStartupTargets(targets: List<StartupTarget>) {
        targets.any { target ->
            val intent = when (target) {
                is StartupTarget.Component -> Intent().setComponent(ComponentName.unflattenFromString(target.name))
                is StartupTarget.Action -> Intent(target.name)
                StartupTarget.AppDetails -> appDetails()
            }
            open(intent)
        }
    }

    private fun openStartup() = openStartupTargets(startupTargets(Build.MANUFACTURER))

    private fun openBackgroundUsage() = openStartupTargets(backgroundTargets(Build.MANUFACTURER))

    private fun copy(text: String) {
        val clip = ClipData.newPlainText("通知原文", text)
        clip.description.extras = PersistableBundle().apply { putBoolean("android.content.extra.IS_SENSITIVE", true) }
        getSystemService(ClipboardManager::class.java).setPrimaryClip(clip)
    }

    @Composable
    private fun Content() {
        val state by app.state.collectAsState()
        val scope = rememberCoroutineScope()
        var screen by rememberSaveable { mutableStateOf("home") }
        var diagnosticBack by rememberSaveable { mutableStateOf("home") }
        var dialog by rememberSaveable { mutableStateOf<String?>(null) }
        var code by remember { mutableStateOf("") }
        var name by rememberSaveable { mutableStateOf(Build.MODEL.orEmpty().ifBlank { "Android" }) }
        var base by rememberSaveable { mutableStateOf(BuildConfig.DEFAULT_BASE_URL) }
        var error by remember { mutableStateOf<String?>(null) }
        var busy by remember { mutableStateOf(false) }
        var updateBusy by remember { mutableStateOf(false) }
        var message by remember { mutableStateOf<String?>(null) }
        var startup by rememberSaveable { mutableStateOf(false) }
        var sources by rememberSaveable { mutableStateOf(false) }
        var now by remember { mutableStateOf(System.currentTimeMillis()) }
        val access = remember(refresh, now) { notificationAccess(this) }
        val posting = remember(refresh) { NotificationManagerCompat.from(this).areNotificationsEnabled() }
        val battery = remember(refresh) { getSystemService(PowerManager::class.java).isIgnoringBatteryOptimizations(packageName) }
        val channels = remember(refresh) { installedChannels() }
        val session = state.session
        val dao = app.database.dao()

        fun execute(onError: (Exception) -> Unit = { error = "操作失败，请重试。" }, block: suspend () -> Unit) {
            if (busy) return
            busy = true
            error = null
            // Binding and revocation must commit the server result even if
            // Android recreates this Activity during the request.
            app.scope.launch {
                try { block() } catch (e: CancellationException) { throw e } catch (e: Exception) { onError(e) }
                finally { busy = false }
            }
        }

        fun diagnostics() {
            diagnosticBack = screen
            screen = "diagnostics"
        }

        LaunchedEffect(Unit) {
            while (true) {
                delay(1_000)
                now = System.currentTimeMillis()
            }
        }
        LaunchedEffect(reopen) {
            if (reopen > 0) {
                dialog = null
                screen = "home"
            }
        }
        LaunchedEffect(session?.deviceId, session?.started) {
            if (session == null && screen == "settings") screen = "home"
            if (session?.started == true) runCatching { app.launchService() }
        }
        LaunchedEffect(scanResult) {
            val raw = scanResult ?: return@LaunchedEffect
            scanResult = null
            val pairing = pairingUri(raw)
            if (pairing == null) error = "二维码无效，或站点地址不受支持。"
            else {
                base = pairing.base
                code = pairing.code
                error = null
            }
        }
        LaunchedEffect(cameraDenied) {
            if (cameraDenied) error = "未授权相机，可手动输入绑定码。"
        }
        BackHandler(screen != "home") {
            screen = if (screen == "diagnostics") diagnosticBack else "home"
        }

        when {
            !state.loaded -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
            screen == "diagnostics" -> {
                val entries by remember { dao.diagnostics() }.collectAsState(emptyList())
                LaunchedEffect(Unit) { app.prune() }
                DiagnosticsScreen(access, entries, { screen = diagnosticBack }, ::openAccess, ::copy)
            }
            session == null -> BindScreen(
                code, name, base, busy, error, { code = it }, { name = it }, { base = it }, ::scan,
                bind = {
                    val url = siteUrl(base)
                    val normalized = cleanCode(code)
                    val device = name.trim()
                    when {
                        url == null -> error = "请输入有效的站点地址。正式版只支持 HTTPS。"
                        normalized == null -> error = "请输入 12 位绑定码。"
                        device.isEmpty() || device.codePointCount(0, device.length) > 80 -> error = "请输入有效的设备名称。"
                        else -> execute({ e -> error = bindError(e) }) {
                            app.bind(url.toString(), normalized, device)
                            code = ""
                            screen = "home"
                        }
                    }
                },
                diagnostics = ::diagnostics,
            )
            !session.started -> SetupScreen(
                SetupChecks(access, posting, battery, startup, sources), ::openAccess, ::openPosting,
                ::openBattery, { dialog = "startup" }, { sources = true },
                start = { execute { app.startMonitoring(); finishAndRemoveTask() } }, busy = busy, error = error,
            )
            screen == "settings" -> SettingsScreen(
                session, { screen = "home"; message = null },
                checkUpdate = {
                    if (!updateBusy) {
                        updateBusy = true
                        scope.launch {
                            try {
                                val latest = app.api.latest(session.base)
                                message = when {
                                    latest == null -> "安装包暂未发布"
                                    !latest.newerThan(BuildConfig.VERSION_NAME) -> "已是最新版本"
                                    else -> {
                                        val url = requireNotNull(siteUrl(session.base)).newBuilder()
                                            .addPathSegments("downloads/${latest.file}").build()
                                        if (open(Intent(Intent.ACTION_VIEW, Uri.parse(url.toString())))) "发现新版本 ${latest.version}"
                                        else "无法打开下载页"
                                    }
                                }
                            } catch (e: CancellationException) { throw e } catch (e: Exception) {
                                message = if (e is ApiFailure && e.status == 404) "安装包暂未发布" else "无法检查更新，请重试"
                            } finally { updateBusy = false }
                        }
                    }
                },
                diagnostics = ::diagnostics, unbind = { if (!busy) dialog = "unbind" }, updateBusy = updateBusy,
                message = error ?: message,
            )
            else -> {
                val id = session.deviceId
                val today = remember(now) { todayStart() }
                val receipts by remember(id) { dao.recent(id) }.collectAsState(emptyList())
                val pending by remember(id) { dao.pending(id) }.collectAsState(0)
                val reported by remember(id, today) { dao.reported(id, today) }.collectAsState(0)
                HomeScreen(state, access, channels, receipts, reported, pending, now, ::finishAndRemoveTask,
                    settings = { error = null; screen = "settings" }, restoreAccess = ::openAccess)
            }
        }
        when (dialog) {
            "startup" -> StartupGuideDialog(
                startupGuide(Build.MANUFACTURER), ::openStartup, ::openBackgroundUsage,
                onConfirm = { startup = true; dialog = null }, onDismiss = { dialog = null },
            )
            "unbind", "local-unbind" -> if (session != null) {
                val local = dialog == "local-unbind"
                AlertDialog(
                    onDismissRequest = { if (!busy) dialog = null }, title = { Text("解绑这台手机") },
                    text = { Text(if (local) "无法连接服务器。仍要解绑本机吗？还需要在商户后台删除这台设备，否则会被判为离线。"
                        else "解绑后本机停止上报，需要在商户后台重新生成绑定码。") },
                    confirmButton = {
                        TextAction(if (busy) "正在解绑…" else "确认解绑", {
                            execute({
                                if (it is IOException && it !is ApiFailure) dialog = "local-unbind"
                                else { error = "解绑失败，请重试。"; dialog = null }
                            }) {
                                if (!local) try { app.api.unbind(session) } catch (e: ApiFailure) { if (e.status != 401) throw e }
                                app.forget(session.deviceId)
                                dialog = null
                                screen = "home"
                            }
                        }, !busy)
                    },
                    dismissButton = { TextAction("取消", { dialog = null }, !busy) },
                )
            }
        }
    }

    private fun installedChannels(): Set<Channel> = Channel.entries.filterTo(mutableSetOf()) { channel ->
        try {
            @Suppress("DEPRECATION")
            packageManager.getPackageInfo(channel.packageName, 0)
            true
        } catch (_: PackageManager.NameNotFoundException) { false }
    }

    private fun bindError(error: Exception) = when ((error as? ApiFailure)?.code) {
        "INVALID_CODE" -> "绑定码无效，请检查后重试。"
        "BINDING_USED" -> "绑定码已使用，请在商户后台重新生成。"
        "BINDING_EXPIRED" -> "绑定码已过期，请在商户后台重新生成。"
        "ACCOUNT_BANNED" -> "商户已被暂停收款，无法绑定。"
        "DEVICE_ALREADY_BOUND" -> "已有绑定设备，请在商户后台确认替换。"
        else -> if (error is IOException && error !is ApiFailure) "无法连接服务器，请检查网络和站点地址。"
            else if (error is JSONException) "站点返回了无效的绑定结果。"
            else "绑定失败，请重试。"
    }
}
