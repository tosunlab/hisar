package com.tryhisar.hisar.ui.vaults

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.StickyNote2
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Badge
import androidx.compose.material.icons.outlined.CreateNewFolder
import androidx.compose.material.icons.outlined.CreditCard
import androidx.compose.material.icons.outlined.DataObject
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.Password
import androidx.compose.material.icons.outlined.Place
import androidx.compose.material.icons.outlined.Storage
import androidx.compose.material.icons.outlined.Verified
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.root.TabBarHeight
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType

/** What a list leaves free under its last row so the New button, above the tab bar, covers none of it. */
val NewButtonClearance = 72.dp

private class NewItemChoice(val title: String, val category: String, val testTag: String, val icon: ImageVector)

private val allNewItemChoice = listOf(
    NewItemChoice("Login", ItemDetail.CATEGORY_LOGIN, "new-login", Icons.Outlined.Password),
    NewItemChoice("Secure note", ItemDetail.CATEGORY_SECURE_NOTE, "new-secure-note", Icons.AutoMirrored.Outlined.StickyNote2),
    NewItemChoice("Credit card", ItemDetail.CATEGORY_CREDIT_CARD, "new-credit-card", Icons.Outlined.CreditCard),
    NewItemChoice("Identity", ItemDetail.CATEGORY_IDENTITY, "new-identity", Icons.Outlined.Badge),
    NewItemChoice("API credential", ItemDetail.CATEGORY_API_CREDENTIAL, "new-api-credential", Icons.Outlined.DataObject),
    NewItemChoice("Address", ItemDetail.CATEGORY_ADDRESS, "new-address", Icons.Outlined.Place),
    NewItemChoice("Software license", ItemDetail.CATEGORY_SOFTWARE_LICENSE, "new-software-license", Icons.Outlined.Verified),
    NewItemChoice("Server", ItemDetail.CATEGORY_SERVER, "new-server", Icons.Outlined.Dns),
    NewItemChoice("Database", ItemDetail.CATEGORY_DATABASE, "new-database", Icons.Outlined.Storage),
)

/** The extended New button of the Vaults and vault screens, just above the floating tab bar. */
@Composable
fun NewButton(onClick: () -> Unit, modifier: Modifier = Modifier) {
    ExtendedFloatingActionButton(
        text = { Text("New", style = HisarType.headline) },
        icon = { Icon(Icons.Outlined.Add, contentDescription = null) },
        onClick = onClick,
        containerColor = HisarTheme.colors.tint,
        contentColor = HisarTheme.colors.onTint,
        modifier = modifier
            .padding(end = ScreenMargin, bottom = TabBarHeight + 12.dp)
            .testTag("new-menu"),
    )
}

/**
 * What New creates: an item of one of nine types in the chosen vault, which starts as the vault shown
 * ([currentVaultID]) when the user may add to it, else the Personal vault; or, last, a new vault.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NewItemSheet(
    allVault: List<VaultSummary>,
    currentVaultID: String?,
    onNewItem: (vaultID: String, category: String) -> Unit,
    onNewVault: () -> Unit,
    onDismiss: () -> Unit,
) {
    var chosenVaultID by rememberSaveable { mutableStateOf<String?>(null) }
    val allWritable = allVault.filter(VaultLogic::isWritable)
    val vault = allWritable.firstOrNull { it.id == chosenVaultID } ?: VaultLogic.chooseNewItemVault(allVault, currentVaultID)
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = HisarTheme.colors.groupedBackground,
    ) {
        Column(
            Modifier
                .verticalScroll(rememberScrollState())
                .padding(horizontal = ScreenMargin)
                .padding(bottom = 24.dp)
                .testTag("new-sheet"),
        ) {
            FormSection {
                PickerRow("Vault", vault, allWritable, { it.name }, { chosenVaultID = it.id }, testTag = "new-item-vault", enabled = allWritable.isNotEmpty())
            }
            FormSection {
                allNewItemChoice.forEachIndexed { index, choice ->
                    if (index > 0) RowDivider()
                    FormButtonRow(choice.title, onClick = {
                        val chosen = vault ?: return@FormButtonRow
                        onDismiss()
                        onNewItem(chosen.id, choice.category)
                    }, testTag = choice.testTag, icon = choice.icon, enabled = vault != null)
                }
            }
            FormSection {
                FormButtonRow("New vault", onClick = {
                    onDismiss()
                    onNewVault()
                }, testTag = "new-vault", icon = Icons.Outlined.CreateNewFolder)
            }
        }
    }
}
