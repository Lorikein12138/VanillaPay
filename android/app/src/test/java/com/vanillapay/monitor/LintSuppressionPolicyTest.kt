package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

class LintSuppressionPolicyTest {
    @Test
    fun `battery optimization request warning is intentionally scoped`() {
        val manifest = File("src/main/AndroidManifest.xml").readText()
        val source = File("src/main/java/com/vanillapay/monitor/ui/PermissionActivity.kt").readText()

        assertTrue(manifest.contains("REQUEST_IGNORE_BATTERY_OPTIMIZATIONS"))
        assertTrue(source.contains("""@SuppressLint("BatteryLife")"""))
        assertTrue(source.contains("Forced gate"))
        assertTrue(source.contains("ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS"))
    }

    @Test
    fun `scan layout keeps explicit frame root with scoped merge warning suppression`() {
        val layout = File("src/main/res/layout/activity_scan.xml").readText()
        val activity = File("src/main/java/com/vanillapay/monitor/ui/ScanActivity.kt").readText()

        assertTrue(activity.contains("setContentView(R.layout.activity_scan)"))
        assertTrue(layout.contains("<FrameLayout"))
        assertTrue(layout.contains("""tools:ignore="MergeRootFrame""""))
    }
}
