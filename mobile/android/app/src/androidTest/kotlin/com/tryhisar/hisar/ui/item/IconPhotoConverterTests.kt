package com.tryhisar.hisar.ui.item

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Color
import androidx.test.ext.junit.runners.AndroidJUnit4
import java.io.ByteArrayOutputStream
import java.nio.ByteBuffer
import java.nio.ByteOrder
import kotlinx.io.bytestring.ByteString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class IconPhotoConverterTests {
    /** A photo-sized 24-bit BMP, a format the device reads and the backend does not, half of it teal. */
    private fun encodeTestBitmapFile(width: Int, height: Int): ByteString {
        val rowSize = (width * 3 + 3) / 4 * 4
        val buffer = ByteBuffer.allocate(54 + rowSize * height).order(ByteOrder.LITTLE_ENDIAN)
        buffer.put('B'.code.toByte()).put('M'.code.toByte()).putInt(buffer.capacity()).putInt(0).putInt(54)
        buffer.putInt(40).putInt(width).putInt(height).putShort(1).putShort(24).putInt(0).putInt(rowSize * height).putInt(2835).putInt(2835).putInt(0).putInt(0)
        val row = ByteArray(rowSize)
        for (x in 0 until width / 2) {
            row[x * 3] = 0xD0.toByte()
            row[x * 3 + 1] = 0xB0.toByte()
        }
        repeat(height) { buffer.put(row) }
        return ByteString(buffer.array())
    }

    private fun decodeSize(data: ByteString): Pair<Int, Int> {
        val bytes = data.toByteArray()
        val bitmap = checkNotNull(BitmapFactory.decodeByteArray(bytes, 0, bytes.size)) { "not an image" }
        return bitmap.width to bitmap.height
    }

    @Test
    fun testPhotosInFormatsTheBackendDoesNotReadBecomeJPEGOrPNG() {
        val converted = checkNotNull(IconPhotoConverter.convert(encodeTestBitmapFile(2016, 1512))) { "the BMP was not converted" }
        assertEquals("an opaque photo becomes a JPEG", listOf(0xFF, 0xD8, 0xFF), (0 until 3).map { converted[it].toInt() and 0xFF })
        assertEquals(512 to 384, decodeSize(converted))

        val transparent = Bitmap.createBitmap(64, 64, Bitmap.Config.ARGB_8888).apply { eraseColor(Color.TRANSPARENT) }
        val pngOutput = ByteArrayOutputStream().also { transparent.compress(Bitmap.CompressFormat.PNG, 100, it) }
        val png = checkNotNull(IconPhotoConverter.convert(ByteString(pngOutput.toByteArray()))) { "the PNG was not converted" }
        assertEquals("transparency keeps a PNG", listOf(0x89, 0x50, 0x4E, 0x47), (0 until 4).map { png[it].toInt() and 0xFF })
        assertEquals("small images are not scaled up", 64 to 64, decodeSize(png))

        assertNull(IconPhotoConverter.convert(ByteString("not a photo".encodeToByteArray())))
    }
}
