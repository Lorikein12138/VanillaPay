package com.vanillapay.monitor

import android.app.Application
import com.vanillapay.monitor.util.AppLog
import com.vanillapay.monitor.util.CrashHandler

/** Process entry point: stand up the persistent logger and crash capture before anything else. */
class MonitorApp : Application() {
    override fun onCreate() {
        super.onCreate()
        AppLog.init(this)
        CrashHandler.install()
    }
}
