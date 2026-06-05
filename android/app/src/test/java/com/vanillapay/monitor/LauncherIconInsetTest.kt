package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.File
import java.util.zip.InflaterInputStream

class LauncherIconInsetTest {
    @Test
    fun `manifest uses gkd style drawable launcher icon for vendor notification surfaces`() {
        val manifest = File("src/main/AndroidManifest.xml").readText()

        assertTrue(manifest.contains("""android:icon="@drawable/ic_launcher""""))
        assertTrue(manifest.contains("""android:roundIcon="@drawable/ic_launcher""""))
        assertTrue(File("src/main/res/drawable/ic_launcher.xml").isFile)
        assertTrue(File("src/main/res/drawable-v26/ic_launcher.xml").isFile)
        assertTrue(!manifest.contains("@mipmap/ic_launcher"))
    }

    @Test
    fun `adaptive launcher icon mirrors gkd vector resource layout`() {
        val adaptiveIcon = File("src/main/res/drawable-v26/ic_launcher.xml")
        val adaptiveSource = adaptiveIcon.readText()

        assertTrue(
            adaptiveSource.contains("""<background android:drawable="@drawable/ic_launcher_background" />"""),
            "${adaptiveIcon.path} is missing the vector background layer",
        )
        assertTrue(
            adaptiveSource.contains("""<foreground android:drawable="@drawable/ic_launcher_foreground" />"""),
            "${adaptiveIcon.path} is missing the vector foreground layer",
        )
        assertTrue(
            adaptiveSource.contains("""<monochrome android:drawable="@drawable/ic_launcher_foreground" />"""),
            "${adaptiveIcon.path} must reuse the foreground vector as its monochrome layer",
        )

        val foregroundIcon = File("src/main/res/drawable/ic_launcher_foreground.xml")
        assertTrue(foregroundIcon.isFile, "${foregroundIcon.path} must exist")
        val foregroundSource = foregroundIcon.readText()
        assertTrue(foregroundSource.contains("<vector"), "${foregroundIcon.path} must be a vector drawable")
        assertTrue(
            foregroundSource.contains("""android:fillColor="#FFFFFFFF""""),
            "${foregroundIcon.path} must use an opaque monochrome foreground",
        )

        val backgroundIcon = File("src/main/res/drawable/ic_launcher_background.xml")
        assertTrue(backgroundIcon.isFile, "${backgroundIcon.path} must exist")
        assertTrue(backgroundIcon.readText().contains("<vector"), "${backgroundIcon.path} must be a vector drawable")
    }

    @Test
    fun `legacy launcher drawable keeps the foreground safely inset`() {
        val launcherIcon = File("src/main/res/drawable/ic_launcher.xml")
        assertTrue(launcherIcon.isFile, "${launcherIcon.path} must exist")
        val source = launcherIcon.readText()

        assertTrue(source.contains("<vector"), "${launcherIcon.path} must be a vector fallback")
        assertTrue(source.contains("""android:viewportWidth="108""""))
        assertTrue(source.contains("""android:viewportHeight="108""""))
        assertTrue(source.contains("""android:pathData="M24,30H43L54,63L65,30H84L63,78H45L24,30Z""""))
        assertTrue(
            source.contains("""android:pathData="M67,64C79,63 89,68 96,81C83,85 72,82 64,74C63,70 64,67 67,64Z""""),
            "${launcherIcon.path} foreground must stay within the Android adaptive-icon safe area",
        )
    }

    @Test
    fun `notification small icon is a monochrome vector drawable`() {
        val icon = File("src/main/res/drawable/ic_notification_vanillapay.xml")
        assertTrue(icon.isFile, "${icon.path} must exist")

        val source = icon.readText()
        assertTrue(source.contains("<vector"), "${icon.path} must be a vector drawable")
        assertTrue(
            source.contains("""android:fillColor="#FFFFFFFF""""),
            "${icon.path} must use an opaque monochrome foreground",
        )
    }

    @Test
    fun `notification small icon has density png fallback for vendor system ui`() {
        val files = File("src/main/res").walkTopDown()
            .filter { it.isFile && it.name == "ic_notification_vanillapay.png" }
            .toList()

        assertTrue(files.size >= 5)

        for (file in files) {
            val png = PngAlpha.read(file)
            val bounds = png.visibleBounds()

            assertTrue(bounds.width > 0, "${file.path} has no visible pixels")
            assertTrue(bounds.height > 0, "${file.path} has no visible pixels")
        }
    }

    @Test
    fun `unused static launcher icon experiment resources are absent`() {
        val files = File("src/main/res").walkTopDown()
            .filter { it.isFile && it.name.startsWith("ic_launcher_static") }
            .toList()

        assertTrue(files.isEmpty(), "static launcher resources should not be packaged: $files")
    }

    @Test
    fun `unused mipmap launcher and template status icons are absent`() {
        val staleFiles = File("src/main/res").walkTopDown()
            .filter { file ->
                file.isFile && (
                    file.name == "ic_stat_monitor.png" ||
                        (file.extension == "png" && file.name.startsWith("ic_launcher")) ||
                        file.path.contains("mipmap-anydpi-v26")
                    )
            }
            .toList()

        assertTrue(staleFiles.isEmpty(), "stale icon resources should not be packaged: $staleFiles")
    }

    private data class Bounds(val width: Int, val height: Int)

    private class PngAlpha(
        val width: Int,
        val height: Int,
        private val colorType: Int,
        private val pixels: ByteArray,
    ) {
        fun visibleBounds(): Bounds {
            var minX = width
            var minY = height
            var maxX = -1
            var maxY = -1

            for (y in 0 until height) {
                for (x in 0 until width) {
                    if (isVisible(x, y)) {
                        minX = minOf(minX, x)
                        minY = minOf(minY, y)
                        maxX = maxOf(maxX, x)
                        maxY = maxOf(maxY, y)
                    }
                }
            }

            return if (maxX < minX || maxY < minY) {
                Bounds(0, 0)
            } else {
                Bounds(maxX - minX + 1, maxY - minY + 1)
            }
        }

        private fun isVisible(x: Int, y: Int): Boolean {
            val stride = width * bytesPerPixel(colorType)
            val offset = y * stride + x * bytesPerPixel(colorType)

            return when (colorType) {
                6 -> pixels[offset + 3].toInt() and 0xff > 0
                4 -> pixels[offset + 1].toInt() and 0xff > 0
                else -> true
            }
        }

        companion object {
            fun read(file: File): PngAlpha {
                val bytes = file.readBytes()
                var index = 8
                var width = 0
                var height = 0
                var bitDepth = 0
                var colorType = 0
                val idat = ByteArrayOutputStream()

                while (index < bytes.size) {
                    val length = bytes.readInt(index)
                    val type = bytes.copyOfRange(index + 4, index + 8).toString(Charsets.US_ASCII)
                    val dataStart = index + 8

                    when (type) {
                        "IHDR" -> {
                            width = bytes.readInt(dataStart)
                            height = bytes.readInt(dataStart + 4)
                            bitDepth = bytes[dataStart + 8].toInt() and 0xff
                            colorType = bytes[dataStart + 9].toInt() and 0xff
                        }
                        "IDAT" -> idat.write(bytes, dataStart, length)
                        "IEND" -> break
                    }

                    index += length + 12
                }

                require(bitDepth == 8) { "${file.path} uses unsupported PNG bit depth $bitDepth" }

                val raw = InflaterInputStream(ByteArrayInputStream(idat.toByteArray())).readBytes()
                val stride = width * bytesPerPixel(colorType)
                val out = ByteArray(height * stride)
                var rawIndex = 0

                for (y in 0 until height) {
                    val filter = raw[rawIndex++].toInt() and 0xff
                    val rowOffset = y * stride
                    for (x in 0 until stride) {
                        val current = raw[rawIndex++].toInt() and 0xff
                        val left = if (x >= bytesPerPixel(colorType)) out[rowOffset + x - bytesPerPixel(colorType)].toInt() and 0xff else 0
                        val up = if (y > 0) out[rowOffset + x - stride].toInt() and 0xff else 0
                        val upLeft = if (y > 0 && x >= bytesPerPixel(colorType)) {
                            out[rowOffset + x - stride - bytesPerPixel(colorType)].toInt() and 0xff
                        } else {
                            0
                        }
                        val value = when (filter) {
                            0 -> current
                            1 -> current + left
                            2 -> current + up
                            3 -> current + ((left + up) / 2)
                            4 -> current + paeth(left, up, upLeft)
                            else -> error("${file.path} uses unsupported PNG filter $filter")
                        }
                        out[rowOffset + x] = (value and 0xff).toByte()
                    }
                }

                return PngAlpha(width, height, colorType, out)
            }

            private fun paeth(left: Int, up: Int, upLeft: Int): Int {
                val p = left + up - upLeft
                val pa = kotlin.math.abs(p - left)
                val pb = kotlin.math.abs(p - up)
                val pc = kotlin.math.abs(p - upLeft)

                return when {
                    pa <= pb && pa <= pc -> left
                    pb <= pc -> up
                    else -> upLeft
                }
            }
        }
    }

    private companion object {
        fun ByteArray.readInt(offset: Int): Int =
            ((this[offset].toInt() and 0xff) shl 24) or
                ((this[offset + 1].toInt() and 0xff) shl 16) or
                ((this[offset + 2].toInt() and 0xff) shl 8) or
                (this[offset + 3].toInt() and 0xff)

        fun bytesPerPixel(colorType: Int): Int = when (colorType) {
            0 -> 1
            2 -> 3
            4 -> 2
            6 -> 4
            else -> error("Unsupported PNG color type $colorType")
        }
    }
}
