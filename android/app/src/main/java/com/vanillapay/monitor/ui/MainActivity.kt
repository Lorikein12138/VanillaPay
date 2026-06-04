package com.vanillapay.monitor.ui

import android.content.Intent
import android.content.res.ColorStateList
import android.net.Uri
import android.os.Bundle
import android.os.PowerManager
import android.provider.Settings
import android.view.View
import android.widget.ImageButton
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.lifecycle.lifecycleScope
import com.vanillapay.monitor.Money
import com.vanillapay.monitor.R
import com.vanillapay.monitor.config.AppConfig
import com.vanillapay.monitor.data.AppDatabase
import com.vanillapay.monitor.data.PushRecord
import com.vanillapay.monitor.service.KeepAliveService
import kotlinx.coroutines.launch
import java.util.Calendar

class MainActivity : AppCompatActivity() {
    private lateinit var config: AppConfig

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        config = AppConfig(this)
        if (!config.isBound) {
            startActivity(Intent(this, BindActivity::class.java))
            finish()
            return
        }
        setContentView(R.layout.activity_main)
        ContextCompat.startForegroundService(this, Intent(this, KeepAliveService::class.java))

        findViewById<ImageButton>(R.id.btnSettings).setOnClickListener {
            startActivity(Intent(this, SettingsActivity::class.java))
        }
        findViewById<LinearLayout>(R.id.recentHeader).setOnClickListener {
            startActivity(Intent(this, LogActivity::class.java))
        }
        findViewById<LinearLayout>(R.id.healthNotif).setOnClickListener {
            startActivity(Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS))
        }
        findViewById<LinearLayout>(R.id.healthBattery).setOnClickListener {
            startActivity(
                Intent(
                    Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS,
                    Uri.parse("package:$packageName"),
                ),
            )
        }

        findViewById<TextView>(R.id.tvDeviceBadge).text =
            getString(R.string.status_device_fmt, config.deviceId)
    }

    override fun onResume() {
        super.onResume()
        renderStatus()
        loadStats()
    }

    private fun renderStatus() {
        val notifOn = isNotificationListenerEnabled()
        val statusIcon = findViewById<ImageView>(R.id.statusIcon)
        val statusIconBg = findViewById<View>(R.id.statusIconBg)
        val tvStatus = findViewById<TextView>(R.id.tvStatus)
        val tvStatusDesc = findViewById<TextView>(R.id.tvStatusDesc)
        if (notifOn) {
            statusIcon.setImageResource(R.drawable.ic_check_circle)
            statusIcon.imageTintList = colorState(R.color.brand_primary)
            statusIconBg.backgroundTintList = colorState(R.color.brand_container)
            tvStatus.setText(R.string.status_running)
            tvStatusDesc.setText(R.string.status_running_desc)
        } else {
            statusIcon.setImageResource(R.drawable.ic_bell)
            statusIcon.imageTintList = colorState(R.color.warning)
            statusIconBg.backgroundTintList = colorState(R.color.warning_container)
            tvStatus.setText(R.string.status_notif_off)
            tvStatusDesc.setText(R.string.status_notif_off_desc)
        }
        bindStateBadge(findViewById(R.id.tvNotifState), notifOn)
        bindStateBadge(findViewById(R.id.tvBatteryState), isIgnoringBatteryOptimizations())
    }

    private fun bindStateBadge(badge: TextView, on: Boolean) {
        if (on) {
            badge.setText(R.string.perm_on)
            badge.setTextColor(ContextCompat.getColor(this, R.color.success))
            badge.backgroundTintList = colorState(R.color.success_container)
        } else {
            badge.setText(R.string.perm_off)
            badge.setTextColor(ContextCompat.getColor(this, R.color.warning))
            badge.backgroundTintList = colorState(R.color.warning_container)
        }
    }

    private fun loadStats() {
        val startOfDay = startOfToday()
        lifecycleScope.launch {
            val dao = AppDatabase.get(this@MainActivity).pushDao()
            val count = dao.countSince(startOfDay)
            val sum = dao.sumSentSince(startOfDay)
            val recent = dao.recent(5)
            findViewById<TextView>(R.id.tvTodayCount).text = count.toString()
            findViewById<TextView>(R.id.tvTodayAmount).text = "¥" + Money.format(sum)
            renderRecent(recent)
        }
    }

    private fun renderRecent(rows: List<PushRecord>) {
        val container = findViewById<LinearLayout>(R.id.recentContainer)
        val empty = findViewById<TextView>(R.id.tvRecentEmpty)
        container.removeAllViews()
        if (rows.isEmpty()) {
            empty.visibility = View.VISIBLE
            container.visibility = View.GONE
            return
        }
        empty.visibility = View.GONE
        container.visibility = View.VISIBLE
        rows.forEachIndexed { index, record ->
            if (index > 0) container.addView(PushRowBinder.divider(this))
            container.addView(PushRowBinder.inflate(layoutInflater, container, record))
        }
    }

    private fun isNotificationListenerEnabled(): Boolean {
        val flat = Settings.Secure.getString(contentResolver, "enabled_notification_listeners")
        return !flat.isNullOrEmpty() && flat.contains(packageName)
    }

    private fun isIgnoringBatteryOptimizations(): Boolean {
        val pm = getSystemService(PowerManager::class.java) ?: return false
        return pm.isIgnoringBatteryOptimizations(packageName)
    }

    private fun colorState(resId: Int) =
        ColorStateList.valueOf(ContextCompat.getColor(this, resId))

    private fun startOfToday(): Long {
        val c = Calendar.getInstance()
        c.set(Calendar.HOUR_OF_DAY, 0)
        c.set(Calendar.MINUTE, 0)
        c.set(Calendar.SECOND, 0)
        c.set(Calendar.MILLISECOND, 0)
        return c.timeInMillis
    }
}
