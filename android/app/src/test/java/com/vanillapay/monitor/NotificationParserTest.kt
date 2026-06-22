package com.vanillapay.monitor

import com.vanillapay.monitor.parse.NotificationParser
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class NotificationParserTest {
    private val parser = NotificationParser(NotificationParser.defaultRules())

    private fun wx(text: String, title: String = "微信支付") =
        parser.parse("com.tencent.mm", title, text)

    private fun ali(text: String, title: String = "支付宝通知") =
        parser.parse("com.eg.android.AlipayGphone", title, text)

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

    // ---- hardened variants ----

    @Test
    fun parsesWeChatParentheticalSuffix() {
        assertEquals(1L, wx("微信支付收款0.01元(朋友到店)")?.amountCents)
    }

    @Test
    fun parsesWeChatGroupedSummaryLine() {
        assertEquals(101L, wx("[4条]微信支付: 微信支付收款1.01元(朋友到店)")?.amountCents)
    }

    @Test
    fun parsesWeChatDaozhangWording() {
        // Merchant accounts show 到账 instead of 收款.
        assertEquals(1234L, wx("微信支付到账12.34元")?.amountCents)
    }

    @Test
    fun parsesAlipayTrailingPeriod() {
        assertEquals(100L, ali("支付宝成功收款1.00元。")?.amountCents)
    }

    @Test
    fun parsesAmountWithSpaces() {
        assertEquals(50L, wx("收款 0.50 元")?.amountCents)
    }

    @Test
    fun parsesThousandsSeparator() {
        assertEquals(123456L, wx("收款1,234.56元")?.amountCents)
    }

    @Test
    fun parsesCurrencySymbolBeforeAmount() {
        assertEquals(80L, wx("收款¥0.80")?.amountCents)
    }

    @Test
    fun parsesFullWidthYuanSymbol() {
        assertEquals(200L, wx("收款￥2.00")?.amountCents)
    }

    @Test
    fun parsesIntegerAmountWithoutDecimals() {
        assertEquals(500L, wx("微信支付收款5元")?.amountCents)
    }

    @Test
    fun ignoresOutgoingPayment() {
        // 付款 = money leaving the wallet; must never be counted as income.
        assertNull(wx("你已向张三付款5.00元"))
    }

    @Test
    fun ignoresZeroAmount() {
        assertNull(wx("收款0.00元"))
    }

    @Test
    fun ignoresKeywordFarFromAmount() {
        // "收款" unrelated to a number elsewhere must not produce a false match.
        assertNull(wx("收款功能已开通，本月剩余额度充足"))
    }
}
