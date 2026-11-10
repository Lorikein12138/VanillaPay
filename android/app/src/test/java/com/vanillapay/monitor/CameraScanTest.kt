package com.vanillapay.monitor

import com.google.zxing.BarcodeFormat
import com.google.zxing.MultiFormatWriter
import com.google.zxing.RGBLuminanceSource
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class CameraScanTest {
    @Test
    fun readsRotatedQr() {
        val text = "vanillapay://test"
        val code = MultiFormatWriter().encode(text, BarcodeFormat.QR_CODE, 160, 160)
        val rotated = IntArray(code.width * code.height) { index ->
            val x = index % code.width
            val y = index / code.width
            if (code[y, code.width - 1 - x]) BLACK else WHITE
        }
        assertEquals(text, decodeQr(RGBLuminanceSource(code.width, code.height, rotated)))
    }

    @Test
    fun rejectsOtherBarcodes() {
        val code = MultiFormatWriter().encode("1234567890", BarcodeFormat.CODE_128, 240, 80)
        val pixels = IntArray(code.width * code.height) { index ->
            if (code[index % code.width, index / code.width]) BLACK else WHITE
        }
        assertNull(decodeQr(RGBLuminanceSource(code.width, code.height, pixels)))
    }

    private companion object {
        const val BLACK = -0x1000000
        const val WHITE = -1
    }
}
