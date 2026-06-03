package com.vanillapay.monitor.work

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.vanillapay.monitor.net.HeartbeatReporter

class HeartbeatWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        HeartbeatReporter(applicationContext).send()
        return Result.success()
    }
}
