package com.vanillapay.monitor.ui

import android.os.Bundle
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import com.vanillapay.monitor.R
import com.vanillapay.monitor.data.AppDatabase
import kotlinx.coroutines.launch

class LogActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_log)
        val logText = findViewById<TextView>(R.id.tvLog)
        lifecycleScope.launch {
            val rows = AppDatabase.get(this@LogActivity).pushDao().due(Long.MAX_VALUE)
            logText.text = if (rows.isEmpty()) {
                "暂无待上报记录"
            } else {
                rows.joinToString("\n") { "#${it.id} ${it.channel} ${it.amountCents}分 ${it.status} 次数${it.attempts}" }
            }
        }
    }
}
