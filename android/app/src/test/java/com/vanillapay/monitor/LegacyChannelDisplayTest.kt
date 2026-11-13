package com.vanillapay.monitor

import android.app.Application
import androidx.compose.foundation.layout.Column
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(application = Application::class, sdk = [35], qualifiers = "zh-rCN-w360dp-h900dp-mdpi")
class LegacyChannelDisplayTest {
    @get:Rule val compose = createComposeRule()

    @Test fun receiptRowsShowAmountsAndStatusesForRetiredChannels() {
        compose.setContent {
            MonitorTheme {
                Column {
                    ReceiptRow(Receipt("1", "device", "qqpay", 101, 0, "", status = MATCHED))
                    ReceiptRow(Receipt("2", "device", "qqpay", 202, 0, "", status = QUEUED))
                    ReceiptRow(Receipt("3", "device", "qqpay", 303, 0, "", status = FAILED))
                }
            }
        }
        compose.onAllNodesWithText("其他渠道").assertCountEquals(3)
        for (text in listOf("¥1.01", "¥2.02", "¥3.03", "已匹配", "待上传", "上报失败")) {
            compose.onNodeWithText(text).assertIsDisplayed()
        }
    }

    @Test fun diagnosticsShowsRetiredChannelAndRecognizedAmount() {
        compose.setContent {
            MonitorTheme {
                DiagnosticsScreen(true, listOf(Diagnostic("1", 0, "qqpay", "旧到账通知", 456)), {}, {}, {})
            }
        }
        compose.onNodeWithText("其他渠道 · ${localTime(0, "MM-dd HH:mm:ss")}").assertIsDisplayed()
        compose.onNodeWithText("识别为 其他渠道 · ¥4.56").assertIsDisplayed()
    }

    @Test @Config(qualifiers = "en-rUS-w360dp-h900dp-mdpi")
    fun receiptRowUsesEnglishFallbackForAnUnknownChannel() {
        compose.setContent {
            MonitorTheme { ReceiptRow(Receipt("1", "device", "unknown", 101, 0, "")) }
        }
        compose.onNodeWithText("Other").assertIsDisplayed()
        compose.onNodeWithText("¥1.01").assertIsDisplayed()
    }
}
