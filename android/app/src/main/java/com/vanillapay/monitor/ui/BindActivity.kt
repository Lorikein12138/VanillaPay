package com.vanillapay.monitor.ui

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.widget.Button
import android.widget.EditText
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.app.ActivityCompat
import com.vanillapay.monitor.R
import com.vanillapay.monitor.bind.BindingPayload
import com.vanillapay.monitor.config.AppConfig

class BindActivity : AppCompatActivity() {
    private val scanLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) { result ->
        if (result.resultCode == Activity.RESULT_OK) {
            val payload = result.data?.getStringExtra(ScanActivity.EXTRA_PAYLOAD).orEmpty()
            findViewById<EditText>(R.id.etPayload).setText(payload)
            bind(payload)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_bind)
        applySystemBarInsets()
        maybeRequestNotificationPermission()
        val payloadInput = findViewById<EditText>(R.id.etPayload)
        findViewById<Button>(R.id.btnBind).setOnClickListener {
            bind(payloadInput.text.toString())
        }
        findViewById<Button>(R.id.btnScan).setOnClickListener {
            scanLauncher.launch(Intent(this, ScanActivity::class.java))
        }
        findViewById<Button>(R.id.btnNotifAccess).setOnClickListener {
            startActivity(Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS))
        }
    }

    private fun bind(text: String) {
        val binding = BindingPayload.parse(text)
        if (binding == null) {
            Toast.makeText(this, "绑定串格式错误", Toast.LENGTH_SHORT).show()
            return
        }
        val config = AppConfig(this)
        config.serverUrl = binding.serverUrl
        config.deviceId = binding.deviceId
        config.deviceKey = binding.deviceKey
        Toast.makeText(this, "绑定成功", Toast.LENGTH_SHORT).show()
        startActivity(Intent(this, MainActivity::class.java))
        finish()
    }

    private fun maybeRequestNotificationPermission() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            ActivityCompat.requestPermissions(this, arrayOf(Manifest.permission.POST_NOTIFICATIONS), 100)
        }
    }
}
