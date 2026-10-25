package com.tryhisar.hisar.ui.settings

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Download
import androidx.compose.material.icons.outlined.Group
import androidx.compose.material.icons.outlined.LaptopMac
import androidx.compose.material.icons.outlined.Link
import androidx.compose.material.icons.outlined.Notifications
import androidx.compose.material.icons.outlined.Security
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
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

/** UnifiedPush's own list of distributors, ntfy among them. */
private const val DISTRIBUTORS_URL = "https://unifiedpush.org/users/distributors/"

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
    val isDistributorInstalled by notifications.isDistributorInstalled.collectAsStateWithLifecycle()
    val errorMessage by notifications.errorMessage.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val lifecycleState by LocalLifecycleOwner.current.lifecycle.currentStateAsState()
    // Coming back from the system's settings may have changed the permission.
    LaunchedEffect(lifecycleState) {
        if (lifecycleState.isAtLeast(Lifecycle.State.RESUMED)) notifications.refresh()
    }
    val permissionLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {
        scope.launch { notifications.refresh() }
    }
    // A launch without permission started no push, so allowing notifications here or in Android's settings starts it.
    var wasPermissionGranted by remember { mutableStateOf(isPermissionGranted) }
    LaunchedEffect(isPermissionGranted) {
        if (isPermissionGranted && !wasPermissionGranted) (context as? MainActivity)?.startPush()
        wasPermissionGranted = isPermissionGranted
    }
    val hasAskedBefore = Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
        (context as? Activity)?.shouldShowRequestPermissionRationale(Manifest.permission.POST_NOTIFICATIONS) == true

    SettingsScreen("Notifications", onBack) {
        FormSection(
            footer = {
                Text(
                    when {
                        isPermissionGranted && !isDistributorInstalled ->
                            "Hisar gets notifications through a UnifiedPush app, such as ntfy, which keeps one connection open for all your apps. Install one, then come back here."
                        isPermissionGranted && !settings.isServerSending ->
                            "Your Hisar server does not send notifications. Its admin can turn them on in the dashboard's settings."
                        else ->
                            "A notification only says what kind of thing happened, never what or who it is about. Open and unlock Hisar to see the details. Your Hisar server and your UnifiedPush app see when a notification is sent, but not what it is about."
                    },
                )
            },
        ) {
            when {
                isPermissionGranted && !isDistributorInstalled -> FormButtonRow(
                    "Get a UnifiedPush app",
                    onClick = { context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(DISTRIBUTORS_URL))) },
                    testTag = "notifications-get-distributor",
                    icon = Icons.Outlined.Download,
                )
                isPermissionGranted -> {
                    PushKind.entries.forEachIndexed { index, kind ->
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
                    // The UnifiedPush app has not given an endpoint yet, or the user has several and has not chosen one.
                    if (settings.isServerSending && !settings.isAvailable) {
                        RowDivider()
                        FormButtonRow(
                            "Connect to your UnifiedPush app",
                            onClick = { (context as? MainActivity)?.startPush() },
                            testTag = "notifications-connect-distributor",
                            icon = Icons.Outlined.Link,
                        )
                    }
                }
                else -> {
                    // Android stops showing its prompt once the user denied it for good, which looks the same here as never
                    // asked, so its settings stay one tap away.
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU && !hasAskedBefore) {
                        FormButtonRow(
                            "Turn on notifications",
                            onClick = { permissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS) },
                            testTag = "notifications-turn-on",
                            icon = Icons.Outlined.Notifications,
                        )
                        RowDivider()
                    }
                    FormButtonRow(
                        "Allow notifications in Android settings",
                        onClick = {
                            context.startActivity(Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, context.packageName))
                        },
                        testTag = "notifications-open-settings",
                        icon = Icons.Outlined.Settings,
                    )
                }
            }
        }
        (errorMessage ?: settings.errorMessage)?.let { message ->
            FormSection { ErrorBanner(message) }
        }
    }
}
