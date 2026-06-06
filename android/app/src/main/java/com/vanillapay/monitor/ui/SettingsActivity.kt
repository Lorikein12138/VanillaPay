package com.vanillapay.monitor.ui

import android.app.ActivityManager
import android.content.Intent
import android.content.res.ColorStateList
import android.os.Bundle
import android.view.View
import android.widget.ImageButton
import android.widget.ImageView
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import com.vanillapay.monitor.BuildConfig
import com.vanillapay.monitor.R
import com.vanillapay.monitor.permission.PermissionManager
import com.vanillapay.monitor.service.KeepAliveService

class SettingsActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_settings)
        applySystemBarInsets()

        findViewById<ImageButton>(R.id.btnBack).setOnClickListener { finish() }
        findViewById<View>(R.id.btnSelfCheck).setOnClickListener {
            if (PermissionManager.allGranted(this)) {
                Toast.makeText(this, R.string.settings_selfcheck_ok_toast, Toast.LENGTH_SHORT).show()
            } else {
                startActivity(Intent(this, PermissionActivity::class.java))
            }
            renderSelfCheck()
        }
        findViewById<View>(R.id.btnRebind).setOnClickListener {
            startActivity(Intent(this, BindActivity::class.java))
        }
        findViewById<View>(R.id.btnHide).setOnClickListener {
            // Keep the foreground listening service (+ its persistent notification) alive,
            // then remove the app from recents entirely so it is fully hidden (GKD style).
            ContextCompat.startForegroundService(this, Intent(this, KeepAliveService::class.java))
            Toast.makeText(this, R.string.hide_toast, Toast.LENGTH_SHORT).show()
            hideFromRecents()
        }

        findViewById<TextView>(R.id.tvVersion).text =
            "${getString(R.string.app_name)} · ${getString(R.string.settings_version_fmt, BuildConfig.VERSION_NAME)}"
    }

    override fun onResume() {
        super.onResume()
        renderSelfCheck()
    }

    /** Remove the app's task(s) from recents; the foreground service keeps running in the background. */
    private fun hideFromRecents() {
        getSystemService(ActivityManager::class.java)?.appTasks?.forEach {
            runCatching { it.finishAndRemoveTask() }
        }
    }

    private fun renderSelfCheck() {
        val ok = PermissionManager.allGranted(this)
        val status = findViewById<TextView>(R.id.tvSelfCheckStatus)
        val icon = findViewById<ImageView>(R.id.ivSelfCheck)
        if (ok) {
            status.setText(R.string.settings_selfcheck_ok)
            status.setTextColor(ContextCompat.getColor(this, R.color.success))
            icon.setImageResource(R.drawable.ic_check_circle)
            icon.imageTintList = ColorStateList.valueOf(ContextCompat.getColor(this, R.color.success))
        } else {
            status.setText(R.string.settings_selfcheck_bad)
            status.setTextColor(ContextCompat.getColor(this, R.color.warning))
            icon.setImageResource(R.drawable.ic_bell)
            icon.imageTintList = ColorStateList.valueOf(ContextCompat.getColor(this, R.color.warning))
        }
    }
}
