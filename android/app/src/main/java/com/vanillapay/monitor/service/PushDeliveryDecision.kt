package com.vanillapay.monitor.service

import com.vanillapay.monitor.net.ApiResult

enum class PushDeliveryDecision {
    SENT,
    RETRY,
}

object PushDeliveryDecider {
    fun decide(result: ApiResult?): PushDeliveryDecision {
        if (result?.ok != true) return PushDeliveryDecision.RETRY
        return when (result.status) {
            "matched", "already_done", "duplicate" -> PushDeliveryDecision.SENT
            else -> PushDeliveryDecision.RETRY
        }
    }
}
