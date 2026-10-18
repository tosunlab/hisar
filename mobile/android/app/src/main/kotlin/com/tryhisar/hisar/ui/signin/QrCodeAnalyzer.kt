package com.tryhisar.hisar.ui.signin

import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.ReaderException
import com.google.zxing.common.HybridBinarizer
import com.google.zxing.qrcode.QRCodeReader
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Reads QR codes from the camera's frames with ZXing, and hands the text of the first one it reads to [onScanned], once,
 * on the analysis thread. It reads only the frame's brightness, the Y plane every camera frame starts with.
 */
class QrCodeAnalyzer(private val onScanned: (String) -> Unit) : ImageAnalysis.Analyzer {
    private val reader = QRCodeReader()
    private val hints = mapOf(DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE), DecodeHintType.TRY_HARDER to true)
    private val hasScanned = AtomicBoolean(false)
    private var luminance = ByteArray(0)

    override fun analyze(image: ImageProxy) {
        image.use {
            if (hasScanned.get()) return
            val text = read(it) ?: return
            if (hasScanned.compareAndSet(false, true)) onScanned(text)
        }
    }

    private fun read(image: ImageProxy): String? {
        val plane = image.planes[0]
        val buffer = plane.buffer.duplicate()
        val size = buffer.remaining()
        if (luminance.size < size) luminance = ByteArray(size)
        buffer.get(luminance, 0, size)
        // Rows are rowStride bytes apart; the bytes past the frame's width on each row are padding the source skips.
        val source = PlanarYUVLuminanceSource(luminance, plane.rowStride, image.height, 0, 0, image.width, image.height, false)
        return try {
            reader.decode(BinaryBitmap(HybridBinarizer(source)), hints).text
        } catch (_: ReaderException) {
            null
        } finally {
            reader.reset()
        }
    }
}
