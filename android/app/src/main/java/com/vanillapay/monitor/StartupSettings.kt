package com.vanillapay.monitor

import android.content.Intent

internal sealed interface StartupTarget {
    data class Component(val name: String) : StartupTarget
    data class Action(val name: String) : StartupTarget
    data object AppDetails : StartupTarget
}

internal fun isVivoManufacturer(manufacturer: String): Boolean = when (manufacturer.lowercase()) {
    "vivo", "iqoo" -> true
    else -> false
}

internal fun startupTargets(manufacturer: String): List<StartupTarget> {
    val component = when (manufacturer.lowercase()) {
        "xiaomi", "redmi" -> "com.miui.securitycenter/com.miui.permcenter.autostart.AutoStartManagementActivity"
        "huawei", "honor" -> "com.huawei.systemmanager/com.huawei.systemmanager.startupmgr.ui.StartupNormalAppListActivity"
        "oppo", "realme", "oneplus" -> "com.coloros.safecenter/com.coloros.safecenter.startupapp.StartupAppListActivity"
        else -> if (isVivoManufacturer(manufacturer)) {
            "com.vivo.permissionmanager/com.vivo.permissionmanager.activity.BgStartUpManagerActivity"
        } else null
    }
    return listOfNotNull(component?.let { StartupTarget.Component(it) }, StartupTarget.AppDetails)
}

internal fun backgroundTargets(manufacturer: String): List<StartupTarget> =
    if (isVivoManufacturer(manufacturer)) listOf(
        StartupTarget.Component("com.iqoo.powersaving/com.iqoo.powersaving.BackgroundHighUsageActivity"),
        StartupTarget.Action(Intent.ACTION_POWER_USAGE_SUMMARY),
        StartupTarget.AppDetails,
    ) else emptyList()
