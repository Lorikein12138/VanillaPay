package com.vanillapay.monitor.net

import okhttp3.CertificatePinner
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

object PinnedClientFactory {
    fun create(host: String, pinSha256: String?): OkHttpClient {
        val builder = OkHttpClient.Builder()
            .connectTimeout(5, TimeUnit.SECONDS)
            .callTimeout(10, TimeUnit.SECONDS)
        if (host.isNotEmpty() && !pinSha256.isNullOrEmpty()) {
            builder.certificatePinner(
                CertificatePinner.Builder()
                    .add(host, "sha256/$pinSha256")
                    .build(),
            )
        }
        return builder.build()
    }
}
