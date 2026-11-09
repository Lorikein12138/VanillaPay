package com.vanillapay.monitor

import org.junit.Assert.assertEquals
import org.junit.Test

class ReceiptDisplayTest {
    @Test
    fun receiptFailuresUseCustomerFacingCopy() {
        assertEquals("与已上报记录冲突", receiptFailureDescription("EVENT_CONFLICT", 100))
        assertEquals("金额超出范围，服务器未接收", receiptFailureDescription("VALIDATION_ERROR", 1_000_000_000))
        assertEquals("服务器拒绝了这条记录", receiptFailureDescription("VALIDATION_ERROR", 999_999_999))
        assertEquals("服务器拒绝了这条记录", receiptFailureDescription("UNKNOWN_ERROR", 100))
    }
}
