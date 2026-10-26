package com.tryhisar.hisar.autofill

import android.annotation.SuppressLint
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import android.view.autofill.AutofillManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material.icons.outlined.OpenInNew
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType

/** Whether Hisar is the device's autofill service, for the "AutoFill" row of Settings. */
fun isAutoFillEnabled(context: Context): Boolean =
    context.getSystemService(AutofillManager::class.java)?.hasEnabledAutofillServices() == true

/**
 * The sections of Settings → AutoFill, the iPhone app's AutoFillSettingsView: whether Hisar fills, the system's choice
 * of service, and Chrome's own switch, which Chrome needs before it asks any service but Google's.
 */
@Composable
fun ColumnScope.AutoFillSettingsSections() {
    val context = LocalContext.current
    var isEnabled by remember { mutableStateOf(isAutoFillEnabled(context)) }
    val chromeSettingsIntent = remember { findChromeAutofillSettings(context) }
    val turnOn = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) {
        isEnabled = isAutoFillEnabled(context)
    }
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { isEnabled = isAutoFillEnabled(context) }

    FormSection(footer = { Text(systemSettingsFooter(isEnabled)) }) {
        if (isEnabled) {
            FormRow(Modifier.testTag("autofill-is-on")) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Outlined.CheckCircle, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.size(24.dp))
                    Spacer(Modifier.size(14.dp))
                    Text("Hisar fills passwords and one-time codes in apps and browsers", style = HisarType.body, color = HisarTheme.colors.label)
                }
            }
        } else {
            FormButtonRow(
                "Turn on Hisar AutoFill",
                onClick = {
                    val intent = Intent(Settings.ACTION_REQUEST_SET_AUTOFILL_SERVICE, Uri.parse("package:${context.packageName}"))
                    try {
                        turnOn.launch(intent)
                    } catch (_: ActivityNotFoundException) {
                        openSystemSettings(context)
                    }
                },
                testTag = "turn-on-autofill",
                icon = Icons.Outlined.Key,
                color = HisarTheme.colors.tint,
            )
        }
    }
    val isCredentialProviderSettingsAvailable = Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE
    if (isCredentialProviderSettingsAvailable || isEnabled) {
        FormSection {
            FormButtonRow(
                if (isCredentialProviderSettingsAvailable) "Open Passwords, passkeys & accounts" else "Open Settings",
                onClick = { openSystemSettings(context) },
                testTag = "open-autofill-settings",
                icon = Icons.Outlined.Settings,
                color = HisarTheme.colors.tint,
            )
        }
    }
    if (chromeSettingsIntent != null) {
        FormSection(
            header = "Chrome",
            footer = { Text("Chrome fills with Google until you choose \"Autofill using another service\" under Autofill services and restart Chrome.") },
        ) {
            FormButtonRow(
                "Open Chrome's autofill settings",
                onClick = { context.startActivity(chromeSettingsIntent) },
                testTag = "open-chrome-autofill-settings",
                icon = Icons.Outlined.OpenInNew,
                color = HisarTheme.colors.tint,
            )
        }
    }
}

private fun systemSettingsFooter(isEnabled: Boolean): String {
    val path = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
        "Passwords, passkeys & accounts"
    } else {
        "System, Languages & input, Autofill service"
    }
    return if (isEnabled) {
        "Change the service in the Settings app: $path."
    } else {
        "You can also turn it on in the Settings app: $path."
    }
}

// Settings answers ACTION_CREDENTIAL_PROVIDER only with a package: data URI; without one the intent finds no activity.
private fun openSystemSettings(context: Context) {
    val intent = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
        Intent(Settings.ACTION_CREDENTIAL_PROVIDER, Uri.parse("package:${context.packageName}"))
    } else {
        Intent(Settings.ACTION_SETTINGS)
    }
    context.startActivity(intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
}

// Chromium documents this intent for autofill services; each installed Chrome channel answers it, and a chooser picks one.
private fun findChromeAutofillSettings(context: Context): Intent? {
    val intent = Intent(Intent.ACTION_APPLICATION_PREFERENCES)
        .addCategory(Intent.CATEGORY_DEFAULT)
        .addCategory(Intent.CATEGORY_APP_BROWSER)
        .addCategory(Intent.CATEGORY_PREFERENCE)
    if (context.packageManager.queryIntentActivities(intent, 0).isEmpty()) return null
    return Intent.createChooser(intent, "Open Chrome's autofill settings")
}
