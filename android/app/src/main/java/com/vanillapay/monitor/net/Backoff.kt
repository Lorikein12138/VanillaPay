package com.vanillapay.monitor.net

object Backoff {
    private val schedule = longArrayOf(5, 15, 30, 60, 120, 300)

    fun delaySeconds(attempt: Int): Long {
        val index = (attempt - 1).coerceIn(0, schedule.size - 1)
        return schedule[index]
    }
}
