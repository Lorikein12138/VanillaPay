package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.File
import java.util.zip.InflaterInputStream
import kotlin.math.max

class LauncherIconInsetTest {
    @Test
    fun `manifest uses adaptive launcher icons for Android 16 themed notification surfaces`() {
        val manifest = File("src/main/AndroidManifest.xml").readText()

        assertTrue(manifest.contains("""android:icon="@mipmap/ic_launcher""""))
        assertTrue(manifest.contains("""android:roundIcon="@mipmap/ic_launcher_round""""))
        assertTrue(File("src/main/res/mipmap-anydpi-v26/ic_launcher.xml").isFile)
        assertTrue(File("src/main/res/mipmap-anydpi-v26/ic_launcher_round.xml").isFile)
    }

    @Test
    fun `adaptive launcher icons expose monochrome layer for themed and vendor notification surfaces`() {
        val adaptiveIcons = listOf(
            File("src/main/res/mipmap-anydpi-v26/ic_launcher.xml"),
            File("src/main/res/mipmap-anydpi-v26/ic_launcher_round.xml"),
        )

        for (file in adaptiveIcons) {
            val source = file.readText()
            assertTrue(
                source.contains("""<monochrome android:drawable="@drawable/ic_launcher_monochrome" />"""),
                "${file.path} is missing the monochrome adaptive-icon layer",
            )
        }

        val monochromeIcon = File("src/main/res/drawable/ic_launcher_monochrome.xml")
        assertTrue(monochromeIcon.isFile, "${monochromeIcon.path} must exist")
        val monochromeSource = monochromeIcon.readText()
        assertTrue(monochromeSource.contains("<vector"), "${monochromeIcon.path} must be a vector drawable")
        assertTrue(
            monochromeSource.contains("""android:fillColor="#FFFFFFFF""""),
            "${monochromeIcon.path} must use an opaque monochrome foreground",
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
    fun `legacy launcher icons keep transparent inset for rounded system masks`() {
        val files = File("src/main/res").walkTopDown()
            .filter { it.isFile && it.name in setOf("ic_launcher.png", "ic_launcher_round.png") }
            .toList()

        assertTrue(files.isNotEmpty())

        for (file in files) {
            val png = PngAlpha.read(file)
            val bounds = png.visibleBounds()
            val maxRatio = max(bounds.width.toDouble() / png.width, bounds.height.toDouble() / png.height)

            assertTrue(maxRatio <= 0.86, "${file.path} visible ratio is $maxRatio")
        }
    }

    @Test
    fun `adaptive launcher foreground stays inside the safe visual area`() {
        val files = File("src/main/res").walkTopDown()
            .filter { it.isFile && it.name == "ic_launcher_foreground.png" }
            .toList()

        assertTrue(files.isNotEmpty())

        for (file in files) {
            val png = PngAlpha.read(file)
            val bounds = png.visibleBounds()
            val maxRatio = max(bounds.width.toDouble() / png.width, bounds.height.toDouble() / png.height)

            assertTrue(maxRatio <= 0.70, "${file.path} visible ratio is $maxRatio")
        }
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
