package com.vanillapay.monitor

import com.vanillapay.monitor.parse.RuleParser
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class RuleParserTest {
    @Test
    fun parsesRulesAndVersion() {
        val json = """{"version":2,"rules":[
            {"channel":"wxpay","package":"com.tencent.mm","keyword":"收款","amountRegex":"收款([0-9.]+)元"}
        ]}"""
        val result = RuleParser.parse(json)
        assertEquals(2, result.version)
        assertEquals(1, result.rules.size)
        assertEquals("wxpay", result.rules[0].channel)
        assertEquals("com.tencent.mm", result.rules[0].packageName)
    }

    @Test
    fun returnsNullVersionOnGarbage() {
        assertEquals(0, RuleParser.parse("not json").version)
        assertTrue(RuleParser.parse("not json").rules.isEmpty())
    }
}
