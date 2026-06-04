package com.vanillapay.monitor.ui

import android.content.res.ColorStateList
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ImageView
import android.widget.TextView
import androidx.core.content.ContextCompat
import androidx.core.graphics.ColorUtils
import com.vanillapay.monitor.Money
import com.vanillapay.monitor.R
import com.vanillapay.monitor.data.PushRecord

/** Inflates and binds a single [item_recent_push] row. Shared by dashboard and log. */
object PushRowBinder {

    fun inflate(inflater: LayoutInflater, parent: ViewGroup, record: PushRecord): View {
        val view = inflater.inflate(R.layout.item_recent_push, parent, false)
        val context = parent.context

        val channelBg = view.findViewById<View>(R.id.ivChannelBg)
        val channelIcon = view.findViewById<ImageView>(R.id.ivChannel)
        val title = view.findViewById<TextView>(R.id.tvItemTitle)
        val subtitle = view.findViewById<TextView>(R.id.tvItemSub)
        val state = view.findViewById<TextView>(R.id.tvItemState)

        val isWechat = record.channel == "wxpay"
        channelIcon.setImageResource(if (isWechat) R.drawable.ic_chat else R.drawable.ic_wallet)
        val channelColor = ContextCompat.getColor(
            context,
            if (isWechat) R.color.channel_wechat else R.color.channel_alipay,
        )
        channelIcon.imageTintList = ColorStateList.valueOf(channelColor)
        channelBg.backgroundTintList =
            ColorStateList.valueOf(ColorUtils.setAlphaComponent(channelColor, 30))

        title.text = context.getString(R.string.log_row_title_fmt, record.id, Money.format(record.amountCents))
        val channelName = context.getString(
            if (isWechat) R.string.channel_wxpay else R.string.channel_alipay,
        )
        subtitle.text = context.getString(R.string.log_row_sub_fmt, channelName, record.attempts)

        val (labelRes, fgRes, bgRes) = when (record.status) {
            "sent" -> Triple(R.string.status_sent, R.color.success, R.color.success_container)
            "failed" -> Triple(R.string.status_failed, R.color.danger, R.color.danger_container)
            else -> Triple(R.string.status_pending, R.color.warning, R.color.warning_container)
        }
        state.setText(labelRes)
        state.setTextColor(ContextCompat.getColor(context, fgRes))
        state.backgroundTintList = ColorStateList.valueOf(ContextCompat.getColor(context, bgRes))

        return view
    }

    /** Adds a thin divider matching the design tokens. */
    fun divider(context: android.content.Context): View {
        val divider = View(context)
        divider.layoutParams = ViewGroup.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT,
            context.resources.getDimensionPixelSize(R.dimen.card_stroke),
        )
        divider.setBackgroundColor(ContextCompat.getColor(context, R.color.border))
        return divider
    }
}
