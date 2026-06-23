package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

class ReleaseShrinkConfigTest {
    @Test
    fun `release shrink keeps firebase component registrars reflective members`() {
        val rules = File("proguard-rules.pro").readText()

        assertTrue(
            rules.contains("implements com.google.firebase.components.ComponentRegistrar") &&
                (rules.contains("{ *; }") || rules.contains("public <init>();")),
            "ML Kit discovers ComponentRegistrar implementations reflectively; release shrink must keep their constructors.",
        )
    }
}
