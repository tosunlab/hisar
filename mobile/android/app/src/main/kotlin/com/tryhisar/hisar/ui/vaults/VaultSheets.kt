package com.tryhisar.hisar.ui.vaults

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.UnfoldMore
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.OrganizationLogic
import com.tryhisar.hisar.model.VaultActions
import com.tryhisar.hisar.model.VaultDeletionLogic
import com.tryhisar.hisar.model.VaultLocation
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.components.noticeClearance
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.launch

/** A sheet's page: its bar, then grouped sections that scroll, and [bottomBar] pinned under them; all stay above the keyboard. */
@Composable
fun SheetPage(bar: @Composable () -> Unit, bottomBar: (@Composable ColumnScope.() -> Unit)? = null, content: @Composable ColumnScope.() -> Unit) {
    Column(
        Modifier
            .fillMaxSize()
            .background(HisarTheme.colors.groupedBackground)
            .imePadding(),
    ) {
        bar()
        Column(
            Modifier
                .weight(1f)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = ScreenMargin),
        ) {
            content()
            Spacer(Modifier.height(32.dp))
        }
        if (bottomBar != null) {
            Column(
                Modifier
                    .fillMaxWidth()
                    .noticeClearance()
                    .windowInsetsPadding(WindowInsets.navigationBars)
                    .padding(horizontal = ScreenMargin, vertical = 8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
                content = bottomBar,
            )
        }
    }
}

/** A row that picks one of [allOption] from a menu, as an inline-labelled Picker. */
@Composable
fun <Option> PickerRow(
    label: String,
    selected: Option?,
    allOption: List<Option>,
    optionLabel: (Option) -> String,
    onSelect: (Option) -> Unit,
    testTag: String,
    enabled: Boolean = true,
) {
    var isExpanded by remember { mutableStateOf(false) }
    Box {
        Row(
            Modifier
                .fillMaxWidth()
                .heightIn(min = 52.dp)
                .clickable(enabled = enabled, role = Role.DropdownList) { isExpanded = true }
                .testTag(testTag)
                .padding(horizontal = 16.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(label, style = HisarType.body, color = if (enabled) HisarTheme.colors.label else HisarTheme.colors.secondaryLabel, modifier = Modifier.weight(1f))
            Text(selected?.let(optionLabel).orEmpty(), style = HisarType.body, color = HisarTheme.colors.secondaryLabel)
            Icon(Icons.Filled.UnfoldMore, contentDescription = null, tint = HisarTheme.colors.secondaryLabel, modifier = Modifier.size(18.dp))
        }
        HisarMenu(isExpanded, onDismiss = { isExpanded = false }) {
            for (option in allOption) {
                HisarMenuItem(optionLabel(option), "$testTag-${optionLabel(option)}", onClick = {
                    isExpanded = false
                    onSelect(option)
                }, icon = if (option == selected) Icons.Filled.Check else null)
            }
        }
    }
}

/** A new vault: its name, and Where once the user's teams are known. The iPhone's NewVaultView. */
@Composable
fun NewVaultScreen(model: AppModel, onCancel: () -> Unit, onCreated: (VaultSummary) -> Unit) {
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val remoteChangeCount by model.remoteChangeCount.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val focusRequester = remember { FocusRequester() }
    var name by rememberSaveable { mutableStateOf("") }
    var locationID by rememberSaveable { mutableStateOf(VaultLocation.sharedVault.id) }
    // Null until Where is loaded, and while it could not be: the teams are unknown then, so Create waits.
    var allLocation by remember { mutableStateOf<List<VaultLocation>?>(null) }
    val location = OrganizationLogic.findVaultLocation(locationID, allLocation)
    val canCreate = name.isNotBlank() && location != null

    LaunchedEffect(Unit) {
        model.errorMessage.value = null
        focusRequester.requestFocus()
    }
    // A failed reload keeps the choices already loaded; without any, Create stays disabled and the banner says why.
    LaunchedEffect(remoteChangeCount) { allLocation = model.vaults.loadVaultLocations() ?: allLocation }
    BackHandler(enabled = isBusy) {}

    fun create() {
        val chosen = location ?: return
        if (!canCreate || isBusy) return
        scope.launch {
            val vault = if (chosen.isSharedVault) {
                model.vaults.createVault(name)
            } else {
                model.vaults.createOrganizationVault(chosen.organizationID, chosen.teamID, name)
            }
            if (vault != null) onCreated(vault)
        }
    }

    SheetPage(bar = {
        SheetTopBar("New vault", onCancel = {
            model.errorMessage.value = null
            onCancel()
        }, cancelTestTag = "new-vault-cancel") {
            if (isBusy) {
                CircularProgressIndicator(Modifier.padding(horizontal = 16.dp).size(22.dp), color = HisarTheme.colors.secondaryLabel, strokeWidth = 2.dp)
            } else {
                BarCapsuleButton("Create", ::create, testTag = "new-vault-create", enabled = canCreate, isBold = true)
            }
        }
    }) {
        errorMessage?.let { message -> FormSection { ErrorBanner(message) } }
        FormSection(footer = location?.let { chosen ->
            {
                Text(
                    if (chosen.isSharedVault) {
                        "A shared vault starts with only you. Share it by email from the vault's ⋯ menu; your Personal vault stays private."
                    } else {
                        "${chosen.teamName}'s members get this vault. Manage it in Settings → Organizations."
                    },
                )
            }
        }) {
            FormTextField(
                name,
                { name = it },
                placeholder = "Production, Staging, Team…",
                testTag = "new-vault-name",
                modifier = Modifier.focusRequester(focusRequester),
                keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Words, imeAction = ImeAction.Done),
                keyboardActions = KeyboardActions(onDone = { create() }),
                label = "Vault name",
            )
            val allLoadedLocation = allLocation
            if (allLoadedLocation != null && (allLoadedLocation.size > 1 || location == null)) {
                RowDivider()
                PickerRow("Where", location, allLoadedLocation, { it.label }, { locationID = it.id }, testTag = "new-vault-where")
            }
        }
    }
}

/**
 * Deleting a vault: who loses access, then moving its items to another vault or deleting them once the vault's name is
 * typed, in the field right above the pinned Delete vault button. The iPhone's VaultDeletionSheet.
 */
@Composable
fun VaultDeletionScreen(model: AppModel, actions: VaultActions, onClose: () -> Unit) {
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var isDeletingItems by rememberSaveable { mutableStateOf(false) }
    var targetVaultID by rememberSaveable { mutableStateOf(VaultDeletionLogic.findDefaultTargetVaultID(actions)) }
    var typedName by rememberSaveable { mutableStateOf("") }
    val itemCount = actions.vault.itemCount
    val isMovePossible = actions.allTargetVault.isNotEmpty()
    val isMoving = isMovePossible && !isDeletingItems
    val request = VaultDeletionLogic.buildRequest(actions, isDeletingItems, targetVaultID, typedName)

    LaunchedEffect(Unit) { model.errorMessage.value = null }
    BackHandler(enabled = isBusy) {}

    SheetPage(
        bar = { SheetTopBar("Delete “${actions.vault.name}”?", onCancel = onClose, cancelTestTag = "delete-vault-cancel") },
        bottomBar = {
            if (itemCount > 0 && !isMoving) {
                Box(Modifier.fillMaxWidth().background(HisarTheme.colors.cellBackground, RoundedCornerShape(26.dp))) {
                    FormTextField(
                        typedName,
                        { typedName = it },
                        placeholder = "Type ${actions.vault.name} to confirm",
                        testTag = "deletion-confirm-name",
                        keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.None, autoCorrectEnabled = false),
                        label = "Vault name",
                    )
                }
            }
            Button(
                onClick = {
                    val chosen = request ?: return@Button
                    scope.launch { if (model.vaults.deleteVault(actions.vault.id, chosen)) onClose() }
                },
                enabled = request != null && !isBusy,
                colors = ButtonDefaults.buttonColors(
                    containerColor = HisarTheme.colors.destructive,
                    contentColor = Color.Black,
                    disabledContainerColor = HisarTheme.colors.fill,
                    disabledContentColor = HisarTheme.colors.secondaryLabel,
                ),
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 52.dp)
                    .testTag("delete-vault-confirm"),
            ) {
                if (isBusy) {
                    CircularProgressIndicator(Modifier.size(22.dp), color = HisarTheme.colors.secondaryLabel, strokeWidth = 2.dp)
                } else {
                    Text("Delete vault", style = HisarType.headline)
                }
            }
        },
    ) {
        errorMessage?.let { message -> FormSection { ErrorBanner(message) } }
        if (actions.allMemberLosingAccess.isNotEmpty()) {
            FormSection(
                header = "These people lose access",
                footer = { Text("Their Hisar removes the vault on its next sync and tells them it is no longer available.") },
            ) {
                actions.allMemberLosingAccess.forEachIndexed { index, member ->
                    if (index > 0) RowDivider()
                    Row(
                        Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 16.dp, vertical = 10.dp)
                            .testTag("losing-access-${member.email}"),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Box(Modifier.weight(1f)) { TitleAndSubtitle(member.name.ifEmpty { member.email }, member.email) }
                        Text(VaultLogic.describeVaultRole(member.role), style = HisarType.body, color = HisarTheme.colors.secondaryLabel)
                    }
                }
            }
        }
        if (itemCount > 0) {
            FormSection(
                header = "Items",
                footer = {
                    Text(
                        if (isMoving) {
                            "Hisar re-encrypts each item on this device with the other vault's key before it moves. The server never sees them."
                        } else {
                            "Deleted items can't be restored. Export your data first if you may need them."
                        },
                    )
                },
            ) {
                val count = VaultLogic.describeItemCount(itemCount)
                if (isMovePossible) {
                    Column(Modifier.selectableGroup().testTag("deletion-choice")) {
                        ChoiceRow("Move $count to another vault", isSelected = !isDeletingItems, testTag = "deletion-choice-move") { isDeletingItems = false }
                        RowDivider()
                        ChoiceRow("Delete the vault and its $count", isSelected = isDeletingItems, testTag = "deletion-choice-delete") { isDeletingItems = true }
                    }
                    if (isMoving) RowDivider()
                }
                if (isMoving) {
                    PickerRow(
                        "Move to",
                        actions.allTargetVault.firstOrNull { it.id == targetVaultID },
                        actions.allTargetVault,
                        { it.name },
                        { targetVaultID = it.id },
                        testTag = "deletion-target",
                    )
                } else if (!isMovePossible) {
                    FormRow { Text("Delete the vault and its $count", style = HisarType.body, color = HisarTheme.colors.label) }
                }
            }
        } else {
            FormSection { FormRow { Text("This vault is empty.", style = HisarType.body, color = HisarTheme.colors.secondaryLabel) } }
        }
    }
}

/** One choice of an inline Picker: its text, with a checkmark when chosen. */
@Composable
private fun ChoiceRow(text: String, isSelected: Boolean, testTag: String, onSelect: () -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 52.dp)
            .selectable(selected = isSelected, role = Role.RadioButton, onClick = onSelect)
            .testTag(testTag)
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(text, style = HisarType.body, color = HisarTheme.colors.label, modifier = Modifier.weight(1f))
        if (isSelected) Icon(Icons.Filled.Check, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.size(20.dp))
    }
}
