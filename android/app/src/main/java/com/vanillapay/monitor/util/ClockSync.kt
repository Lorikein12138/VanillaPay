package com.vanillapay.monitor.util

class ClockSync {
    @Volatile
    private var offset: Long = 0

    fun sync(serverTime: Long, localTime: Long) {
        offset = serverTime - localTime
    }

    fun now(localTime: Long = System.currentTimeMillis() / 1000): Long = localTime + offset
}
