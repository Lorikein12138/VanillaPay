package com.vanillapay.monitor

import androidx.room.Dao
import androidx.room.Database
import androidx.room.Entity
import androidx.room.Index
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.RoomDatabase
import androidx.room.withTransaction
import com.vanillapay.monitor.parse.NotificationIdentity
import com.vanillapay.monitor.parse.ParsedReceipt
import com.vanillapay.monitor.parse.PaymentNotice
import com.vanillapay.monitor.parse.rawText
import kotlinx.coroutines.flow.Flow
import org.json.JSONObject
import java.time.Instant
import java.util.UUID

const val QUEUED = "queued"
const val MATCHED = "matched"
const val UNMATCHED = "unmatched"
const val FAILED = "failed"
internal const val WEEK = 7 * 24 * 60 * 60 * 1000L

@Entity(indices = [Index("scope", "status", "retryAt"), Index("scope", "channel", "providerTradeNo", unique = true)])
data class Receipt(
    @PrimaryKey val eventId: String,
    val scope: String,
    val channel: String,
    val cents: Long,
    val occurredAt: Long,
    val payload: String,
    val providerTradeNo: String? = null,
    val status: String = QUEUED,
    val retryAt: Long = 0,
    val attempts: Int = 0,
    val completedAt: Long? = null,
    val detail: String? = null,
)

@Entity(primaryKeys = ["scope", "sourceKey"])
data class NotificationStamp(
    val scope: String, val sourceKey: String, val postTime: Long, val notificationTime: Long,
    val fingerprint: String, val groupCount: Int?, val seenAt: Long,
) {
    fun identity() = NotificationIdentity(postTime, notificationTime, fingerprint, groupCount)
}

@Entity(primaryKeys = ["sourceKey", "postTime"])
data class Diagnostic(
    val sourceKey: String, val postTime: Long, val channel: String, val raw: String, val cents: Long?,
)

@Dao
interface MonitorDao {
    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insertReceipt(receipt: Receipt): Long

    @Query("SELECT * FROM Receipt WHERE scope = :scope ORDER BY occurredAt DESC, eventId DESC LIMIT 50")
    fun recent(scope: String): Flow<List<Receipt>>

    @Query("SELECT COUNT(*) FROM Receipt WHERE scope = :scope AND status = 'queued'")
    fun pending(scope: String): Flow<Int>

    @Query("SELECT * FROM Receipt WHERE scope = :scope AND status = 'queued' AND retryAt <= :now ORDER BY retryAt, occurredAt LIMIT 1")
    suspend fun next(scope: String, now: Long): Receipt?

    @Query("SELECT MIN(retryAt) FROM Receipt WHERE scope = :scope AND status = 'queued'")
    suspend fun nextRetry(scope: String): Long?

    @Query("SELECT COUNT(*) FROM Receipt WHERE scope = :scope AND status IN ('matched','unmatched') AND completedAt >= :start")
    fun reported(scope: String, start: Long): Flow<Int>

    @Query("UPDATE Receipt SET attempts = :attempts, retryAt = :next WHERE eventId = :id AND status = 'queued'")
    suspend fun retry(id: String, attempts: Int, next: Long)

    @Query("UPDATE Receipt SET status = :status, detail = :detail, completedAt = :now WHERE eventId = :id AND status = 'queued'")
    suspend fun complete(id: String, status: String, detail: String?, now: Long)

    @Query("SELECT * FROM NotificationStamp WHERE scope = :scope AND sourceKey = :key")
    suspend fun stamp(scope: String, key: String): NotificationStamp?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun setStamp(stamp: NotificationStamp)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun saveDiagnostic(diagnostic: Diagnostic)

    @Query("SELECT * FROM Diagnostic ORDER BY postTime DESC, sourceKey DESC LIMIT 50")
    fun diagnostics(): Flow<List<Diagnostic>>

    @Query("DELETE FROM Diagnostic WHERE postTime < :cutoff OR rowid NOT IN (SELECT rowid FROM Diagnostic ORDER BY postTime DESC, sourceKey DESC LIMIT 50)")
    suspend fun pruneDiagnostics(cutoff: Long)

    @Query("DELETE FROM NotificationStamp WHERE seenAt < :cutoff OR scope != :scope")
    suspend fun pruneStamps(cutoff: Long, scope: String)

    @Query("DELETE FROM Receipt WHERE scope != :scope OR completedAt < :cutoff")
    suspend fun pruneReceipts(cutoff: Long, scope: String)
}

@Database(entities = [Receipt::class, NotificationStamp::class, Diagnostic::class], version = 1, exportSchema = false)
abstract class MonitorDatabase : RoomDatabase() {
    abstract fun dao(): MonitorDao

    suspend fun capture(notice: PaymentNotice, parsed: ParsedReceipt?, diagnosticChannel: String?, session: Credential?, now: Long): Boolean =
        withTransaction {
            val dao = dao()
            if (notice.postTime < now - WEEK) return@withTransaction false
            val raw = notice.rawText()
            if (diagnosticChannel != null) {
                dao.saveDiagnostic(Diagnostic(notice.key, notice.postTime, diagnosticChannel, raw, parsed?.cents))
                dao.pruneDiagnostics(now - WEEK)
            }
            if (parsed == null || session?.started != true) return@withTransaction false
            val source = "${notice.packageName}:${notice.key}"
            val stamp = NotificationStamp(session.deviceId, source, notice.postTime, notice.notificationTime,
                parsed.fingerprint, parsed.groupCount, now)
            val previous = dao.stamp(session.deviceId, source)
            if (previous != null && (notice.postTime < previous.postTime || stamp.identity().repeats(previous.identity()))) {
                return@withTransaction false
            }
            val event = newReceipt(notice, parsed, raw, session)
            dao.setStamp(stamp)
            dao.insertReceipt(event) != -1L
        }
}

internal fun newReceipt(notice: PaymentNotice, parsed: ParsedReceipt, raw: String, session: Credential): Receipt {
    val occurred = notice.postTime + session.offsetMillis
    val id = UUID.randomUUID().toString()
    val payload = JSONObject().put("event_id", id).put("channel", parsed.channel.wire)
        .put("amount_cents", parsed.cents).put("occurred_at", Instant.ofEpochMilli(occurred).toString())
        .put("raw_text", raw).apply { if (parsed.tradeNo != null) put("provider_trade_no", parsed.tradeNo) }.toString()
    return Receipt(id, session.deviceId, parsed.channel.wire, parsed.cents, occurred, payload, parsed.tradeNo)
}

internal fun retryDelay(attempts: Int) = (1_000L shl attempts.coerceIn(0, 8)).coerceAtMost(300_000L)
