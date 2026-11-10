package com.vanillapay.monitor

import android.content.Intent
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class StartupSettingsTest {
    @Test
    fun vivoAndIqooUseStartupAndBackgroundTargetsInOrderRegardlessOfCase() {
        for (manufacturer in listOf("vivo", "iqoo", "ViVo", "iQOO")) {
            assertEquals(
                listOf(
                    StartupTarget.Component("com.vivo.permissionmanager/com.vivo.permissionmanager.activity.BgStartUpManagerActivity"),
                    StartupTarget.AppDetails,
                ),
                startupTargets(manufacturer),
            )
            assertEquals(
                listOf(
                    StartupTarget.Component("com.iqoo.powersaving/com.iqoo.powersaving.BackgroundHighUsageActivity"),
                    StartupTarget.Action(Intent.ACTION_POWER_USAGE_SUMMARY),
                    StartupTarget.AppDetails,
                ),
                backgroundTargets(manufacturer),
            )
        }
    }

    @Test
    fun otherManufacturersHaveNoBackgroundUsageTargets() {
        for (manufacturer in listOf("Xiaomi", "Huawei", "OPPO", "Google", "")) {
            assertTrue(backgroundTargets(manufacturer).isEmpty())
        }
        assertEquals(listOf(StartupTarget.AppDetails), startupTargets("Google"))
    }
}
