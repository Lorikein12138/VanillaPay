package com.vanillapay.monitor.util

/**
 * Catches otherwise-fatal uncaught exceptions, persists the full stack trace via [AppLog]
 * (so it can be exported from the diagnostics screen after relaunch), then delegates to the
 * previously installed handler so the system still records / surfaces the crash normally.
 */
object CrashHandler : Thread.UncaughtExceptionHandler {
    private var previous: Thread.UncaughtExceptionHandler? = null

    fun install() {
        if (Thread.getDefaultUncaughtExceptionHandler() === this) return
        previous = Thread.getDefaultUncaughtExceptionHandler()
        Thread.setDefaultUncaughtExceptionHandler(this)
    }

    override fun uncaughtException(thread: Thread, error: Throwable) {
        runCatching { AppLog.e("Crash", "FATAL on thread '${thread.name}'", error) }
        previous?.uncaughtException(thread, error)
    }
}
