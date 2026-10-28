package com.tryhisar.hisar.ui.root

import android.content.ActivityNotFoundException
import android.content.Intent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.SystemUpdate
import androidx.compose.material.icons.outlined.Warning
import androidx.compose.material3.CircularProgressIndicator
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
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.model.UpdateRequirement
import com.tryhisar.hisar.platform.ClipboardAdapter
import com.tryhisar.hisar.ui.components.ContentUnavailable
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import androidx.core.net.toUri

@Composable
fun StartingScreen() {
    Box(Modifier.fillMaxSize().background(HisarTheme.colors.groupedBackground), contentAlignment = Alignment.Center) {
        CircularProgressIndicator(color = HisarTheme.colors.secondaryLabel)
    }
}

@Composable
fun StartupFailureScreen(message: String) {
    Box(Modifier.fillMaxSize().background(HisarTheme.colors.groupedBackground)) {
        ContentUnavailable("Hisar could not start", Icons.Outlined.Warning, description = {
            Text(message, style = HisarType.subheadline, color = HisarTheme.colors.secondaryLabel, textAlign = TextAlign.Center, modifier = Modifier.testTag("error-message"))
        })
    }
}

/** The server needs a newer Hisar: nothing else opens until it accepts this version. */
@Composable
fun UpdateRequiredScreen(requirement: UpdateRequirement, isChecking: Boolean, checkAgain: () -> Unit) {
    val context = LocalContext.current
    var isUpdatePageUnavailable by remember(requirement.updateURL) { mutableStateOf(false) }
    val clipboard = remember(context) { ClipboardAdapter(context) }
    Box(Modifier.fillMaxSize().background(HisarTheme.colors.groupedBackground).testTag("update-required")) {
        ContentUnavailable(
            "Update required",
            Icons.Outlined.SystemUpdate,
            description = {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                    Text(
                        "This server needs Hisar ${requirement.minimumVersion} or later. This device has ${requirement.currentVersionDescription}.",
                        style = HisarType.subheadline,
                        color = HisarTheme.colors.secondaryLabel,
                        textAlign = TextAlign.Center,
                        modifier = Modifier.testTag("update-required-message"),
                    )
                    if (isUpdatePageUnavailable) {
                        Text(
                            "No app on this phone can open the update page. Copy the link and open it in a browser: ${requirement.updateURL}",
                            style = HisarType.subheadline,
                            color = HisarTheme.colors.secondaryLabel,
                            textAlign = TextAlign.Center,
                            modifier = Modifier.testTag("update-required-open-failed"),
                        )
                    }
                }
            },
            actions = {
                Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    ProminentButton("Install the latest Hisar", {
                        try {
                            context.startActivity(Intent(Intent.ACTION_VIEW, requirement.updateURL.toUri()))
                        } catch (_: ActivityNotFoundException) {
                            isUpdatePageUnavailable = true
                        }
                    }, testTag = "update-required-install")
                    if (isUpdatePageUnavailable) {
                        PlainButton("Copy link", { clipboard.setPlainText(requirement.updateURL) }, testTag = "update-required-copy-link")
                    }
                    if (isChecking) {
                        CircularProgressIndicator(color = HisarTheme.colors.secondaryLabel)
                    } else {
                        PlainButton("Check again", checkAgain, testTag = "update-required-check-again")
                    }
                }
            },
        )
    }
}
