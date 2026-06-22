package com.vanillapay.monitor

import com.vanillapay.monitor.parse.NotificationParser
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class NotificationParserTest {
    private val parser = NotificationParser(NotificationParser.defaultRules())

    @Test
    fun parsesWeChatIncome() {
        val result = parser.parse("com.tencent.mm", "微信支付", "你已成功收款10.00元")
        assertNotNull(result)
        assertEquals("wxpay", result!!.channel)
        assertEquals(1000L, result.amountCents)
    }

    @Test
    fun parsesAlipayIncome() {
        val result = parser.parse("com.eg.android.AlipayGphone", "支付宝", "成功收款88.50元")
        assertNotNull(result)
        assertEquals("alipay", result!!.channel)
        assertEquals(8850L, result.amountCents)
    }

    @Test
    fun ignoresNonPaymentNotification() {
        assertNull(parser.parse("com.tencent.mm", "微信", "你有一条新消息"))
    }

    @Test
    fun ignoresUnknownPackage() {
        assertNull(parser.parse("com.android.chrome", "x", "成功收款10.00元"))
    }
}
