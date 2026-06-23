package com.vanillapay.monitor.util

import android.annotation.SuppressLint
import android.content.Context
import android.os.Build
import android.util.Log
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * Lightweight on-device logger that persists to a file so logs survive a process death
 * (e.g. the camera failing to open). Mirrors to logcat for `adb`, and exposes the buffer
 * to the in-app diagnostics screen so a user can copy / export it without a cable.
 *
 * Writes are synchronous + flushed under a lock, so a hard crash immediately after a log
 * call still keeps that line on disk. The file rotates at [MAX_BYTES], keeping one backup.
 */
object AppLog {
    private const val TAG_PREFIX = "VanillaPay"
    private const val MAX_BYTES = 256 * 1024L
    private const val DIR = "logs"
    private const val FILE = "app.log"
    private const val BACKUP = "app.log.1"

    private val lock = Any()
    private val timeFormat = SimpleDateFormat("MM-dd HH:mm:ss.SSS", Locale.US)

    @SuppressLint("StaticFieldLeak")
    private var logDir: File? = null

    fun init(context: Context) {
        val dir = File(context.filesDir, DIR).apply { mkdirs() }
        synchronized(lock) { logDir = dir }
        writeHeader(context)
    }

    fun d(tag: String, message: String) = append("D", tag, message, null)

    fun i(tag: String, message: String) = append("I", tag, message, null)

    fun w(tag: String, message: String, error: Throwable? = null) = append("W", tag, message, error)

    fun e(tag: String, message: String, error: Throwable? = null) = append("E", tag, message, error)

    /** Whole buffer (rotated backup + current), oldest-first, for display / export. */
    fun dump(): String = synchronized(lock) {
        val dir = logDir ?: return ""
        buildString {
            File(dir, BACKUP).takeIf { it.isFile }?.let { append(it.readText()) }
            File(dir, FILE).takeIf { it.isFile }?.let { append(it.readText()) }
        }
    }

    fun clear() {
        synchronized(lock) {
            logDir?.let {
                File(it, FILE).delete()
                File(it, BACKUP).delete()
            }
        }
    }

    private fun writeHeader(context: Context) {
        val version = runCatching {
            context.packageManager.getPackageInfo(context.packageName, 0).versionName
        }.getOrNull().orEmpty()
        i(
            "App",
            "=== start v$version | ${Build.MANUFACTURER} ${Build.MODEL} | " +
                "Android ${Build.VERSION.RELEASE}(API ${Build.VERSION.SDK_INT}) | ROM ${Build.DISPLAY} ===",
        )
    }

    private fun append(level: String, tag: String, message: String, error: Throwable?) {
        val logcatTag = "$TAG_PREFIX.$tag"
        when (level) {
            "E" -> Log.e(logcatTag, message, error)
            "W" -> Log.w(logcatTag, message, error)
            "D" -> Log.d(logcatTag, message)
            else -> Log.i(logcatTag, message)
        }
        val dir = synchronized(lock) { logDir } ?: return
        val line = buildString {
            append(timeFormat.format(Date()))
            append(' ').append(level).append('/').append(tag).append(": ").append(message)
            if (error != null) {
                append('\n').append(Log.getStackTraceString(error).trimEnd())
            }
            append('\n')
        }
        synchronized(lock) {
            runCatching {
                val file = File(dir, FILE)
                if (file.length() > MAX_BYTES) {
                    File(dir, BACKUP).delete()
                    file.renameTo(File(dir, BACKUP))
                }
                file.appendText(line)
            }
        }
    }
}
