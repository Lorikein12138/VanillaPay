package com.vanillapay.monitor.net

import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

class DeviceSigner {
    fun sign(params: Map<String, String>, key: String): String {
        val base = params
            .filterKeys { it != "sign" }
            .filterValues { it.isNotEmpty() }
            .toSortedMap()
            .entries
            .joinToString("&") { "${it.key}=${it.value}" }
        return hmacSha256(base, key)
    }

    private fun hmacSha256(value: String, key: String): String =
        Mac.getInstance("HmacSHA256").apply {
            init(SecretKeySpec(key.toByteArray(Charsets.UTF_8), "HmacSHA256"))
        }
            .doFinal(value.toByteArray(Charsets.UTF_8))
            .joinToString("") { "%02x".format(it) }
}
