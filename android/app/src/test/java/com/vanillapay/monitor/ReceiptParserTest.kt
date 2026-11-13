package com.vanillapay.monitor

import com.vanillapay.monitor.parse.Channel
import com.vanillapay.monitor.parse.PaymentNotice
import com.vanillapay.monitor.parse.isDiagnostic
import com.vanillapay.monitor.parse.parseReceipt
import com.vanillapay.monitor.parse.rawText
import org.junit.Assert.*
import org.junit.Test

class ReceiptParserTest {
    @Test fun incomeFormatsAreChannelSpecificAndExactInCents() {
        val examples = listOf(
            Example(Channel.Alipay, "支付宝", "你已成功收款1.00元", 100),
            Example(Channel.Alipay, "支付宝", "支付宝成功收款1.00元", 100),
            Example(Channel.Alipay, "支付宝收款", "个人收款码到账¥0.01", 1),
            Example(Channel.WeChat, "微信支付", "微信支付收款0.01元(朋友到店)", 1),
            Example(Channel.WeChat, "微信", "[2条]微信支付: 收款0.02元", 2),
            Example(Channel.WeChat, "[3条]微信支付", "个人收款码到账¥0.03", 3),
        )
        for ((channel, title, body, cents) in examples) {
            val parsed = parseReceipt(notice(channel, title, body))
            assertNotNull(body, parsed)
            assertEquals(channel, parsed!!.channel)
            assertEquals(body, cents, parsed.cents)
        }
        val withID = parseReceipt(notice(Channel.Alipay, "支付宝", "支付宝成功收款3.30元\n交易号：ABC_123-45"))
        assertEquals("ABC_123-45", withID?.tradeNo)
    }

    @Test fun rejectsPaymentsRefundsAndChatForEveryProvider() {
        val titles = listOf("支付宝", "微信支付")
        for ((channel, title) in Channel.entries.zip(titles)) {
            for (body in listOf("付款1.00元", "退款到账¥1.00", "转出1.00元", "今天午餐花了1.00元", "请收款1.00元")) {
                val notice = notice(channel, title, body)
                assertNull(body, parseReceipt(notice))
                assertTrue(body, isDiagnostic(notice))
            }
            assertNull(parseReceipt(notice(channel, "聊天群", "你已成功收款1.00元")))
        }
        assertNull(parseReceipt(notice(Channel.Alipay, "支付宝", "支付宝成功收款0.00元")))
        assertNull(parseReceipt(notice(Channel.WeChat, "微信支付", "微信支付收款1.001元")))
    }

    @Test fun rawTextTruncatesWithoutSplittingUnicode() {
        val notice = notice(Channel.Alipay, "支付宝", "🌼".repeat(5000))
        val raw = notice.rawText()
        assertTrue(raw.startsWith("支付宝\n"))
        assertEquals(4096, raw.codePointCount(0, raw.length))
        assertTrue(raw.endsWith("🌼"))
    }

    private data class Example(val channel: Channel, val title: String, val body: String, val cents: Long)
    private fun notice(channel: Channel, title: String, body: String) = PaymentNotice("provider", channel.packageName, 1, 1, title, body)
}
