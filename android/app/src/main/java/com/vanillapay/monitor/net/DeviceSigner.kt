package com.vanillapay.monitor.net

import java.security.MessageDigest

class DeviceSigner {
    fun sign(params: Map<String, String>, key: String): String {
        val base = params
            .filterKeys { it != "sign" }
            .filterValues { it.isNotEmpty() }
            .toSortedMap()
            .entries
            .joinToString("&") { "${it.key}=${it.value}" }
        return md5(base + key)
    }

    private fun md5(value: String): String =
        MessageDigest.getInstance("MD5")
            .digest(value.toByteArray(Charsets.UTF_8))
            .joinToString("") { "%02x".format(it) }
}
