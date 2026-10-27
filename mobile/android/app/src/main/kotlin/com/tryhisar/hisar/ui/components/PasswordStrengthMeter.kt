package com.tryhisar.hisar.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.model.PasswordStrength
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType

private val segmentShape = RoundedCornerShape(2.dp)

/**
 * The iPhone app's PasswordStrengthMeter: four segments and the label (Weak, Fair, Good, Strong) of core/item's rating of
 * a password. It shows nothing for an empty password.
 */
@Composable
fun PasswordStrengthMeter(strength: PasswordStrength, modifier: Modifier = Modifier) {
    if (strength.level.isEmpty()) return
    val colors = HisarTheme.colors
    val filledSegmentCount = when (strength.level) {
        "weak" -> 1
        "fair" -> 2
        "good" -> 3
        "strong" -> 4
        else -> 0
    }
    // iOS's system green, which the iPhone app's meter fills Good and Strong with.
    val fillColor = when (strength.level) {
        "weak" -> colors.destructive
        "fair" -> colors.warning
        else -> if (colors.isDark) Color(0xFF30D158) else Color(0xFF34C759)
    }
    Row(
        modifier
            .testTag("password-strength-meter")
            .semantics(mergeDescendants = true) { contentDescription = "Password strength" },
        horizontalArrangement = Arrangement.spacedBy(4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        repeat(4) { segment ->
            Box(Modifier.size(width = 28.dp, height = 4.dp).background(if (segment < filledSegmentCount) fillColor else colors.fill, segmentShape))
        }
        Text(strength.label, style = HisarType.footnote, color = colors.secondaryLabel, modifier = Modifier.padding(start = 6.dp))
    }
}
