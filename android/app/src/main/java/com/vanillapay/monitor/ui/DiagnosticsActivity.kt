package com.vanillapay.monitor.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Intent
import android.os.Bundle
import android.view.View
import android.widget.ImageButton
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.FileProvider
import androidx.lifecycle.lifecycleScope
import com.vanillapay.monitor.BuildConfig
import com.vanillapay.monitor.R
import com.vanillapay.monitor.util.AppLog
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File

/** Shows the persistent app log so a user can copy / export it for crash diagnosis. */
class DiagnosticsActivity : AppCompatActivity() {
    private lateinit var logView: TextView
    private lateinit var scroll: ScrollView
    private var current: String = ""

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_diagnostics)
        applySystemBarInsets()
        logView = findViewById(R.id.tvLog)
        scroll = findViewById(R.id.logScroll)
        findViewById<ImageButton>(R.id.btnBack).setOnClickListener { finish() }
        findViewById<View>(R.id.btnCopy).setOnClickListener { copy() }
        findViewById<View>(R.id.btnShare).setOnClickListener { share() }
        findViewById<View>(R.id.btnClear).setOnClickListener { clear() }
        findViewById<View>(R.id.btnRefresh).setOnClickListener { reload() }
    }

    override fun onResume() {
        super.onResume()
        reload()
    }

    private fun reload() {
        lifecycleScope.launch {
            val text = withContext(Dispatchers.IO) { AppLog.dump() }
            current = text
            logView.text = text.ifBlank { getString(R.string.diag_empty) }
            scroll.post { scroll.fullScroll(View.FOCUS_DOWN) }
        }
    }

    private fun copy() {
        if (current.isBlank()) {
            Toast.makeText(this, R.string.diag_empty, Toast.LENGTH_SHORT).show()
            return
        }
        getSystemService(ClipboardManager::class.java)
            .setPrimaryClip(ClipData.newPlainText("VanillaPay log", current))
        Toast.makeText(this, R.string.diag_copied, Toast.LENGTH_SHORT).show()
    }

    private fun share() {
        if (current.isBlank()) {
            Toast.makeText(this, R.string.diag_empty, Toast.LENGTH_SHORT).show()
            return
        }
        lifecycleScope.launch {
            val uri = withContext(Dispatchers.IO) {
                val dir = File(cacheDir, "shared").apply { mkdirs() }
                val out = File(dir, "vanillapay-log.txt").apply { writeText(current) }
                FileProvider.getUriForFile(
                    this@DiagnosticsActivity,
                    "${BuildConfig.APPLICATION_ID}.fileprovider",
                    out,
                )
            }
            val send = Intent(Intent.ACTION_SEND).apply {
                type = "text/plain"
                putExtra(Intent.EXTRA_SUBJECT, getString(R.string.diag_share_subject))
                putExtra(Intent.EXTRA_STREAM, uri)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
            startActivity(Intent.createChooser(send, getString(R.string.diag_share)))
        }
    }

    private fun clear() {
        AppLog.clear()
        AppLog.i("Diag", "log cleared by user")
        reload()
        Toast.makeText(this, R.string.diag_cleared, Toast.LENGTH_SHORT).show()
    }
}
