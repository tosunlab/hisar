package com.tryhisar.hisar.ui.item

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Terminal
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.text
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.OneTimeCode
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import java.time.Duration
import java.time.Instant
import kotlinx.coroutines.delay

/**
 * A login's current one-time code, split in two halves, with a ring counting down the seconds it has left. It asks for the
 * next code exactly when this one expires.
 */
@Composable
internal fun OneTimeCodeRow(model: AppModel, vaultID: String, itemID: String, copy: () -> Unit, copyReference: () -> Unit) {
    var oneTimeCode by remember { mutableStateOf<OneTimeCode?>(null) }
    var isUnavailable by remember { mutableStateOf(false) }
    var now by remember { mutableStateOf(Instant.now()) }
    LaunchedEffect(vaultID, itemID) {
        while (true) {
            val code = model.item.loadOneTimeCode(vaultID, itemID)
            oneTimeCode = code
            isUnavailable = code == null
            if (code == null) return@LaunchedEffect
            val untilExpiry = code.expiryDate?.let { Duration.between(Instant.now(), it).toMillis() } ?: 0
            delay(maxOf(1_000, untilExpiry))
        }
    }
    LaunchedEffect(Unit) {
        while (true) {
            now = Instant.now()
            delay(1_000 - now.toEpochMilli() % 1_000)
        }
    }
    val allAction = listOf(
        MenuAction("Copy one-time code", Icons.Outlined.ContentCopy, copy),
        MenuAction("Copy secret reference", Icons.Outlined.Terminal, copyReference),
    )
    MenuRow(allAction, onClick = copy.takeIf { oneTimeCode != null }, onClickLabel = "Copy one-time code") {
        Row(
            Modifier.padding(horizontal = 16.dp, vertical = 11.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                Text("One-time code", style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
                val code = oneTimeCode
                when {
                    code != null -> RollingCode(code.formattedCode)
                    isUnavailable -> Text("The one-time code could not be read.", style = HisarType.body, color = HisarTheme.colors.secondaryLabel)
                    else -> CircularProgressIndicator(Modifier.size(20.dp), color = HisarTheme.colors.secondaryLabel, strokeWidth = 2.dp)
                }
            }
            oneTimeCode?.let { code ->
                CountdownRing(secondsLeft = code.countSecondsLeft(now), period = code.period)
                FieldIconButton(Icons.Outlined.ContentCopy, "Copy one-time code", "copy-one-time-code", copy)
            }
        }
    }
}

/** The code, each changed digit rolling up into place, as `.contentTransition(.numericText())` animates it. */
@Composable
private fun RollingCode(code: String) {
    Row(
        Modifier
            .clipToBounds()
            .testTag("one-time-code-value")
            .clearAndSetSemantics { text = AnnotatedString(code) },
    ) {
        for (character in code) {
            AnimatedContent(
                targetState = character,
                transitionSpec = { (slideInVertically { it } + fadeIn()) togetherWith (slideOutVertically { -it } + fadeOut()) },
                label = "digit",
            ) { shown ->
                Text(
                    shown.toString(),
                    style = HisarType.title2.copy(fontWeight = FontWeight.SemiBold, fontFeatureSettings = "tnum"),
                    color = HisarTheme.colors.label,
                )
            }
        }
    }
}

@Composable
private fun CountdownRing(secondsLeft: Int, period: Int) {
    val remainingFraction by animateFloatAsState(
        if (period > 0) secondsLeft.toFloat() / period else 0f,
        animationSpec = tween(900, easing = LinearEasing),
        label = "remaining",
    )
    val trackColor = HisarTheme.colors.fill
    val ringColor = if (secondsLeft <= 5) HisarTheme.colors.warning else HisarTheme.colors.tint
    Box(
        Modifier
            .size(30.dp)
            .testTag("one-time-code-seconds")
            .clearAndSetSemantics { contentDescription = "$secondsLeft seconds left" },
        contentAlignment = Alignment.Center,
    ) {
        Canvas(Modifier.fillMaxSize()) {
            val strokeWidth = 3.dp.toPx()
            drawCircle(trackColor, radius = (size.minDimension - strokeWidth) / 2, style = Stroke(strokeWidth))
            drawArc(
                ringColor,
                startAngle = -90f,
                sweepAngle = 360f * remainingFraction,
                useCenter = false,
                topLeft = Offset(strokeWidth / 2, strokeWidth / 2),
                size = Size(size.width - strokeWidth, size.height - strokeWidth),
                style = Stroke(strokeWidth, cap = StrokeCap.Round),
            )
        }
        Text(
            "$secondsLeft",
            style = HisarType.footnote.copy(fontSize = 11.sp, fontWeight = FontWeight.SemiBold, fontFeatureSettings = "tnum", fontFamily = FontFamily.Default),
            color = HisarTheme.colors.secondaryLabel,
        )
    }
}
