package com.vanillapay.monitor.parse

import java.math.BigDecimal

enum class Channel(val wire: String, val packageName: String, val display: String) {
    Alipay("alipay", "com.eg.android.AlipayGphone", "支付宝"),
    WeChat("wxpay", "com.tencent.mm", "微信"),
}

data class PaymentNotice(
    val key: String,
    val packageName: String,
    val postTime: Long,
    val notificationTime: Long,
    val title: String,
    val body: String,
)

data class ParsedReceipt(
    val channel: Channel,
    val cents: Long,
    val tradeNo: String?,
    val fingerprint: String,
    val groupCount: Int?,
)

internal const val MONEY = "([0-9]{1,14}(?:[.][0-9]{1,2})?)"
private val tradeNo = Regex("""(?:交易号|交易单号|订单号)\s*[:：]\s*([A-Za-z0-9_-]{1,128})(?![A-Za-z0-9_-])""")
private val group = Regex("""^\s*\[\s*([0-9]+)\s*条\s*]\s*""")
private val blocked = Regex("付款|转出|退款|退回|收款请求|请收款|向你收款|向您收款|收款方")
private val diagnosticWords = Regex("收款|到账|付款|元|[¥￥]")

fun parseReceipt(notice: PaymentNotice): ParsedReceipt? {
    val channel = Channel.entries.firstOrNull { it.packageName == notice.packageName } ?: return null
    val title = group.replaceFirst(notice.title, "").trim()
    val body = group.replaceFirst(notice.body, "").trim()
    if (blocked.containsMatchIn(title) || blocked.containsMatchIn(body)) return null
    val count = (group.find(notice.title) ?: group.find(notice.body))?.groupValues?.get(1)?.toIntOrNull()
    val match = when (channel) {
        Channel.Alipay -> alipay(title, body)
        Channel.WeChat -> weChat(title, body, count != null)
    } ?: return null
    val cents = runCatching { BigDecimal(match.groupValues[1]).movePointRight(2).longValueExact() }.getOrNull()
        ?.takeIf { it > 0 } ?: return null
    val canonical = if (channel == Channel.WeChat) body.replaceFirst(Regex("""^微信支付[:：]?\s*"""), "") else body
    return ParsedReceipt(
        channel, cents, tradeNo.find(body)?.groupValues?.get(1),
        "${channel.wire}\n${canonical.replace(Regex("\\s+"), " ")}", count,
    )
}

fun isDiagnostic(notice: PaymentNotice) =
    Channel.entries.any { it.packageName == notice.packageName } &&
        (diagnosticWords.containsMatchIn(notice.title) || diagnosticWords.containsMatchIn(notice.body))

fun PaymentNotice.rawText(): String {
    val text = listOf(title, body).filter { it.isNotBlank() }.joinToString("\n")
    return text.substring(0, text.offsetByCodePoints(0, minOf(4096, text.codePointCount(0, text.length))))
}

internal fun startsWithReceipt(body: String, vararg patterns: Regex): MatchResult? =
    patterns.firstNotNullOfOrNull { it.find(body) }
