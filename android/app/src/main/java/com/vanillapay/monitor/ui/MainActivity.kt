package com.vanillapay.monitor.ui

import android.Manifest
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.content.res.ColorStateList
import android.os.Build
import android.os.Bundle
import android.view.View
import android.widget.ImageButton
import android.widget.ImageView
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import androidx.lifecycle.lifecycleScope
import com.vanillapay.monitor.Money
import com.vanillapay.monitor.R
import com.vanillapay.monitor.config.AppConfig
import com.vanillapay.monitor.data.AppDatabase
import com.vanillapay.monitor.net.HeartbeatReporter
import com.vanillapay.monitor.permission.PermissionManager
import com.vanillapay.monitor.service.KeepAliveService
import com.vanillapay.monitor.util.HeartbeatTime
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class MainActivity : AppCompatActivity() {
    private lateinit var config: AppConfig

    /** Fired by [HeartbeatReporter] on every successful heartbeat (manual or automatic). */
    private val heartbeatReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            updateHeartbeatLabel()
            updatePid()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        config = AppConfig(this)
        if (!config.isBound) {
            startActivity(Intent(this, BindActivity::class.java))
            finish()
            return
        }
        setContentView(R.layout.activity_main)
        applySystemBarInsets()
        ContextCompat.startForegroundService(this, Intent(this, KeepAliveService::class.java))
        maybeRequestNotificationPermission()

        updatePid()
        findViewById<ImageButton>(R.id.btnSettings).setOnClickListener {
            startActivity(Intent(this, SettingsActivity::class.java))
        }
        findViewById<ImageButton>(R.id.btnLog).setOnClickListener {
            startActivity(Intent(this, LogActivity::class.java))
        }
        findViewById<View>(R.id.btnTestHeartbeat).setOnClickListener { testHeartbeat() }
    }

    override fun onResume() {
        super.onResume()
        if (!config.isBound) {
            startActivity(Intent(this, BindActivity::class.java))
            finish()
            return
        }
        if (!PermissionManager.allGranted(this)) {
            startActivity(Intent(this, PermissionActivity::class.java))
            return
        }
        renderStatus()
        loadStats()
        updateHeartbeatLabel()
        updatePid()
        ContextCompat.registerReceiver(
            this,
            heartbeatReceiver,
            IntentFilter(HeartbeatReporter.ACTION_HEARTBEAT),
            ContextCompat.RECEIVER_NOT_EXPORTED,
        )
    }

    override fun onPause() {
        super.onPause()
        runCatching { unregisterReceiver(heartbeatReceiver) }
    }

    private fun updateHeartbeatLabel() {
        findViewById<TextView>(R.id.tvHeartbeat).text =
            HeartbeatTime.format(config.lastHeartbeatAt)
    }

    private fun updatePid() {
        findViewById<TextView>(R.id.tvPid).text = config.merchantPid.ifEmpty { "—" }
    }

    private fun testHeartbeat() {
        val button = findViewById<View>(R.id.btnTestHeartbeat)
        button.isEnabled = false
        lifecycleScope.launch {
            val ok = withContext(Dispatchers.IO) { HeartbeatReporter(applicationContext).send() }
            updateHeartbeatLabel()
            button.isEnabled = true
            Toast.makeText(
                this@MainActivity,
                if (ok) R.string.heartbeat_ok else R.string.heartbeat_fail,
                Toast.LENGTH_SHORT,
            ).show()
        }
    }

    private fun renderStatus() {
        val notifOn = PermissionManager.isNotificationListenerEnabled(this)
        val statusIcon = findViewById<ImageView>(R.id.statusIcon)
        val statusIconBg = findViewById<View>(R.id.statusIconBg)
        val tvStatus = findViewById<TextView>(R.id.tvStatus)
        if (notifOn) {
            statusIcon.setImageResource(R.drawable.ic_check_circle)
            statusIcon.imageTintList = colorState(R.color.brand_primary)
            statusIconBg.backgroundTintList = colorState(R.color.brand_container)
            tvStatus.setText(R.string.status_running)
        } else {
            statusIcon.setImageResource(R.drawable.ic_bell)
            statusIcon.imageTintList = colorState(R.color.warning)
            statusIconBg.backgroundTintList = colorState(R.color.warning_container)
            tvStatus.setText(R.string.status_notif_off)
        }
    }

    private fun loadStats() {
        lifecycleScope.launch {
            val dao = AppDatabase.get(this@MainActivity).pushDao()
            val count = dao.countSent()
            val sum = dao.sumSentTotal()
            findViewById<TextView>(R.id.tvTotalCount).text = count.toString()
            findViewById<TextView>(R.id.tvTotalAmount).text = "¥" + Money.format(sum)
        }
    }

    private fun maybeRequestNotificationPermission() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            ActivityCompat.requestPermissions(this, arrayOf(Manifest.permission.POST_NOTIFICATIONS), 100)
        }
    }

    private fun colorState(resId: Int) =
        ColorStateList.valueOf(ContextCompat.getColor(this, resId))
}
