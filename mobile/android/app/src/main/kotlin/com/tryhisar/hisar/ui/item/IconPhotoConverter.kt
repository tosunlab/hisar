package com.tryhisar.hisar.ui.item

import android.graphics.Bitmap
import android.graphics.ImageDecoder
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.nio.ByteBuffer
import kotlin.math.max
import kotlin.math.roundToInt
import kotlinx.io.bytestring.ByteString

/**
 * Turns a photo as the library keeps it (HEIF from the camera, BMP, PNG…) into what the backend reads: decoded with its
 * orientation applied, scaled down to [maxSide], and encoded as PNG when it has transparency, else as JPEG. Null for data
 * that is no image.
 */
object IconPhotoConverter {
    fun convert(photo: ByteString, maxSide: Int = 512): ByteString? {
        val bitmap = try {
            ImageDecoder.decodeBitmap(ImageDecoder.createSource(ByteBuffer.wrap(photo.toByteArray()))) { decoder, info, _ ->
                decoder.allocator = ImageDecoder.ALLOCATOR_SOFTWARE
                val scale = minOf(1.0, maxSide.toDouble() / max(info.size.width, info.size.height))
                decoder.setTargetSize(max(1, (info.size.width * scale).roundToInt()), max(1, (info.size.height * scale).roundToInt()))
            }
        } catch (_: IOException) {
            return null
        } catch (_: IllegalArgumentException) {
            return null
        }
        val output = ByteArrayOutputStream()
        val format = if (bitmap.hasAlpha()) Bitmap.CompressFormat.PNG else Bitmap.CompressFormat.JPEG
        bitmap.compress(format, 90, output)
        bitmap.recycle()
        return ByteString(output.toByteArray())
    }
}
