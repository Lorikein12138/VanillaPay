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
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/** Inflates and binds a single [item_recent_push] row for the report log (time / channel / amount). */
object PushRowBinder {
    private val timeFormat = SimpleDateFormat("MM-dd HH:mm:ss", Locale.getDefault())

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

        title.text = "¥" + Money.format(record.amountCents)
        val channelName = context.getString(
            if (isWechat) R.string.channel_wxpay else R.string.channel_alipay,
        )
        subtitle.text = context.getString(
            R.string.log_row_sub_fmt,
            channelName,
            timeFormat.format(Date(record.createdAt)),
        )
        state.visibility = View.GONE

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
