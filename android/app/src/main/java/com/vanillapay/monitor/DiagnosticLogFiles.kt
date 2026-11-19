package com.vanillapay.monitor

import android.content.ClipData
import android.content.ContentResolver
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.core.content.FileProvider
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File
import java.io.IOException

class DiagnosticLogFileProvider : FileProvider()

private fun Uri.isDocumentUri() = scheme == ContentResolver.SCHEME_CONTENT && !authority.isNullOrBlank()

internal class DiagnosticLogFiles(private val context: Context) {
    private val directory get() = File(context.cacheDir, "diagnostic-log-shares")
    private var pendingShare: File? = null

    fun writeDocument(uri: Uri, text: String) {
        require(uri.isDocumentUri())
        val stream = context.contentResolver.openOutputStream(uri, "wt")
            ?: throw IOException("Document provider returned no output stream")
        stream.use { it.write(text.toByteArray(Charsets.UTF_8)) }
    }

    @Synchronized
    fun createShare(): File {
        check(pendingShare == null)
        prune()
        val folder = directory
        if (!folder.mkdirs() && !folder.isDirectory) throw IOException("Unable to create diagnostic share directory")
        return File.createTempFile(SHARE_PREFIX, ".txt", folder).also { pendingShare = it }
    }

    @Synchronized
    fun finishShare(file: File, handedOff: Boolean) {
        check(file == pendingShare)
        try {
            // Reserve space for the new file only after chooser launch succeeds.
            if (handedOff) prune(System.currentTimeMillis(), MAX_SHARED_FILES - 1) else file.delete()
        } finally {
            pendingShare = null
        }
    }

    @Synchronized
    fun prune(now: Long = System.currentTimeMillis()) = prune(now, MAX_SHARED_FILES)

    private fun prune(now: Long, maxFiles: Int) {
        val files = directory.listFiles { file ->
            file != pendingShare && file.isFile && file.name.startsWith(SHARE_PREFIX) && file.extension == "txt"
        }.orEmpty().sortedByDescending { it.lastModified() }
        files.forEachIndexed { index, file ->
            if (index >= maxFiles || file.lastModified() <= now - SHARE_TTL_MS) file.delete()
        }
    }

    fun chooser(file: File): Intent {
        val uri = FileProvider.getUriForFile(context, "${context.packageName}.diagnostic-log-files", file)
        val send = Intent(Intent.ACTION_SEND).apply {
            type = "text/plain"
            putExtra(Intent.EXTRA_STREAM, uri)
            clipData = ClipData.newRawUri("连接诊断日志", uri)
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        }
        return Intent.createChooser(send, "分享连接诊断")
    }

    companion object {
        const val SHARE_PREFIX = "connection-diagnostics-"
        const val SHARE_TTL_MS = 24 * 60 * 60 * 1_000L
        const val MAX_SHARED_FILES = 8
    }
}

// Action state lives with the app. Only the share operation uses the Activity's scope to open its chooser.
internal class DiagnosticLogActions(
    private val diagnostics: MonitorDiagnostics,
    private val files: DiagnosticLogFiles,
    private val appScope: CoroutineScope,
) {
    var busy by mutableStateOf(false)
        private set
    var message by mutableStateOf<String?>(null)
        private set

    fun pickerOpened() { message = null }

    fun pickerCancelled() { message = "已取消导出。" }

    fun pickerFailed() { message = "无法打开文件保存位置，请重试。" }

    fun export(uri: Uri) = run(appScope, "无法导出日志文件，请重试。") {
        require(uri.isDocumentUri())
        withSnapshot { text ->
            withContext(Dispatchers.IO) { files.writeDocument(uri, text) }
            "已导出日志文件。"
        }
    }

    fun share(scope: CoroutineScope, present: (Intent) -> Unit) = run(scope, "无法分享日志文件，请重试。") {
        withSnapshot { text ->
            var file: File? = null
            var handedOff = false
            try {
                val chooser = withContext(Dispatchers.IO) {
                    val created = files.createShare()
                    // Keep the reference even if cancellation discards the IO block's return value.
                    file = created
                    created.writeText(text, Charsets.UTF_8)
                    files.chooser(created)
                }
                present(chooser)
                handedOff = true
                null
            } finally {
                file?.let { created ->
                    withContext(NonCancellable + Dispatchers.IO) { files.finishShare(created, handedOff) }
                }
            }
        }
    }

    fun clear() = run(appScope, "无法清除诊断日志，请重试。") {
        if (!diagnostics.clear()) throw IOException("Unable to clear diagnostic log")
        "已清除连接诊断日志。"
    }

    private suspend fun withSnapshot(action: suspend (String) -> String?): String? {
        val text = diagnostics.snapshot()?.text ?: return "无法读取诊断日志，请重试。"
        if (text.isEmpty()) return "暂无可导出的诊断日志。"
        return action(text)
    }

    private fun run(scope: CoroutineScope, failure: String, action: suspend () -> String?) {
        if (busy) return
        busy = true
        message = null
        scope.launch(start = CoroutineStart.UNDISPATCHED) {
            try {
                ensureActive()
                message = action()
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                message = failure
            } finally {
                busy = false
            }
        }
    }
}
