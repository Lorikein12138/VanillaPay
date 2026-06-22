package com.vanillapay.monitor.service

import android.content.Context
import android.content.Intent
import android.widget.Toast
import androidx.core.content.ContextCompat
import com.vanillapay.monitor.R

object ForegroundServiceStarter {
    fun startKeepAlive(context: Context, showToast: Boolean = true): Boolean =
        runCatching {
            ContextCompat.startForegroundService(
                context,
                Intent(context, KeepAliveService::class.java),
            )
        }.fold(
            onSuccess = { true },
            onFailure = {
                if (showToast) {
                    Toast.makeText(
                        context.applicationContext,
                        context.getString(R.string.keepalive_start_failed, it.message.orEmpty()),
                        Toast.LENGTH_LONG,
                    ).show()
                }
                false
            },
        )
}
