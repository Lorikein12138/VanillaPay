package com.vanillapay.monitor.data

import androidx.room.Entity
import androidx.room.Index
import androidx.room.PrimaryKey

@Entity(
    tableName = "push_record",
    indices = [
        Index(value = ["rawHash"]),
        Index(value = ["status", "nextRetryAt"]),
        Index(value = ["createdAt"]),
    ],
)
data class PushRecord(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    val rawHash: String,
    val channel: String,
    val amountCents: Long,
    val t: Long,
    val raw: String?,
    val status: String = "pending",
    val attempts: Int = 0,
    val nextRetryAt: Long = 0,
    val createdAt: Long = System.currentTimeMillis(),
)
