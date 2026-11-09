package com.vanillapay.monitor.parse

private val titles = setOf("支付宝", "支付宝收款", "收钱到账", "收款到账", "收款通知", "支付宝通知")
private val successful = Regex("""^(?:你已成功收款|您已成功收款|支付宝成功收款)\s*[¥￥]?$MONEY\s*元(?=\s|[，。！(（]|$)""")
private val personal = Regex("""^个人收款码到账\s*[¥￥]$MONEY(?:\s*元)?(?=\s|[，。！(（]|$)""")

internal fun alipay(title: String, body: String) =
    if (title in titles) startsWithReceipt(body, successful, personal) else null
