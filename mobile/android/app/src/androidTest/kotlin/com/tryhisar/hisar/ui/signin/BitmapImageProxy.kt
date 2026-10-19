package com.tryhisar.hisar.ui.signin

import android.graphics.Bitmap
import android.graphics.Color
import android.graphics.ImageFormat
import android.graphics.Rect
import android.media.Image
import androidx.camera.core.ImageInfo
import androidx.camera.core.ImageProxy
import java.nio.ByteBuffer

/**
 * A camera frame showing [bitmap], as CameraX hands one to an ImageAnalysis analyzer: YUV_420_888 whose brightness plane
 * has rows padded past the frame's width, as camera buffers often are. Only the brightness plane carries the picture.
 */
class BitmapImageProxy(bitmap: Bitmap, private val rowPadding: Int = 32) : ImageProxy {
    private val frameWidth = bitmap.width
    private val frameHeight = bitmap.height
    private val paddedRowStride = frameWidth + rowPadding
    private val luminance: ByteBuffer = ByteBuffer.allocateDirect(paddedRowStride * (frameHeight - 1) + frameWidth).apply {
        val row = IntArray(frameWidth)
        for (y in 0 until frameHeight) {
            bitmap.getPixels(row, 0, frameWidth, 0, y, frameWidth, 1)
            position(y * paddedRowStride)
            for (pixel in row) put(((Color.red(pixel) * 299 + Color.green(pixel) * 587 + Color.blue(pixel) * 114) / 1000).toByte())
            if (y < frameHeight - 1) repeat(rowPadding) { put(0x55.toByte()) }
        }
        rewind()
    }
    private val brightnessPlane = object : ImageProxy.PlaneProxy {
        override fun getRowStride(): Int = paddedRowStride

        override fun getPixelStride() = 1

        override fun getBuffer(): ByteBuffer = luminance
    }
    private val colorPlane = object : ImageProxy.PlaneProxy {
        override fun getRowStride(): Int = paddedRowStride

        override fun getPixelStride() = 2

        override fun getBuffer(): ByteBuffer = ByteBuffer.allocateDirect(0)
    }

    var isClosed = false
        private set

    override fun close() {
        isClosed = true
    }

    override fun getCropRect() = Rect(0, 0, frameWidth, frameHeight)

    override fun setCropRect(rect: Rect?) = Unit

    override fun getFormat() = ImageFormat.YUV_420_888

    override fun getHeight() = frameHeight

    override fun getWidth() = frameWidth

    override fun getPlanes(): Array<ImageProxy.PlaneProxy> = arrayOf(brightnessPlane, colorPlane, colorPlane)

    override fun getImageInfo(): ImageInfo = error("the QR code analyzer reads only the planes")

    override fun getImage(): Image? = null
}
