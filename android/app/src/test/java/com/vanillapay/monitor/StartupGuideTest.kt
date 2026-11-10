package com.vanillapay.monitor

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(application = Application::class, sdk = [35], qualifiers = "zh-rCN-w360dp-h900dp-notnight-mdpi")
class StartupGuideTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun vivoGuideShowsBothStepsAndDispatchesTheirActionsSeparately() {
        val actions = mutableListOf<String>()
        compose.setContent {
            MonitorTheme(dark = false) {
                StartupGuideDialog(
                    startupGuide("iQOO"),
                    openStartup = { actions.add("startup") },
                    openBackground = { actions.add("background") },
                    onConfirm = { actions.add("confirm") },
                    onDismiss = {},
                )
            }
        }

        compose.onNodeWithText("第一步：自启动").assertIsDisplayed()
        compose.onNodeWithText("第二步：允许后台高耗电").assertIsDisplayed()
        compose.onNodeWithText("到账要等打开 app 才上报", substring = true).assertIsDisplayed()
        compose.onNodeWithText("打开自启动设置").assertIsDisplayed().performClick()
        compose.onNodeWithText("打开后台耗电设置").assertIsDisplayed().performClick()
        compose.onNodeWithText("我已确认").assertIsDisplayed().performClick()
        compose.runOnIdle { assertEquals(listOf("startup", "background", "confirm"), actions) }
    }
}
