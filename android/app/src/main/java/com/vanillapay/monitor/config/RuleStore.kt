package com.vanillapay.monitor.config

import android.content.Context
import com.vanillapay.monitor.parse.NotificationParser
import com.vanillapay.monitor.parse.RuleParser
import com.vanillapay.monitor.parse.RuleSet

class RuleStore(context: Context) {
    private val preferences = context.getSharedPreferences("vanillapay_rules", Context.MODE_PRIVATE)

    fun save(json: String) {
        preferences.edit().putString("rules_json", json).apply()
    }

    fun version(): Int = current().version

    fun current(): RuleSet {
        val json = preferences.getString("rules_json", null) ?: return defaults()
        val parsed = RuleParser.parse(json)
        return if (parsed.rules.isEmpty()) defaults() else parsed
    }

    private fun defaults(): RuleSet = RuleSet(1, NotificationParser.defaultRules())
}
