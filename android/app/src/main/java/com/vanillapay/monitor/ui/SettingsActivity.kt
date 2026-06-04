package com.vanillapay.monitor.ui

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.provider.Settings
import android.view.View
import android.widget.ImageButton
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import com.vanillapay.monitor.BuildConfig
import com.vanillapay.monitor.R
import com.vanillapay.monitor.config.AppConfig

class SettingsActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_settings)

        findViewById<ImageButton>(R.id.btnBack).setOnClickListener { finish() }
        findViewById<View>(R.id.btnBattery).setOnClickListener {
            startActivity(
                Intent(
                    Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS,
                    Uri.parse("package:$packageName"),
                ),
            )
        }
        findViewById<View>(R.id.btnNotifAccess).setOnClickListener {
            startActivity(Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS))
        }
        findViewById<View>(R.id.btnRebind).setOnClickListener {
            startActivity(Intent(this, BindActivity::class.java))
        }

        findViewById<TextView>(R.id.tvVersion).text = getString(
            R.string.settings_version_fmt,
            BuildConfig.VERSION_NAME,
        ).let { "${getString(R.string.app_name)} · $it" }
        findViewById<TextView>(R.id.tvDeviceId).text =
            getString(R.string.status_device_fmt, AppConfig(this).deviceId)
    }
}
