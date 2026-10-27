package com.tryhisar.hisar.ui.item

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.VaultDestinations
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MoveItemSheet(model: AppModel, sourceVaultID: String, item: ItemDetail, onDismiss: () -> Unit, onMoved: (String) -> Unit) {
    val data by model.unlockedData.collectAsStateWithLifecycle()
    val busy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val groups = VaultDestinations.groups(model.item.moveDestinations(sourceVaultID), data.allOrganization)
    var targetVaultID by remember { mutableStateOf("") }
    val selected = groups.flatMap { it.destinations }.firstOrNull { it.id == targetVaultID }
    val scope = rememberCoroutineScope()
    ModalBottomSheet(
        onDismissRequest = { if (!busy) onDismiss() },
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = HisarTheme.colors.groupedBackground,
    ) {
        Column(Modifier.fillMaxHeight(0.9f).testTag("move-item-sheet")) {
            Text("Move to vault", style = HisarType.headline, color = HisarTheme.colors.label, modifier = Modifier.padding(16.dp))
            Column(Modifier.weight(1f).verticalScroll(rememberScrollState()).padding(horizontal = 16.dp)) {
                Text("Move “${item.title}” from ${data.findVault(sourceVaultID)?.name ?: "this vault"}.", style = HisarType.body, color = HisarTheme.colors.label)
                errorMessage?.let { ErrorBanner(it) }
                for (group in groups) {
                    Text(group.name, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel, modifier = Modifier.padding(top = 16.dp))
                    for (destination in group.destinations) {
                        Row(Modifier.fillMaxWidth().selectable(selected = targetVaultID == destination.id, enabled = !busy, role = Role.RadioButton,
                            onClick = { targetVaultID = destination.id }).testTag("move-item-target-${destination.id}").padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                            RadioButton(selected = targetVaultID == destination.id, onClick = null)
                            Text(destination.label, style = HisarType.body, color = HisarTheme.colors.label)
                        }
                    }
                }
                if (selected != null) {
                    Column(Modifier.padding(vertical = 16.dp).testTag("move-item-context")) {
                        Text(selected.context, style = HisarType.body, color = HisarTheme.colors.label)
                        Text("Everyone with access to the destination gains access to this item. People with source access but no destination access lose it on their next sync. Copies they retained independently cannot be revoked.", style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
                        Text("hisar:// secret references that use the source vault's name need the new vault name.", style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
                        if (item.isServer && !item.server?.sshKeyID.isNullOrEmpty()) Text("The linked SSH key must exist in the destination vault or be selected again after moving.", style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
                    }
                }
            }
            Row(Modifier.fillMaxWidth().padding(16.dp)) {
                ToolbarTextButton("Cancel", onDismiss, "move-item-cancel", enabled = !busy)
                ToolbarTextButton("Move", {
                    scope.launch { if (model.item.moveItem(sourceVaultID, item.id, targetVaultID) != null) onMoved(targetVaultID) }
                }, "move-item-confirm", enabled = selected != null && !busy, isProminent = true)
            }
        }
    }
}
