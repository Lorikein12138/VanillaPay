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

    @Update
    suspend fun update(record: PushRecord)
}
