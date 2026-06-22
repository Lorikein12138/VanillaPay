package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

class PushRecordRetentionTest {
    @Test
    fun `push records are indexed and pruned after drain`() {
        val entity = File("src/main/java/com/vanillapay/monitor/data/PushRecord.kt").readText()
        val dao = File("src/main/java/com/vanillapay/monitor/data/PushDao.kt").readText()
        val reporter = File("src/main/java/com/vanillapay/monitor/service/Reporter.kt").readText()

        assertTrue(entity.contains("Index(value = [\"rawHash\"])"))
        assertTrue(entity.contains("Index(value = [\"status\", \"nextRetryAt\"])"))
        assertTrue(dao.contains("deleteSentOlderThan"))
        assertTrue(dao.contains("deleteExhaustedOlderThan"))
        assertTrue(reporter.contains("cleanup(nowMillis)"))
    }

    @Test
    fun `database migration creates push record indexes`() {
        val database = File("src/main/java/com/vanillapay/monitor/data/AppDatabase.kt").readText()

        assertTrue(database.contains("version = 2"))
        assertTrue(database.contains("Migration(1, 2)"))
        assertTrue(database.contains("CREATE INDEX IF NOT EXISTS index_push_record_rawHash"))
        assertTrue(database.contains(".addMigrations(MIGRATION_1_2)"))
    }
}
