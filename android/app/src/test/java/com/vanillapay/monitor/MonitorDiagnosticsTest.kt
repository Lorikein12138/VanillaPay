package com.vanillapay.monitor

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.io.IOException
import java.time.Instant
import java.util.concurrent.atomic.AtomicLong

class MonitorDiagnosticsTest {
    @get:Rule val temp = TemporaryFolder()

    private val now = AtomicLong(Instant.parse("2026-11-18T03:00:00.001Z").toEpochMilli())
    private val elapsed = AtomicLong(0)
    private val heartbeat = Heartbeat(0, 30, online = true, banned = false)

    private fun log(scope: CoroutineScope, directory: File) =
        MonitorDiagnostics(scope, { directory }, now::get, elapsed::get)

    private fun testLog(block: suspend (MonitorDiagnostics, File) -> Unit) = runBlocking {
        val directory = temp.newFolder()
        val scope = CoroutineScope(SupervisorJob())
        try {
            block(log(scope, directory), directory)
        } finally {
            scope.cancel()
        }
    }

    @Test fun countLimitKeepsExactlyTheNewest1024ShortRecords() = testLog { log, directory ->
        repeat(DIAGNOSTIC_MAX_ENTRIES + 1) { index ->
            elapsed.set(index.toLong())
            log.record(DiagnosticEvent.SERVICE_STARTED)
            // Let the bounded writer catch up during a synthetic burst.
            if (index % 64 == 0 || index == DIAGNOSTIC_MAX_ENTRIES - 1) {
                assertEquals((index + 1).coerceAtMost(DIAGNOSTIC_MAX_ENTRIES), log.snapshot()!!.count)
            }
        }
        val snapshot = log.snapshot()!!
        assertEquals(DIAGNOSTIC_MAX_ENTRIES, snapshot.count)
        assertTrue(snapshot.text.lineSequence().first().contains("elapsed_ms=1 "))
        assertTrue(snapshot.text.contains("elapsed_ms=1024 "))
        assertEquals(snapshot.text, File(directory, "monitor-connection.log").readText())
        assertTrue(snapshot.text.length < DIAGNOSTIC_MAX_BYTES)
    }

    @Test fun byteLimitKeepsAsManyNewestRecordsAsFit() = testLog { log, directory ->
        val start = Long.MAX_VALUE - DIAGNOSTIC_MAX_ENTRIES
        repeat(DIAGNOSTIC_MAX_ENTRIES) { index ->
            elapsed.set(start + index)
            log.heartbeatSucceeded(Long.MAX_VALUE, heartbeat)
            if (index % 64 == 0) assertNotNull(log.snapshot())
        }
        val snapshot = log.snapshot()!!
        val file = File(directory, "monitor-connection.log")
        val lineBytes = snapshot.text.lineSequence().first().length + 1
        val expected = DIAGNOSTIC_MAX_BYTES / lineBytes
        assertTrue(expected < DIAGNOSTIC_MAX_ENTRIES)
        assertEquals(expected, snapshot.count)
        assertTrue(snapshot.text.startsWith("${Instant.ofEpochMilli(now.get())} elapsed_ms=${Long.MAX_VALUE - expected} "))
        assertTrue(snapshot.text.contains("elapsed_ms=${Long.MAX_VALUE - 1} "))
        assertTrue(file.length() <= DIAGNOSTIC_MAX_BYTES)
        assertTrue(file.length() + lineBytes > DIAGNOSTIC_MAX_BYTES)
        assertEquals(snapshot.text, file.readText())
    }

    @Test fun restartDoesNotResetAgeAndExpirationPrunesDiskAt24Hours() = testLog { log, directory ->
        elapsed.set(90_000_000)
        log.record(DiagnosticEvent.LISTENER_DISCONNECTED)
        assertEquals(1, log.snapshot()!!.count)
        elapsed.set(2_000)
        now.addAndGet(DIAGNOSTIC_MAX_AGE_MS - 1)
        val restart = SupervisorJob()
        try {
            val reopened = log(CoroutineScope(restart), directory)
            assertEquals(1, reopened.snapshot()!!.count)
            reopened.record(DiagnosticEvent.LISTENER_CONNECTED)
            now.incrementAndGet()
            val snapshot = reopened.snapshot()!!
            assertEquals(1, snapshot.count)
            assertTrue(snapshot.text.contains("elapsed_ms=2000 event=LISTENER_CONNECTED"))
            assertEquals(snapshot.text, File(directory, "monitor-connection.log").readText())
        } finally {
            restart.cancelAndJoin()
        }
    }

    @Test fun exceptionTextNeverEntersDiskOrExport() = testLog { log, directory ->
        val sentinel = "https://private.test/ Bearer T0K3N order-123 account-456 device-789 通知原文"
        log.heartbeatFailed(987, ApiFailure(401, sentinel))
        log.heartbeatFailed(123, ApiFailure(901, sentinel))
        log.heartbeatFailed(456, IOException(sentinel, IllegalStateException(sentinel)))
        log.heartbeatFailed(789, IllegalStateException(sentinel))
        val snapshot = log.snapshot()!!
        assertEquals(4, snapshot.count)
        val fields = snapshot.text.trimEnd().lines().map { it.substringAfter("event=") }
        assertEquals(listOf(
            "HEARTBEAT_HTTP request_ms=987 http_status=401",
            "HEARTBEAT_HTTP request_ms=123 http_status=0",
            "HEARTBEAT_IO request_ms=456",
            "HEARTBEAT_OTHER request_ms=789",
        ), fields)
        assertEquals(snapshot.text, File(directory, "monitor-connection.log").readText())
    }

    @Test fun restartRejectsUnknownFieldsInExistingFileAndCanClearIt() = runBlocking {
        val directory = temp.newFolder()
        val file = File(directory, "monitor-connection.log")
        file.writeText("${Instant.ofEpochMilli(now.get())} elapsed_ms=0 event=SERVICE_CREATED token=SECRET\n")
        val job = SupervisorJob()
        try {
            val reopened = log(CoroutineScope(job), directory)
            assertNull(reopened.snapshot())
            assertTrue(reopened.clear())
            assertFalse(file.exists())
            assertEquals("", reopened.snapshot()!!.text)
        } finally {
            job.cancelAndJoin()
        }
    }

    @Test fun clearingCannotResurrectQueuedRecordsOrAStagedFileOnRestart() = testLog { log, directory ->
        log.record(DiagnosticEvent.SERVICE_CREATED)
        val oldSnapshot = log.snapshot()!!
        val staging = File(directory, "monitor-connection.pending")
        staging.writeText(oldSnapshot.text)
        log.record(DiagnosticEvent.SERVICE_STARTED)
        assertTrue(log.clear())
        assertFalse(staging.exists())
        assertEquals(0, log.snapshot()!!.count)
        log.record(DiagnosticEvent.LISTENER_CONNECTED)
        assertEquals(1, log.snapshot()!!.count)
        val restart = SupervisorJob()
        try {
            val text = log(CoroutineScope(restart), directory).snapshot()!!.text
            assertTrue(text.contains("LISTENER_CONNECTED"))
            assertFalse(text.contains("SERVICE_"))
        } finally {
            restart.cancelAndJoin()
        }
    }

    @Test fun observingHeartbeatPreservesResultsAndErrorsEvenWhenDiskIsUnavailable() = runBlocking {
        val directory = temp.newFile()
        val job = SupervisorJob()
        try {
            val log = log(CoroutineScope(job), directory)
            val failure = ApiFailure(401, "DEVICE_UNAUTHORIZED")
            assertSame(heartbeat, log.observeHeartbeat(91_000, elapsed::get) { heartbeat })
            val thrown = runCatching { log.observeHeartbeat(0, elapsed::get) { throw failure } }.exceptionOrNull()
            assertSame(failure, thrown)
            assertTrue((thrown as ApiFailure).unauthorized)
            assertNull(log.snapshot())
            assertFalse(log.clear())
            assertSame(heartbeat, log.observeHeartbeat(0, elapsed::get) { heartbeat })
        } finally {
            job.cancelAndJoin()
        }
    }

    @Test fun heartbeatRecordsElapsedRequestTimeAndSchedulingDelaySeparately() = testLog { log, _ ->
        elapsed.set(91_001)
        log.observeHeartbeat(91_000, elapsed::get) {
            elapsed.addAndGet(432)
            heartbeat
        }
        val snapshot = log.snapshot()!!
        assertTrue(snapshot.text.contains("elapsed_ms=91001 event=HEARTBEAT_STARTED schedule_delay_ms=91000"))
        assertTrue(snapshot.text.contains("elapsed_ms=91433 event=HEARTBEAT_SUCCESS request_ms=432 online=true banned=false"))
    }
}
