package com.vanillapay.monitor

import com.vanillapay.monitor.net.ApiResult
import com.vanillapay.monitor.service.PushDeliveryDecider
import com.vanillapay.monitor.service.PushDeliveryDecision
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class PushDeliveryDecisionTest {
    @Test
    fun `matched push is marked sent`() {
        val result = apiResult(status = "matched", matched = true)

        assertEquals(PushDeliveryDecision.SENT, PushDeliveryDecider.decide(result))
    }

    @Test
    fun `already done push is marked sent`() {
        val result = apiResult(status = "already_done", matched = true)

        assertEquals(PushDeliveryDecision.SENT, PushDeliveryDecider.decide(result))
    }

    @Test
    fun `duplicate push is marked sent`() {
        val result = apiResult(status = "duplicate", matched = true)

        assertEquals(PushDeliveryDecision.SENT, PushDeliveryDecider.decide(result))
    }

    @Test
    fun `unmatched push stays retryable`() {
        val result = apiResult(status = "unmatched", matched = false)

        assertEquals(PushDeliveryDecision.RETRY, PushDeliveryDecider.decide(result))
    }

    @Test
    fun `failed request stays retryable`() {
        val result = apiResult(ok = false, code = -1, status = "", matched = false)

        assertEquals(PushDeliveryDecision.RETRY, PushDeliveryDecider.decide(result))
    }

    private fun apiResult(
        ok: Boolean = true,
        code: Int = 1,
        status: String,
        matched: Boolean,
    ) = ApiResult(
        ok = ok,
        code = code,
        status = status,
        matched = matched,
        serverTime = 0,
        parseRulesVersion = 0,
    )
}
