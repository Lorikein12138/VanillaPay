package com.vanillapay.monitor.parse

private val titles = setOf("QQ钱包", "QQ钱包收款", "QQ收款")
private val successful = Regex("""^(?:QQ钱包\s*[:：·…]?\s*)?(?:你已成功|您已成功|成功)?收款\s*[¥￥]?$MONEY\s*元(?=\s|[，。！(（]|$)""")
private val credited = Regex("""^(?:QQ钱包\s*[:：·…]?\s*)?收款到账\s*[¥￥]$MONEY(?:\s*元)?(?=\s|[，。！(（]|$)""")

internal fun qqPay(title: String, body: String) =
    if (title in titles) startsWithReceipt(body, successful, credited) else null
