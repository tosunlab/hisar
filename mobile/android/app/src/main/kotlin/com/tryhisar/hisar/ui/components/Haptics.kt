package com.tryhisar.hisar.ui.components

import android.os.Build
import android.view.HapticFeedbackConstants
import android.view.View

/** The success tap the iPhone app plays on a copy: CONFIRM from API 30, the key tap before it. */
fun View.performConfirmHaptic() {
    performHapticFeedback(if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) HapticFeedbackConstants.CONFIRM else HapticFeedbackConstants.VIRTUAL_KEY)
}
