package com.vanillapay.monitor.parse

import org.json.JSONObject

data class RuleSet(
    val version: Int,
    val rules: List<ParseRule>,
)

object RuleParser {
    fun parse(json: String): RuleSet {
        return runCatching {
            val root = JSONObject(json)
            val rows = root.optJSONArray("rules")
            val parsed = buildList {
                if (rows != null) {
                    for (index in 0 until rows.length()) {
                        val row = rows.getJSONObject(index)
                        add(
                            ParseRule(
                                channel = row.getString("channel"),
                                packageName = row.getString("package"),
                                keyword = row.getString("keyword"),
                                amountRegex = row.getString("amountRegex"),
                            ),
                        )
                    }
                }
            }
            RuleSet(root.optInt("version", 0), parsed)
        }.getOrElse {
            RuleSet(0, emptyList())
        }
    }
}
