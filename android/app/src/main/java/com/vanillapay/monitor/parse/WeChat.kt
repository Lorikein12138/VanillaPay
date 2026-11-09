package com.vanillapay.monitor.parse

private val titles = setOf("微信支付", "微信支付收款", "微信收款助手")
private val successful = Regex("""^(?:微信支付[:：]\s*)?(?:微信支付\s*)?收款\s*[¥￥]?$MONEY\s*元(?=\s|[，。！(（]|$)""")
private val personal = Regex("""^(?:微信支付[:：]\s*)?个人收款码到账\s*[¥￥]$MONEY(?:\s*元)?(?=\s|[，。！(（]|$)""")

internal fun weChat(title: String, body: String, grouped: Boolean) =
    if (title in titles || (title == "微信" && grouped && body.startsWith("微信支付"))) {
        startsWithReceipt(body, successful, personal)
    } else null
