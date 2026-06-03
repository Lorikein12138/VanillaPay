package com.vanillapay.monitor.parse

import com.vanillapay.monitor.Money

class NotificationParser(private val rules: List<ParseRule>) {
    fun parse(packageName: String, title: String, text: String): ParsedNotification? {
        val haystack = "$title $text"
        for (rule in rules) {
            if (rule.packageName != packageName) continue
            if (!haystack.contains(rule.keyword)) continue
            val match = Regex(rule.amountRegex).find(haystack) ?: continue
            val amount = match.groupValues.getOrNull(1) ?: continue
            return ParsedNotification(rule.channel, Money.toCents(amount))
        }
        return null
    }

    companion object {
        fun defaultRules(): List<ParseRule> = listOf(
            ParseRule("wxpay", "com.tencent.mm", "收款", """收款([0-9]+(?:\.[0-9]{1,2})?)元"""),
            ParseRule("alipay", "com.eg.android.AlipayGphone", "收款", """收款([0-9]+(?:\.[0-9]{1,2})?)元"""),
        )
    }
}
