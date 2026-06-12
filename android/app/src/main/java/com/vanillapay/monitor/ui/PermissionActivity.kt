package com.vanillapay.monitor.ui

import android.content.Intent
import android.content.res.ColorStateList
import android.os.Bundle
import android.provider.Settings
import android.view.View
import android.widget.TextView
import androidx.activity.addCallback
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.core.net.toUri
import com.google.android.material.button.MaterialButton
import com.google.android.material.checkbox.MaterialCheckBox
import com.vanillapay.monitor.R
import com.vanillapay.monitor.config.AppConfig
import com.vanillapay.monitor.permission.AutostartIntents
import com.vanillapay.monitor.permission.PermissionManager

/** Forced gate: the user must grant all three permissions before reaching the dashboard. */
class PermissionActivity : AppCompatActivity() {
    private lateinit var config: AppConfig

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        config = AppConfig(this)
        setContentView(R.layout.activity_permission)
        applySystemBarInsets()
        onBackPressedDispatcher.addCallback(this) {
            moveTaskToBack(true)
        }

        findViewById<View>(R.id.rowNotif).setOnClickListener {
            runCatching { startActivity(Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS)) }
        }
        findViewById<View>(R.id.rowBattery).setOnClickListener {
            runCatching {
                startActivity(
                    Intent(
                        Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS,
                        "package:$packageName".toUri(),
                    ),
                )
            }
        }
        findViewById<View>(R.id.rowAutostart).setOnClickListener {
            AutostartIntents.open(this)
        }
        findViewById<MaterialCheckBox>(R.id.cbAutostart).setOnCheckedChangeListener { _, checked ->
            config.autostartConfirmed = checked
            render()
        }
        findViewById<MaterialButton>(R.id.btnEnter).setOnClickListener {
            if (PermissionManager.allGranted(this)) finish()
        }
    }

    override fun onResume() {
        super.onResume()
        render()
    }

    private fun render() {
        val notif = PermissionManager.isNotificationListenerEnabled(this)
        val battery = PermissionManager.isIgnoringBatteryOptimizations(this)
        val autostart = config.autostartConfirmed
        bindBadge(findViewById(R.id.tvNotifState), notif)
        bindBadge(findViewById(R.id.tvBatteryState), battery)
        bindBadge(findViewById(R.id.tvAutostartState), autostart)
        findViewById<MaterialCheckBox>(R.id.cbAutostart).isChecked = autostart
        findViewById<MaterialButton>(R.id.btnEnter).isEnabled = notif && battery && autostart
    }

    private fun bindBadge(badge: TextView, on: Boolean) {
        if (on) {
            badge.setText(R.string.perm_on)
            badge.setTextColor(ContextCompat.getColor(this, R.color.success))
            badge.backgroundTintList = ColorStateList.valueOf(ContextCompat.getColor(this, R.color.success_container))
        } else {
            badge.setText(R.string.perm_off)
            badge.setTextColor(ContextCompat.getColor(this, R.color.warning))
            badge.backgroundTintList = ColorStateList.valueOf(ContextCompat.getColor(this, R.color.warning_container))
        }
    }
}
