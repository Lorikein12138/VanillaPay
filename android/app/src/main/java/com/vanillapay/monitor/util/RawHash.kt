package com.vanillapay.monitor.util

import java.security.MessageDigest

object RawHash {
    fun of(packageName: String, text: String, postTime: Long): String =
        MessageDigest.getInstance("SHA-256")
            .digest("$packageName|$text|$postTime".toByteArray(Charsets.UTF_8))
            .joinToString("") { "%02x".format(it) }
}
