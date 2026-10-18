package com.tryhisar.hisar.ui.settings

import android.text.format.DateUtils
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.outlined.DesktopWindows
import androidx.compose.material.icons.outlined.Extension
import androidx.compose.material.icons.outlined.Laptop
import androidx.compose.material.icons.outlined.Smartphone
import androidx.compose.material.icons.outlined.Terminal
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.SwipeToDismissBox
import androidx.compose.material3.SwipeToDismissBoxValue
import androidx.compose.material3.Text
import androidx.compose.material3.rememberSwipeToDismissBoxState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.Device
import com.tryhisar.hisar.model.DeviceIcon
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.LabeledRow
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import kotlinx.coroutines.launch

private val DeviceIcon.image: ImageVector
    get() = when (this) {
        DeviceIcon.PHONE -> Icons.Outlined.Smartphone
        DeviceIcon.LAPTOP -> Icons.Outlined.Laptop
        DeviceIcon.DESKTOP -> Icons.Outlined.DesktopWindows
        DeviceIcon.TERMINAL -> Icons.Outlined.Terminal
        DeviceIcon.EXTENSION -> Icons.Outlined.Extension
    }

private const val REVOKE_MESSAGE = "It loses access to your vaults at once and needs your Secret Key and master password to sign in again."

private fun formatDateTime(date: Instant): String =
    DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT).withZone(ZoneId.systemDefault()).format(date)

/** Settings → Devices: this device, the others with when they were last seen, and signing one out. */
@Composable
fun DevicesScreen(model: AppModel, onBack: () -> Unit) {
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var allDevice by remember { mutableStateOf<List<Device>?>(null) }
    var deviceToRevoke by remember { mutableStateOf<Device?>(null) }
    var openedDevice by remember { mutableStateOf<Device?>(null) }

    suspend fun load() {
        model.errorMessage.value = null
        allDevice = model.settings.loadAllDevice() ?: allDevice ?: emptyList()
    }
    LaunchedEffect(Unit) { load() }

    val opened = openedDevice
    if (opened != null) {
        BackHandler { openedDevice = null }
        DeviceScreen(model, opened, onBack = { openedDevice = null }, finishRevoke = {
            openedDevice = null
            scope.launch { load() }
        })
        return
    }

    SettingsScreen("Devices", onBack) {
        errorMessage?.let { FormSection { ErrorBanner(it) } }
        val shownDevices = allDevice
        if (shownDevices == null) {
            FormSection {
                FormRow {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(14.dp)) {
                        CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp, color = HisarTheme.colors.secondaryLabel)
                        Text("Loading devices…", style = HisarType.body, color = HisarTheme.colors.secondaryLabel)
                    }
                }
            }
        } else {
            val allCurrentDevice = shownDevices.filter { it.isCurrent }
            val allOtherDevice = shownDevices.filter { !it.isCurrent }
            if (allCurrentDevice.isNotEmpty()) {
                FormSection(header = "This device") {
                    allCurrentDevice.forEach { DeviceRow(it, onClick = null) }
                }
            }
            FormSection(
                header = "Other devices",
                footer = {
                    Text(
                        if (allOtherDevice.isEmpty()) {
                            "No other device is signed in to your account."
                        } else {
                            "Sign out a device you no longer use or don't recognise. It needs your Secret Key and master password to sign in again."
                        },
                    )
                },
            ) {
                // The swipe's red background would square the card's corners, so the rows clip to them.
                Column(Modifier.clip(RoundedCornerShape(26.dp))) {
                    allOtherDevice.forEachIndexed { index, device ->
                        if (index > 0) RowDivider()
                        SwipeToSignOut(onSwipe = { deviceToRevoke = device }) {
                            DeviceRow(device, onClick = { openedDevice = device })
                        }
                    }
                }
            }
        }
    }

    deviceToRevoke?.let { device ->
        ConfirmationDialog(
            title = "Sign out ${device.name.ifEmpty { "this device" }}?",
            message = REVOKE_MESSAGE,
            confirmLabel = "Sign Out Device",
            confirmTestTag = "confirm-revoke-device",
            onConfirm = { scope.launch { if (model.settings.revokeDevice(device)) load() } },
            onDismiss = { deviceToRevoke = null },
        )
    }
}

/** A device row that reveals a red "Sign Out" when swiped to the start, as the iPhone's swipe action. */
@Composable
private fun SwipeToSignOut(onSwipe: () -> Unit, content: @Composable () -> Unit) {
    val state = rememberSwipeToDismissBoxState()
    LaunchedEffect(state.currentValue) {
        if (state.currentValue == SwipeToDismissBoxValue.EndToStart) {
            onSwipe()
            state.reset()
        }
    }
    SwipeToDismissBox(
        state = state,
        enableDismissFromStartToEnd = false,
        backgroundContent = {
            Box(
                Modifier
                    .fillMaxSize()
                    .background(HisarTheme.colors.destructive)
                    .padding(horizontal = 20.dp),
                contentAlignment = Alignment.CenterEnd,
            ) {
                Text("Sign Out", style = HisarType.body, color = Color.White)
            }
        },
    ) {
        Box(Modifier.background(HisarTheme.colors.cellBackground)) { content() }
    }
}

@Composable
private fun DeviceRow(device: Device, onClick: (() -> Unit)?) {
    val lastSeenDate = device.lastSeenDate
    val subtitle = when {
        device.isCurrent -> "${device.platformDescription} · Active now"
        lastSeenDate != null -> {
            val lastSeen = DateUtils.getRelativeTimeSpanString(lastSeenDate.toEpochMilli(), System.currentTimeMillis(), DateUtils.MINUTE_IN_MILLIS)
            "${device.platformDescription} · Last seen ${lastSeen.toString().replaceFirstChar { it.lowercase() }}"
        }
        else -> device.platformDescription
    }
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 60.dp)
            .then(if (onClick != null) Modifier.clickable(role = Role.Button, onClick = onClick) else Modifier)
            .testTag("device-${device.name}")
            .padding(horizontal = 16.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RowIcon(device.icon.image)
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(device.name.ifEmpty { device.platformDescription }, style = HisarType.body, color = HisarTheme.colors.label)
            Text(subtitle, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
        }
        if (onClick != null) {
            Icon(
                Icons.AutoMirrored.Filled.KeyboardArrowRight,
                contentDescription = null,
                tint = HisarTheme.colors.secondaryLabel.copy(alpha = 0.6f),
                modifier = Modifier.size(22.dp),
            )
        }
    }
}

/** One other device: what it is, when it was last seen and signed in, and signing it out. */
@Composable
private fun DeviceScreen(model: AppModel, device: Device, onBack: () -> Unit, finishRevoke: () -> Unit) {
    val scope = rememberCoroutineScope()
    var isRevokeConfirmationShown by remember { mutableStateOf(false) }
    SettingsScreen(device.name, onBack, isTitleInline = true) {
        FormSection {
            LabeledRow("Name", device.name)
            RowDivider()
            LabeledRow("Platform", device.platformDescription)
            device.lastSeenDate?.let {
                RowDivider()
                LabeledRow("Last seen", formatDateTime(it))
            }
            device.createdDate?.let {
                RowDivider()
                LabeledRow("Signed in", formatDateTime(it))
            }
        }
        FormSection(footer = { Text("Signing out ends the device's session on the server, so it can no longer sync your vaults or answer for your account.") }) {
            DestructiveRow("Sign Out Device", { isRevokeConfirmationShown = true }, testTag = "revoke-device")
        }
    }
    if (isRevokeConfirmationShown) {
        ConfirmationDialog(
            title = "Sign out ${device.name}?",
            message = REVOKE_MESSAGE,
            confirmLabel = "Sign Out Device",
            confirmTestTag = "confirm-revoke-device",
            onConfirm = { scope.launch { if (model.settings.revokeDevice(device)) finishRevoke() } },
            onDismiss = { isRevokeConfirmationShown = false },
        )
    }
}
