package com.vanillapay.monitor.ui

import android.os.Bundle
import android.view.View
import android.widget.ImageButton
import android.widget.LinearLayout
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
        findViewById<ImageButton>(R.id.btnBack).setOnClickListener { finish() }

        val container = findViewById<LinearLayout>(R.id.logContainer)
        val empty = findViewById<TextView>(R.id.tvLogEmpty)
        lifecycleScope.launch {
            val rows = AppDatabase.get(this@LogActivity).pushDao().recent(200)
            if (rows.isEmpty()) {
                empty.visibility = View.VISIBLE
                return@launch
            }
            empty.visibility = View.GONE
            rows.forEachIndexed { index, record ->
                if (index > 0) container.addView(PushRowBinder.divider(this@LogActivity))
                container.addView(PushRowBinder.inflate(layoutInflater, container, record))
            }
        }
    }
}
