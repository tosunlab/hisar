package com.tryhisar.hisar.ui.item

import androidx.compose.foundation.layout.Box
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.PasswordStrength
import com.tryhisar.hisar.ui.components.PasswordStrengthMeter

@Composable
fun ItemPasswordStrengthMeter(model: AppModel, password: String, modifier: Modifier = Modifier) {
    var strength by remember(password) { mutableStateOf<PasswordStrength?>(null) }
    LaunchedEffect(password) {
        if (password.isNotEmpty()) strength = model.ratePassword(password)
    }
    if (password.isNotEmpty()) {
        strength?.let {
            Box(modifier.testTag("item-password-strength")) { PasswordStrengthMeter(it) }
        }
    }
}
