package com.vanillapay.monitor

import java.math.BigDecimal
import java.math.RoundingMode

object Money {
    fun toCents(amount: String): Long =
        BigDecimal(amount).multiply(BigDecimal(100)).setScale(0, RoundingMode.HALF_UP).toLong()

    fun format(cents: Long): String =
        BigDecimal(cents).divide(BigDecimal(100)).setScale(2, RoundingMode.HALF_UP).toPlainString()
}
