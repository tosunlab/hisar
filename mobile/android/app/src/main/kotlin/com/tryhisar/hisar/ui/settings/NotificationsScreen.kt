package com.tryhisar.hisar.ui.settings

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.os.Build
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Group
import androidx.compose.material.icons.outlined.LaptopMac
import androidx.compose.material.icons.outlined.Notifications
import androidx.compose.material.icons.outlined.Security
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.compose.currentStateAsState
import com.tryhisar.hisar.MainActivity
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.PushKind
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.RowDivider
import kotlinx.coroutines.launch

private data class KindDescription(val title: String, val icon: ImageVector)

private fun describe(kind: PushKind): KindDescription = when (kind) {
    PushKind.APPROVAL -> KindDescription("Approval requests", Icons.Outlined.LaptopMac)
    PushKind.SECURITY -> KindDescription("Security alerts", Icons.Outlined.Security)
    PushKind.SHARING -> KindDescription("Sharing", Icons.Outlined.Group)
}

/** Settings → Notifications: the iPhone app's NotificationSettingsView, with Android 13's permission prompt. */
@Composable
fun NotificationsScreen(model: AppModel, onBack: () -> Unit) {
    val notifications = model.notifications
    val settings by notifications.settings.collectAsStateWithLifecycle()
    val isPermissionGranted by notifications.isPermissionGranted.collectAsStateWithLifecycle()
    val errorMessage by notifications.errorMessage.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val lifecycleState by LocalLifecycleOwner.current.lifecycle.currentStateAsState()
    // Coming back from the system's settings may have changed the permission.
    LaunchedEffect(lifecycleState) {
        if (lifecycleState.isAtLeast(Lifecycle.State.RESUMED)) notifications.refresh()
    }
    val permissionLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { isGranted ->
        if (isGranted) (context as? MainActivity)?.fetchPushToken()
        scope.launch { notifications.refresh() }
    }
    val hasAskedBefore = Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
        (context as? Activity)?.shouldShowRequestPermissionRationale(Manifest.permission.POST_NOTIFICATIONS) == true

    SettingsScreen("Notifications", onBack) {
        FormSection(
            footer = {
                Text(
                    if (isPermissionGranted && !settings.isServerSending) {
                        "Your Hisar server does not send notifications. Its admin can turn them on in the dashboard's settings."
                    } else {
                        "A notification only says what kind of thing happened, never what or who it is about. Open and unlock Hisar to see the details. Hisar's push relay, Google and your Hisar server see when a notification is sent, but not what it is about."
                    },
                )
            },
        ) {
            when {
                isPermissionGranted -> PushKind.entries.forEachIndexed { index, kind ->
                    if (index > 0) RowDivider()
                    val description = describe(kind)
                    ToggleRow(
                        description.title,
                        isOn = settings.isEnabled(kind),
                        onChange = { isEnabled -> scope.launch { notifications.setEnabled(kind, isEnabled) } },
                        testTag = "notifications-${kind.value}-toggle",
                        icon = description.icon,
                        enabled = settings.isServerSending,
                    )
                }
                Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU && !hasAskedBefore -> FormButtonRow(
                    "Turn on notifications",
                    onClick = { permissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS) },
                    testTag = "notifications-turn-on",
                    icon = Icons.Outlined.Notifications,
                )
                else -> FormButtonRow(
                    "Allow notifications in Android settings",
                    onClick = {
                        context.startActivity(Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, context.packageName))
                    },
                    testTag = "notifications-open-settings",
                    icon = Icons.Outlined.Settings,
                )
            }
        }
        (errorMessage ?: settings.errorMessage)?.let { message ->
            FormSection { ErrorBanner(message) }
        }
    }
}
