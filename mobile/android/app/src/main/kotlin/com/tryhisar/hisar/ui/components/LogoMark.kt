package com.tryhisar.hisar.ui.components

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.ui.theme.HisarTheme

private val markBackground = Color(0xFFF2F3F5)
private val markForeground = Color(0xFF0B0C0E)

/** The Hisar mark: the tower of assets/logo/hisar-mark.svg on its tile, as the iPhone app's LogoMark draws it. */
@Composable
fun LogoMark(modifier: Modifier = Modifier, size: Dp = 56.dp) {
    val shape = RoundedCornerShape(size / 4)
    Box(
        modifier
            .size(size)
            .background(markBackground, shape)
            .border(1.dp, HisarTheme.colors.separator, shape),
    ) {
        Canvas(Modifier.fillMaxSize()) {
            val scale = minOf(this.size.width, this.size.height) / 32f
            fun point(x: Float, y: Float) = Offset(x * scale, y * scale)
            val tower = Path().apply {
                moveTo(point(8f, 26f))
                for ((x, y) in listOf(8f to 12f, 11f to 12f, 11f to 9f, 13.5f to 9f, 13.5f to 12f, 18.5f to 12f, 18.5f to 9f, 21f to 9f, 21f to 12f, 24f to 12f, 24f to 26f, 18f to 26f, 18f to 21f)) {
                    lineTo(point(x, y))
                }
                // The gate's arch, from its right side over the top to its left side.
                arcTo(Rect(point(14f, 19f), point(18f, 23f)), startAngleDegrees = 0f, sweepAngleDegrees = -180f, forceMoveTo = false)
                lineTo(point(14f, 26f))
                close()
            }
            drawPath(tower, markForeground)
        }
    }
}

private fun Path.moveTo(offset: Offset) = moveTo(offset.x, offset.y)

private fun Path.lineTo(offset: Offset) = lineTo(offset.x, offset.y)
