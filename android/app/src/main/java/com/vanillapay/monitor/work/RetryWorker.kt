package com.vanillapay.monitor.work

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.vanillapay.monitor.service.Reporter
import com.vanillapay.monitor.util.ClockSync

class RetryWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        Reporter(applicationContext, ClockSync()).drain()
        return Result.success()
    }
}
