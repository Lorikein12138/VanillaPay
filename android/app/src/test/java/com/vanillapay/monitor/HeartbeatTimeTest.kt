package com.vanillapay.monitor

import com.vanillapay.monitor.util.HeartbeatTime
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class HeartbeatTimeTest {
    @Test
    fun `zero or negative timestamp shows placeholder`() {
        assertEquals("尚无心跳", HeartbeatTime.format(0L))
        assertEquals("尚无心跳", HeartbeatTime.format(-1L))
    }

    @Test
    fun `positive timestamp shows absolute clock time`() {
        val ts = 1_700_000_000_000L
        val expected = SimpleDateFormat("MM-dd HH:mm:ss", Locale.getDefault()).format(Date(ts))
        assertEquals(expected, HeartbeatTime.format(ts))
    }
}
