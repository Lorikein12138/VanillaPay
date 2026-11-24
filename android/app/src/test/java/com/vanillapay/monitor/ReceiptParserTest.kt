package com.vanillapay.monitor

import com.vanillapay.monitor.parse.Channel
import com.vanillapay.monitor.parse.PaymentNotice
import com.vanillapay.monitor.parse.isDiagnostic
import com.vanillapay.monitor.parse.parseReceipt
import com.vanillapay.monitor.parse.rawText
import org.junit.Assert.*
import org.junit.Test

class ReceiptParserTest {
    @Test fun alipayReceiptTitleParsesScreenshotAmount() {
        val parsed = parseReceipt(alipayTitleNotice)
        assertNotNull("截图中的支付宝收款通知应被识别", parsed)
        assertEquals(Channel.Alipay, parsed!!.channel)
        assertEquals(5000L, parsed.cents)
        assertNull(parsed.tradeNo)
    }

    @Test fun alipayReceiptTitleFingerprintUsesCentsWithoutTreatingNotesAsTradeNumbers() {
        val original = parseReceipt(alipayTitleNotice)!!
        assertEquals("alipay\nreceipt-title\n5000", original.fingerprint)

        val updated = parseReceipt(alipayTitleNotice.copy(
            title = " [2条]你已成功收款50.0元(订单号：ABC) ",
            body = " [2条]已转入余额 \t免费领取官方收钱码>> ",
        ))!!
        assertEquals(2, updated.groupCount)
        assertEquals(original.cents, updated.cents)
        assertEquals(original.fingerprint, updated.fingerprint)
        assertNull(updated.tradeNo)

        val withoutNote = parseReceipt(alipayTitleNotice.copy(title = "你已成功收款50元"))!!
        assertEquals(original.fingerprint, withoutNote.fingerprint)
        val differentAmount = parseReceipt(alipayTitleNotice.copy(title = "你已成功收款51.00元"))!!
        assertEquals(5100L, differentAmount.cents)
        assertEquals("alipay\nreceipt-title\n5100", differentAmount.fingerprint)
    }

    @Test fun alipayReceiptTitleRejectsOtherSourcesAndUnrelatedText() {
        val sample = alipayTitleNotice
        for (packageName in listOf("com.example.chat", Channel.WeChat.packageName)) {
            assertNull(packageName, parseReceipt(sample.copy(packageName = packageName)))
        }
        val titles = listOf(
            "付款50.00元", "退款50.00元", "转出50.00元", "收款请求50.00元",
            "", "支付宝", "群聊", "活动提醒", "提醒：你已成功收款50.00元",
            "你已成功收款50.00元（付款）", "你已成功收款50.00元（退款）", "你已成功收款50.00元（转出）",
        )
        for (title in titles) {
            assertNull(title, parseReceipt(sample.copy(title = title)))
        }
        val bodies = listOf(
            "", "已转入余额", "已转入余额 查看详情", "已转入余额免费领取官方收钱码>>",
            "已转入余额\n免费领取官方收钱码>>", "已转入余额 免费领取官方收钱码>> 活动",
            "付款50.00元", "退款50.00元", "转出50.00元",
        )
        for (body in bodies) {
            assertNull(body, parseReceipt(sample.copy(body = body)))
        }
    }

    @Test fun alipayReceiptTitleRejectsInvalidAmountsAndIncompleteTitles() {
        val amounts = listOf("0", "0.00", "-50.00", "50.001", "1,000.00", "50e0", "５０.００", "123456789012345")
        for (amount in amounts) {
            assertNull(amount, parseReceipt(alipayTitleNotice.copy(title = "你已成功收款${amount}元")))
        }
        val titles = listOf(
            "你已成功收款50.00", "你已成功收款50.00元（老顾客",
            "你已成功收款50.00元（老顾客)", "你已成功收款50.00元()",
            "你已成功收款50.00元（老\n顾客）", "你已成功收款50.00元（老顾客）尾随文字",
        )
        for (title in titles) {
            assertNull(title, parseReceipt(alipayTitleNotice.copy(title = title)))
        }
        val max = parseReceipt(alipayTitleNotice.copy(title = "你已成功收款99999999999999.99元"))
        assertEquals(9999999999999999L, max?.cents)
    }

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
        val legacyAlipay = parseReceipt(notice(Channel.Alipay, "支付宝", "你已成功收款1.00元"))
        assertEquals("alipay\n你已成功收款1.00元", legacyAlipay?.fingerprint)
        val personalAlipay = parseReceipt(notice(Channel.Alipay, "支付宝收款", "个人收款码到账¥0.01"))
        assertEquals("alipay\n个人收款码到账¥0.01", personalAlipay?.fingerprint)
        val withID = parseReceipt(notice(Channel.Alipay, "支付宝", "支付宝成功收款3.30元\n交易号：ABC_123-45"))
        assertEquals("ABC_123-45", withID?.tradeNo)
        assertEquals("alipay\n支付宝成功收款3.30元 交易号：ABC_123-45", withID?.fingerprint)
        val legacyWeChat = parseReceipt(notice(Channel.WeChat, "微信支付", "微信支付收款0.01元(朋友到店)"))
        assertEquals("wxpay\n收款0.01元(朋友到店)", legacyWeChat?.fingerprint)
        val groupedWeChat = parseReceipt(notice(Channel.WeChat, "微信", "[2条]微信支付: 收款0.02元"))
        assertEquals("wxpay\n收款0.02元", groupedWeChat?.fingerprint)
        assertEquals(2, groupedWeChat?.groupCount)
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

internal val alipayTitleNotice = PaymentNotice(
    "alipay-provider", Channel.Alipay.packageName, 1_000, 1_000,
    "你已成功收款50.00元（老顾客…）", "已转入余额 免费领取官方收钱码>>",
)
