package com.vanillapay.monitor.ui

import android.content.Intent
import android.os.Bundle
import android.widget.Button
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import com.vanillapay.monitor.R
import com.vanillapay.monitor.config.AppConfig
import com.vanillapay.monitor.service.KeepAliveService

class MainActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val config = AppConfig(this)
        if (!config.isBound) {
            startActivity(Intent(this, BindActivity::class.java))
            finish()
            return
        }
        setContentView(R.layout.activity_main)
        ContextCompat.startForegroundService(this, Intent(this, KeepAliveService::class.java))
        findViewById<TextView>(R.id.tvStatus).text = "已绑定：设备 #${config.deviceId}，监听运行中"
        findViewById<Button>(R.id.btnLog).setOnClickListener {
            startActivity(Intent(this, LogActivity::class.java))
        }
        findViewById<Button>(R.id.btnSettings).setOnClickListener {
            startActivity(Intent(this, SettingsActivity::class.java))
        }
    }
}
