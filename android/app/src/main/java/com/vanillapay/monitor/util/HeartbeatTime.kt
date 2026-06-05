package com.vanillapay.monitor.util

import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * Pure, testable formatter for the dashboard heartbeat label. Shows the absolute time of
 * the last heartbeat; the label only changes when a heartbeat actually lands (manual or
 * automatic), so the UI updates on those events rather than ticking continuously.
 */
object HeartbeatTime {
    private val formatter = SimpleDateFormat("MM-dd HH:mm:ss", Locale.getDefault())

    fun format(lastAtMillis: Long): String =
        if (lastAtMillis <= 0L) "尚无心跳" else formatter.format(Date(lastAtMillis))
}
