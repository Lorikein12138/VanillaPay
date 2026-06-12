package com.vanillapay.monitor

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.File
import java.util.zip.InflaterInputStream

class LauncherIconInsetTest {
    @Test
    fun `manifest uses drawable launcher icon for both icon and roundIcon`() {
        val manifest = File("src/main/AndroidManifest.xml").readText()

        assertTrue(manifest.contains("""android:icon="@drawable/ic_launcher""""))
        assertTrue(manifest.contains("""android:roundIcon="@drawable/ic_launcher""""))
        assertTrue(File("src/main/res/drawable-anydpi-v26/ic_launcher.xml").isFile)
        assertTrue(!manifest.contains("@mipmap/ic_launcher"))
    }

    @Test
    fun `adaptive icon uses bitmap background art and transparent foreground`() {
        val adaptive = File("src/main/res/drawable-anydpi-v26/ic_launcher.xml").readText()
        assertTrue(
            adaptive.contains("""<background android:drawable="@drawable/ic_launcher_bg" />"""),
            "adaptive icon must use the full-bleed bitmap art as its background layer",
        )
        assertTrue(
            adaptive.contains("""<foreground android:drawable="@android:color/transparent" />"""),
            "adaptive icon foreground must be transparent so launchers can't shrink it into a seam",
        )

        val bg = File("src/main/res").walkTopDown()
            .filter { it.isFile && it.name == "ic_launcher_bg.png" }
            .toList()
        assertTrue(bg.size >= 5, "expected per-density background art bitmaps, found $bg")
    }

    @Test
    fun `legacy V launcher vectors are removed`() {
        assertTrue(!File("src/main/res/drawable/ic_launcher.xml").exists())
        assertTrue(!File("src/main/res/drawable/ic_launcher_foreground.xml").exists())
    }

    @Test
    fun `launcher background art bitmaps are full-bleed with no transparent corners`() {
        val files = File("src/main/res").walkTopDown()
            .filter { it.isFile && it.name == "ic_launcher_bg.png" }
            .toList()

        assertTrue(files.size >= 5, "expected per-density background art bitmaps, found $files")

        for (file in files) {
            val png = PngAlpha.read(file)
            val bounds = png.visibleBounds()
            assertTrue(
                bounds.left == 0 && bounds.top == 0 &&
                    bounds.right == png.width - 1 && bounds.bottom == png.height - 1,
                "${file.path} must be full-bleed (art reaches every edge so launchers never fill the corners white), got $bounds",
            )
        }
    }

    @Test
    fun `legacy launcher bitmaps exist and have visible pixels`() {
        val files = File("src/main/res").walkTopDown()
            .filter { it.isFile && it.name == "ic_launcher.png" }
            .toList()

        assertTrue(files.size >= 5, "expected per-density legacy launcher bitmaps, found $files")
        for (file in files) {
            assertTrue(!PngAlpha.read(file).visibleBounds().empty, "${file.path} has no visible pixels")
        }
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
            val bounds = PngAlpha.read(file).visibleBounds()
            assertTrue(!bounds.empty, "${file.path} has no visible pixels")
        }
    }

    @Test
    fun `unused mipmap launcher and template status icons are absent`() {
        val staleFiles = File("src/main/res").walkTopDown()
            .filter { file ->
                file.isFile && (
                    file.name == "ic_stat_monitor.png" ||
                        file.name == "ic_notification_large.png" ||
                        file.path.replace('\\', '/').contains("mipmap-anydpi-v26")
                    )
            }
            .toList()

        assertTrue(staleFiles.isEmpty(), "stale icon resources should not be packaged: $staleFiles")
    }

    private data class Bounds(val left: Int, val top: Int, val right: Int, val bottom: Int) {
        val empty: Boolean get() = right < left || bottom < top
    }

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
            return Bounds(minX, minY, maxX, maxY)
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
