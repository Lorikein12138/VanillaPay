package com.vanillapay.monitor

import android.os.SystemClock
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.launch
import kotlinx.coroutines.selects.onTimeout
import kotlinx.coroutines.selects.select
import org.json.JSONException
import java.io.File
import java.io.IOException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import java.nio.file.Files
import java.nio.file.NoSuchFileException
import java.nio.file.StandardCopyOption
import java.time.Instant
import java.time.format.DateTimeParseException
import javax.net.ssl.SSLException
import kotlin.coroutines.cancellation.CancellationException

internal const val DIAGNOSTIC_MAX_ENTRIES = 1_024
internal const val DIAGNOSTIC_MAX_BYTES = 128 * 1_024
internal const val DIAGNOSTIC_MAX_AGE_MS = 24 * 60 * 60 * 1_000L

internal enum class DiagnosticEvent {
    SERVICE_CREATED, SERVICE_DESTROYED, SERVICE_STARTED, STICKY_RESTART,
    PERMISSION_SKIPPED, PERMISSION_RESTORED, LISTENER_CONNECTED, LISTENER_DISCONNECTED,
    HEARTBEAT_STARTED, HEARTBEAT_SUCCESS, HEARTBEAT_HTTP, HEARTBEAT_TIMEOUT,
    HEARTBEAT_DNS, HEARTBEAT_TLS, HEARTBEAT_IO, HEARTBEAT_RESPONSE_INVALID,
    HEARTBEAT_CANCELLED, HEARTBEAT_OTHER,
}

internal data class DiagnosticSnapshot(
    val count: Int,
    val latestUtc: Instant?,
    val text: String,
    val writeFailed: Boolean = false,
)

// The file and export share this allowlist. A damaged or unrecognized line cannot be exported.
private val diagnosticLine = Regex(
    """(\S+) elapsed_ms=\d{1,19} event=([A-Z_]+)(?: (.*))?""",
)
private val scheduleFields = Regex("""schedule_delay_ms=\d{1,19}""")
private val durationFields = Regex("""request_ms=\d{1,19}""")
private val successFields = Regex("""request_ms=\d{1,19} online=(?:true|false) banned=(?:true|false)""")
private val httpFields = Regex("""request_ms=\d{1,19} http_status=(?:[1-5]\d\d|0)""")

private data class DiagnosticLine(val utc: Instant, val text: String) {
    // The validated format is ASCII, including the terminating newline.
    val bytes get() = text.length + 1

    companion object {
        fun parse(text: String): DiagnosticLine {
            val match = requireNotNull(diagnosticLine.matchEntire(text))
            val utc = Instant.parse(match.groupValues[1])
            require(match.groupValues[1] == utc.toString())
            val fields = match.groupValues[3]
            val valid = when (DiagnosticEvent.valueOf(match.groupValues[2])) {
                DiagnosticEvent.HEARTBEAT_STARTED -> scheduleFields.matches(fields)
                DiagnosticEvent.HEARTBEAT_SUCCESS -> successFields.matches(fields)
                DiagnosticEvent.HEARTBEAT_HTTP -> httpFields.matches(fields)
                DiagnosticEvent.HEARTBEAT_TIMEOUT, DiagnosticEvent.HEARTBEAT_DNS, DiagnosticEvent.HEARTBEAT_TLS,
                DiagnosticEvent.HEARTBEAT_IO, DiagnosticEvent.HEARTBEAT_RESPONSE_INVALID,
                DiagnosticEvent.HEARTBEAT_CANCELLED, DiagnosticEvent.HEARTBEAT_OTHER -> durationFields.matches(fields)
                else -> fields.isEmpty()
            }
            require(valid)
            return DiagnosticLine(utc, text)
        }
    }
}

@OptIn(ExperimentalCoroutinesApi::class)
internal class MonitorDiagnostics(
    scope: CoroutineScope,
    directory: () -> File,
    private val wallClock: () -> Long = System::currentTimeMillis,
    private val elapsedClock: () -> Long = SystemClock::elapsedRealtime,
) {
    private sealed interface Command {
        data class Record(val line: DiagnosticLine) : Command
        data class Read(val answer: CompletableDeferred<DiagnosticSnapshot?>) : Command
        data class Clear(val answer: CompletableDeferred<Boolean>) : Command
        data object Prune : Command
    }

    private val commands = Channel<Command>(capacity = 256)

    init {
        scope.launch(Dispatchers.IO) {
            val store = Store(directory)
            var writeFailed = false
            var command = commands.receive()
            while (true) {
                when (command) {
                    is Command.Record, Command.Prune -> {
                        when (attempt { store.update((command as? Command.Record)?.line) }) {
                            null -> writeFailed = true
                            true -> writeFailed = false
                            false -> Unit
                        }
                    }
                    is Command.Read -> {
                        val snapshot = attempt {
                            if (store.update(null)) writeFailed = false
                            val lines = store.lines
                            DiagnosticSnapshot(lines.size, lines.lastOrNull()?.utc, encode(lines), writeFailed)
                        }
                        if (snapshot == null) writeFailed = true
                        command.answer.complete(snapshot)
                    }
                    is Command.Clear -> {
                        val cleared = attempt { store.clear() } != null
                        writeFailed = !cleared
                        command.answer.complete(cleared)
                    }
                }
                command = select {
                    commands.onReceive { it }
                    val expiresIn = store.lines.firstOrNull()?.let {
                        it.utc.toEpochMilli() + DIAGNOSTIC_MAX_AGE_MS - wallClock()
                    }
                    if (writeFailed || expiresIn != null) {
                        val delayMs = if (writeFailed) 60_000L else requireNotNull(expiresIn).coerceIn(1, 3_600_000)
                        onTimeout(delayMs) { Command.Prune }
                    }
                }
            }
        }
    }

    fun prune() { commands.trySend(Command.Prune) }

    fun record(event: DiagnosticEvent) = enqueue(event)

    fun heartbeatStarted(scheduleDelayMs: Long) =
        enqueue(DiagnosticEvent.HEARTBEAT_STARTED, " schedule_delay_ms=${scheduleDelayMs.coerceAtLeast(0)}")

    fun heartbeatSucceeded(requestMs: Long, result: Heartbeat) = enqueue(
        DiagnosticEvent.HEARTBEAT_SUCCESS,
        " request_ms=$requestMs online=${result.online} banned=${result.banned}",
    )

    fun heartbeatFailed(requestMs: Long, failure: Exception) {
        val event = when (failure) {
            is ApiFailure -> DiagnosticEvent.HEARTBEAT_HTTP
            is SocketTimeoutException -> DiagnosticEvent.HEARTBEAT_TIMEOUT
            is UnknownHostException -> DiagnosticEvent.HEARTBEAT_DNS
            is SSLException -> DiagnosticEvent.HEARTBEAT_TLS
            is IOException -> DiagnosticEvent.HEARTBEAT_IO
            is JSONException, is DateTimeParseException, is IllegalArgumentException ->
                DiagnosticEvent.HEARTBEAT_RESPONSE_INVALID
            is CancellationException -> DiagnosticEvent.HEARTBEAT_CANCELLED
            else -> DiagnosticEvent.HEARTBEAT_OTHER
        }
        val status = (failure as? ApiFailure)?.status?.let {
            " http_status=${it.takeIf { code -> code in 100..599 } ?: 0}"
        }.orEmpty()
        enqueue(event, " request_ms=$requestMs$status")
    }

    suspend fun snapshot(): DiagnosticSnapshot? {
        val answer = CompletableDeferred<DiagnosticSnapshot?>()
        commands.send(Command.Read(answer))
        return answer.await()
    }

    suspend fun clear(): Boolean {
        val answer = CompletableDeferred<Boolean>()
        commands.send(Command.Clear(answer))
        return answer.await()
    }

    private fun enqueue(event: DiagnosticEvent, fields: String = "") {
        val utc = Instant.ofEpochMilli(wallClock())
        val elapsed = elapsedClock().coerceAtLeast(0)
        commands.trySend(Command.Record(DiagnosticLine(utc, "$utc elapsed_ms=$elapsed event=${event.name}$fields")))
    }

    private fun retain(lines: List<DiagnosticLine>, next: DiagnosticLine?): List<DiagnosticLine> {
        val now = wallClock()
        val retained = ArrayDeque<DiagnosticLine>()
        var bytes = 0
        for (line in if (next == null) lines else lines + next) {
            if (line.utc.toEpochMilli() !in (now - DIAGNOSTIC_MAX_AGE_MS + 1)..now) continue
            retained.addLast(line)
            bytes += line.bytes
        }
        while (retained.size > DIAGNOSTIC_MAX_ENTRIES || bytes > DIAGNOSTIC_MAX_BYTES) {
            bytes -= retained.removeFirst().bytes
        }
        return retained.toList()
    }

    private inner class Store(private val directory: () -> File) {
        private var loaded = false
        var lines = emptyList<DiagnosticLine>()
            private set

        fun update(next: DiagnosticLine?): Boolean {
            val file = logFile()
            if (!loaded) {
                Files.deleteIfExists(staging(file).toPath())
                lines = read(file)
                loaded = true
            }
            val retained = retain(lines, next)
            if (lines == retained) return false
            write(file, retained)
            lines = retained
            return true
        }

        fun clear() {
            val file = logFile()
            Files.deleteIfExists(staging(file).toPath())
            Files.deleteIfExists(file.toPath())
            lines = emptyList()
            loaded = true
        }

        private fun read(file: File): List<DiagnosticLine> {
            val size = try { Files.size(file.toPath()) } catch (_: NoSuchFileException) { return emptyList() }
            require(size <= DIAGNOSTIC_MAX_BYTES)
            return file.useLines { rows -> rows.map(DiagnosticLine::parse).toList() }
        }

        private fun write(file: File, lines: List<DiagnosticLine>) {
            if (lines.isEmpty()) {
                Files.deleteIfExists(file.toPath())
                return
            }
            val pending = staging(file)
            try {
                pending.writeText(encode(lines))
                Files.move(pending.toPath(), file.toPath(), StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING)
            } finally {
                Files.deleteIfExists(pending.toPath())
            }
        }

        private fun staging(file: File) = File(file.parentFile, "monitor-connection.pending")

        private fun logFile(): File {
            val parent = directory()
            require(parent.isDirectory)
            return File(parent, "monitor-connection.log")
        }
    }

    private fun encode(lines: List<DiagnosticLine>): String = buildString {
        lines.forEach { appendLine(it.text) }
    }

    private fun <T> attempt(operation: () -> T): T? = try {
        operation()
    } catch (_: Exception) {
        null
    }
}

internal suspend fun MonitorDiagnostics.observeHeartbeat(
    scheduleDelayMs: Long,
    clock: () -> Long = SystemClock::elapsedRealtime,
    request: suspend () -> Heartbeat,
): Heartbeat {
    heartbeatStarted(scheduleDelayMs)
    val start = clock()
    val result = try {
        request()
    } catch (failure: Exception) {
        heartbeatFailed((clock() - start).coerceAtLeast(0), failure)
        throw failure
    }
    heartbeatSucceeded((clock() - start).coerceAtLeast(0), result)
    return result
}
