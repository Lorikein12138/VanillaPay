package com.vanillapay.monitor.ui

import android.view.View
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat

/**
 * Pads the view down by the system-bar insets so content no longer draws under the
 * status / navigation bars. targetSdk 35 is edge-to-edge by default, so each screen
 * must consume insets itself. Original padding is preserved and added to.
 */
fun applySystemBarInsets(view: View, top: Boolean = true, bottom: Boolean = true) {
    val left = view.paddingLeft
    val baseTop = view.paddingTop
    val right = view.paddingRight
    val baseBottom = view.paddingBottom
    ViewCompat.setOnApplyWindowInsetsListener(view) { v, insets ->
        val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars())
        v.setPadding(
            left,
            baseTop + if (top) bars.top else 0,
            right,
            baseBottom + if (bottom) bars.bottom else 0,
        )
        insets
    }
}

/** Applies system-bar insets to the activity's content root. */
fun AppCompatActivity.applySystemBarInsets(top: Boolean = true, bottom: Boolean = true) {
    applySystemBarInsets(findViewById(android.R.id.content), top, bottom)
}
