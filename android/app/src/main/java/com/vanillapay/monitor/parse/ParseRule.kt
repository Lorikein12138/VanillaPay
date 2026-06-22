package com.vanillapay.monitor.parse

data class ParseRule(
    val channel: String,
    val packageName: String,
    val keyword: String,
    val amountRegex: String,
)

data class ParsedNotification(
    val channel: String,
    val amountCents: Long,
)
