package com.vanillapay.monitor.parse

import com.vanillapay.monitor.Money

class NotificationParser(private val rules: List<ParseRule>) {
    fun parse(packageName: String, title: String, text: String): ParsedNotification? {
        val haystack = normalize("$title $text")
        for (rule in rules) {
            if (rule.packageName != packageName) continue
            if (!matchesKeyword(haystack, rule.keyword)) continue
            val cents = runCatching {
                val match = Regex(rule.amountRegex).find(haystack) ?: return@runCatching null
                val raw = match.groupValues.getOrNull(1)
                    ?.replace(",", "")
                    ?.takeIf { it.isNotBlank() }
                    ?: return@runCatching null
                Money.toCents(raw)
            }.getOrNull() ?: continue
            if (cents <= 0) continue
            return ParsedNotification(rule.channel, cents)
        }
        return null
    }

    /** A blank keyword matches anything; otherwise any '|'-separated alternative must be present. */
    private fun matchesKeyword(haystack: String, keyword: String): Boolean {
        if (keyword.isBlank()) return true
        return keyword.split('|').any { it.isNotBlank() && haystack.contains(it) }
    }

    /**
     * Fold notification text so format variations across phone vendors and WeChat/Alipay
     * versions still match: drop all whitespace (so "收款 0.01 元" reads like "收款0.01元"),
     * normalize full-width digits/punctuation and unify the yuan symbol.
     */
    private fun normalize(raw: String): String {
        val sb = StringBuilder(raw.length)
        for (ch in raw) {
            when {
                ch.isWhitespace() -> Unit
                ch in '０'..'９' -> sb.append('0' + (ch - '０'))
                ch == '．' -> sb.append('.')
                ch == '，' -> sb.append(',')
                ch == '￥' -> sb.append('¥')
                else -> sb.append(ch)
            }
        }
        return sb.toString()
    }

    companion object {
        // Incoming-money keywords only (收款/到账/入账/收钱); deliberately excludes
        // 付款/支出/退款 so outgoing or refunded payments are never counted as income.
        private const val INCOME_KEYWORDS = "收款|到账|入账|收钱"

        // The amount sits right after an income keyword. Tolerate a few separators
        // (¥ : ：「金额」etc., up to 4 non-digit chars), accept thousands separators
        // and capture up to two decimals. The trailing 元/¥ is optional.
        private const val AMOUNT_REGEX =
            "(?:收款|到账|入账|收钱)[^0-9]{0,4}([0-9][0-9,]*(?:\\.[0-9]{1,2})?)"

        fun defaultRules(): List<ParseRule> = listOf(
            ParseRule("wxpay", "com.tencent.mm", INCOME_KEYWORDS, AMOUNT_REGEX),
            ParseRule("alipay", "com.eg.android.AlipayGphone", INCOME_KEYWORDS, AMOUNT_REGEX),
        )
    }
}
