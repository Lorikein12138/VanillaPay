package com.vanillapay.monitor.data

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Update

@Dao
interface PushDao {
    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insert(record: PushRecord): Long

    @Query("SELECT COUNT(*) FROM push_record WHERE rawHash = :hash")
    suspend fun countByHash(hash: String): Int

    @Query("SELECT * FROM push_record WHERE status != 'sent' AND nextRetryAt <= :now ORDER BY id ASC LIMIT 50")
    suspend fun due(now: Long): List<PushRecord>

    @Query("SELECT * FROM push_record ORDER BY id DESC LIMIT :limit")
    suspend fun recent(limit: Int): List<PushRecord>

    @Query("SELECT COUNT(*) FROM push_record WHERE createdAt >= :since")
    suspend fun countSince(since: Long): Int

    @Query("SELECT COALESCE(SUM(amountCents), 0) FROM push_record WHERE status = 'sent' AND createdAt >= :since")
    suspend fun sumSentSince(since: Long): Long

    @Update
    suspend fun update(record: PushRecord)
}
