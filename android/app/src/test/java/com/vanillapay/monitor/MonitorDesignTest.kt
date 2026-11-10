package com.vanillapay.monitor

import androidx.compose.ui.graphics.Color
import com.vanillapay.monitor.parse.Channel
import org.junit.Assert.assertEquals
import org.junit.Test

class MonitorDesignTest {
    @Test
    fun eachChannelHasItsOwnDrawableAndBrandColors() {
        val artwork = Channel.entries.associateWith(::channelArtwork)
        assertEquals(R.drawable.channel_alipay, artwork.getValue(Channel.Alipay).drawable)
        assertEquals(R.drawable.channel_wechat, artwork.getValue(Channel.WeChat).drawable)
        assertEquals(R.drawable.channel_qq, artwork.getValue(Channel.QQ).drawable)
        assertEquals(artwork.size, artwork.values.map { it.drawable }.toSet().size)

        val light = artwork.mapValues { (_, icon) -> icon.color(darkTheme = false) }
        val dark = artwork.mapValues { (_, icon) -> icon.color(darkTheme = true) }
        assertEquals(mapOf(
            Channel.Alipay to Color(0xff1677ff),
            Channel.WeChat to Color(0xff07a855),
            Channel.QQ to Color(0xff008cdb),
        ), light)
        assertEquals(mapOf(
            Channel.Alipay to Color(0xff6ba6ff),
            Channel.WeChat to Color(0xff47d68a),
            Channel.QQ to Color(0xff58bfff),
        ), dark)
        assertEquals(artwork.size, light.values.toSet().size)
        assertEquals(artwork.size, dark.values.toSet().size)
    }

    @Test
    fun pendingUploadsUseAccentOnlyForPositiveCount() {
        assertEquals(Color.Black, pendingUploadColor(0, accent = Color.Red, normal = Color.Black))
        assertEquals(Color.Red, pendingUploadColor(1, accent = Color.Red, normal = Color.Black))
    }
}
