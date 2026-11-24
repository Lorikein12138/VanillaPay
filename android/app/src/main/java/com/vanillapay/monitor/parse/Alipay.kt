package com.vanillapay.monitor.parse

private val titles = setOf("支付宝", "支付宝收款", "收钱到账", "收款到账", "收款通知", "支付宝通知")
private val successful = Regex("""^(?:你已成功收款|您已成功收款|支付宝成功收款)\s*[¥￥]?$MONEY\s*元(?=\s|[，。！(（]|$)""")
private val personal = Regex("""^个人收款码到账\s*[¥￥]$MONEY(?:\s*元)?(?=\s|[，。！(（]|$)""")
private val receiptTitle = Regex("""你已成功收款${MONEY}元(?:（[^（）()\r\n]+）|\([^（）()\r\n]+\))?""")
private val receiptTitleBody = Regex("""已转入余额[ \t]+免费领取官方收钱码>>""")

internal fun alipay(title: String, body: String) =
    if (title in titles) startsWithReceipt(body, successful, personal) else null

internal fun alipayTitleAmount(title: String, body: String) =
    if (receiptTitleBody.matches(body)) receiptTitle.matchEntire(title) else null
