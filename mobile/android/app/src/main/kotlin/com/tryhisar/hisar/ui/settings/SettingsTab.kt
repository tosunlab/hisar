package com.tryhisar.hisar.ui.settings

import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.automirrored.outlined.HelpOutline
import androidx.compose.material.icons.outlined.AutoFixHigh
import androidx.compose.material.icons.outlined.Business
import androidx.compose.material.icons.outlined.Devices
import androidx.compose.material.icons.outlined.Group
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.LaptopMac
import androidx.compose.material.icons.outlined.Notifications
import androidx.compose.material.icons.outlined.Palette
import androidx.compose.material.icons.outlined.Password
import androidx.compose.material.icons.outlined.Security
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.compose.currentStateAsState
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.SettingsDestination
import com.tryhisar.hisar.autofill.AutoFillSettingsSections
import com.tryhisar.hisar.autofill.isAutoFillEnabled
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.NavigationTransition
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.components.TabScaffold
import com.tryhisar.hisar.ui.item.StandalonePasswordGeneratorScreen
import com.tryhisar.hisar.ui.organizations.OrganizationsScreen
import com.tryhisar.hisar.ui.root.TabBarHeight
import com.tryhisar.hisar.ui.sharing.PeopleScreen
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.launch

private val stackSaver = Saver<List<SettingsDestination>, ArrayList<String>>(
    save = { stack -> ArrayList(stack.map { it.name }) },
    restore = { names -> names.map(SettingsDestination::valueOf) },
)

/** The Settings tab: its root list and the screens it pushes, each Back returning one screen. */
@Composable
fun SettingsTab(model: AppModel) {
    var stack by rememberSaveable(stateSaver = stackSaver) { mutableStateOf(emptyList<SettingsDestination>()) }
    val push: (SettingsDestination) -> Unit = { stack = stack + it }
    val pop: () -> Unit = { stack = stack.dropLast(1) }
    val requestedSettings by model.requestedSettings.collectAsStateWithLifecycle()
    LaunchedEffect(requestedSettings) {
        val destination = requestedSettings ?: return@LaunchedEffect
        model.requestedSettings.value = null
        // The new Secret Key is shown only once, so its screen stays until Done.
        if (stack.lastOrNull() != SettingsDestination.REPLACE_SECRET_KEY) stack = listOf(destination)
    }

    NavigationTransition(
        stack,
        onPop = pop,
        isBackEnabled = stack.isNotEmpty() && stack.last() != SettingsDestination.REPLACE_SECRET_KEY,
        contentKey = { it.lastOrNull() },
    ) { shownStack ->
        when (shownStack.lastOrNull()) {
            null -> SettingsRoot(model, push)
            SettingsDestination.ACCOUNT -> AccountScreen(model, push, pop)
            SettingsDestination.SECRET_KEY -> SecretKeyScreen(model, pop)
            SettingsDestination.SECURITY -> SecurityScreen(model, push, pop)
            SettingsDestination.APPEARANCE -> AppearanceScreen(model, pop)
            SettingsDestination.MAC_APPROVALS -> MacApprovalsScreen(model, pop)
            SettingsDestination.NOTIFICATIONS -> NotificationsScreen(model, pop)
            SettingsDestination.DEVICES -> DevicesScreen(model, pop)
            SettingsDestination.HELP -> HelpScreen(model, pop)
            SettingsDestination.ABOUT -> AboutScreen(pop)
            SettingsDestination.CHANGE_MASTER_PASSWORD -> ChangeMasterPasswordScreen(model, pop)
            SettingsDestination.REPLACE_SECRET_KEY -> ReplaceSecretKeyScreen(model, pop)
            SettingsDestination.DELETE_ACCOUNT -> DeleteAccountScreen(model, pop)
            SettingsDestination.EXPORT_DATA -> ExportDataScreen(model, pop)
            SettingsDestination.AUTOFILL -> SettingsScreen("AutoFill", onBack = pop) { AutoFillSettingsSections() }
            SettingsDestination.PASSWORD_GENERATOR -> StandalonePasswordGeneratorScreen(model, pop)
            SettingsDestination.PEOPLE -> PeopleScreen(model, pop)
            SettingsDestination.ORGANIZATIONS -> OrganizationsScreen(model, pop)
        }
    }
}

@Composable
private fun SettingsRoot(model: AppModel, push: (SettingsDestination) -> Unit) {
    val status by model.status.collectAsStateWithLifecycle()
    val theme by model.theme.collectAsStateWithLifecycle()
    val isMacApprovalAllowed by model.isMacApprovalAllowed.collectAsStateWithLifecycle()
    val data by model.unlockedData.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var isSignOutConfirmationShown by remember { mutableStateOf(false) }
    val lifecycleState by LocalLifecycleOwner.current.lifecycle.currentStateAsState()
    var isAutoFillOn by remember { mutableStateOf(isAutoFillEnabled(context)) }
    LaunchedEffect(lifecycleState) {
        if (lifecycleState.isAtLeast(Lifecycle.State.RESUMED)) isAutoFillOn = isAutoFillEnabled(context)
    }
    LaunchedEffect(Unit) { model.invitations.refreshInvitations() }

    TabScaffold(title = "Settings", bottomInset = TabBarHeight) {
        FormSection {
            AccountCard(status, onClick = { push(SettingsDestination.ACCOUNT) })
        }
        FormSection {
            NavigationRow("Security", "open-security", { push(SettingsDestination.SECURITY) }, Icons.Outlined.Security, value = describeSecurity(status))
            RowDivider()
            NavigationRow(
                "Mac approvals", "open-mac-approvals", { push(SettingsDestination.MAC_APPROVALS) }, Icons.Outlined.LaptopMac,
                value = if (isMacApprovalAllowed) "On" else "Off",
            )
            RowDivider()
            NavigationRow("Notifications", "open-notifications", { push(SettingsDestination.NOTIFICATIONS) }, Icons.Outlined.Notifications)
        }
        FormSection {
            NavigationRow(
                "Appearance", "open-appearance", { push(SettingsDestination.APPEARANCE) }, Icons.Outlined.Palette,
                value = "${theme.appearance.label} · ${theme.color.label}",
            )
        }
        FormSection {
            NavigationRow(
                "People", "open-people", { push(SettingsDestination.PEOPLE) }, Icons.Outlined.Group,
                value = data.allReceivedInvitation.size.takeIf { it > 0 }?.toString(),
            )
            RowDivider()
            NavigationRow(
                "Organizations", "open-organizations", { push(SettingsDestination.ORGANIZATIONS) }, Icons.Outlined.Business,
                value = data.allOrganizationInvitation.size.takeIf { it > 0 }?.toString(),
            )
            RowDivider()
            NavigationRow("Devices", "open-devices", { push(SettingsDestination.DEVICES) }, Icons.Outlined.Devices)
        }
        FormSection {
            NavigationRow(
                "AutoFill", "open-autofill", { push(SettingsDestination.AUTOFILL) }, Icons.Outlined.Password,
                value = if (isAutoFillOn) "On" else "Off",
            )
            RowDivider()
            NavigationRow("Password generator", "open-password-generator", { push(SettingsDestination.PASSWORD_GENERATOR) }, Icons.Outlined.AutoFixHigh)
        }
        FormSection {
            NavigationRow("Help", "open-help", { push(SettingsDestination.HELP) }, Icons.AutoMirrored.Outlined.HelpOutline)
            RowDivider()
            NavigationRow("About Hisar", "open-about", { push(SettingsDestination.ABOUT) }, Icons.Outlined.Info, value = appVersionText(context))
        }
        FormSection {
            DestructiveRow("Sign out", { isSignOutConfirmationShown = true }, testTag = "sign-out")
        }
    }

    if (isSignOutConfirmationShown) {
        ConfirmationDialog(
            title = "Sign out of Hisar on this device?",
            message = "You will need your Secret Key and master password to sign in again.",
            confirmLabel = "Sign out",
            confirmTestTag = "confirm-sign-out",
            onConfirm = { scope.launch { model.lock.signOut() } },
            onDismiss = { isSignOutConfirmationShown = false },
        )
    }
}

@Composable
private fun AccountCard(status: VaultStatus, onClick: () -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .clickable(role = Role.Button, onClick = onClick)
            .testTag("open-account")
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        InitialsBadge(describeInitials(status))
        Spacer(Modifier.size(14.dp))
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(status.name.ifEmpty { "Account" }, style = HisarType.headline, color = HisarTheme.colors.label)
            Text(
                "${status.email} · ${VaultLogic.describeServer(status.server)}",
                style = HisarType.footnote,
                color = HisarTheme.colors.secondaryLabel,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Icon(
            Icons.AutoMirrored.Filled.KeyboardArrowRight,
            contentDescription = null,
            tint = HisarTheme.colors.secondaryLabel.copy(alpha = 0.6f),
            modifier = Modifier.size(22.dp),
        )
    }
}

private fun describeInitials(status: VaultStatus): String =
    status.name.ifEmpty { status.email }.split(" ").filter { it.isNotEmpty() }.take(2).joinToString("") { it.first().uppercase() }

private fun describeSecurity(status: VaultStatus): String {
    val autoLock = VaultStatus.describeShortAutoLock(status.autoLockSeconds)
    return if (status.biometricEnabled) "Biometrics · $autoLock" else autoLock
}

/** "1.2 (34)": the version name and the build, as About Hisar shows them. */
fun appVersionText(context: Context): String {
    val packageManager = context.packageManager
    val info = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
        packageManager.getPackageInfo(context.packageName, PackageManager.PackageInfoFlags.of(0))
    } else {
        packageManager.getPackageInfo(context.packageName, 0)
    }
    return "${info.versionName ?: "?"} (${info.longVersionCode})"
}
