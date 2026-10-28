package com.tryhisar.hisar.ui.item

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.EditorRequest
import com.tryhisar.hisar.app.ItemEditorDraft
import com.tryhisar.hisar.app.CredentialImportModel
import com.tryhisar.hisar.model.CardDescription
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import com.tryhisar.hisar.ui.vaults.AppItemIcon
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** The item editor for a new or an existing item; [onDone] gets the saved item's ID, or null when cancelled. */
@Composable
fun ItemEditorScreen(model: AppModel, request: EditorRequest, onDone: (savedItemID: String?) -> Unit) {
    val session by model.itemEditor.session.collectAsStateWithLifecycle()
    val vaultID = session?.chosenVaultID ?: request.vaultID
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val unlockedData by model.unlockedData.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val draft = session?.takeIf { it.request === request }?.draft
    val lockCount by model.lockCount.collectAsStateWithLifecycle()
    var revealed by remember(request, lockCount) { mutableStateOf(RevealedSecrets()) }
    var isGeneratorShown by remember { mutableStateOf(false) }
    var isIconPickerShown by remember { mutableStateOf(false) }
    var isDeleteConfirmationShown by remember { mutableStateOf(false) }
    var isDiscardConfirmationShown by remember { mutableStateOf(false) }
    val allFieldNameFocus = remember { mutableMapOf<Int, FocusRequester>() }
    val fieldNameFocus: (Int) -> FocusRequester = { index -> allFieldNameFocus.getOrPut(index) { FocusRequester() } }

    LaunchedEffect(request) {
        model.itemEditor.clearError()
        model.itemEditor.open(request)
    }

    fun cancel() {
        model.itemEditor.clearError()
        model.itemEditor.discard()
        onDone(null)
    }

    fun requestClose() {
        if (isBusy) return
        if (draft?.hasChanges == true || vaultID != request.vaultID) isDiscardConfirmationShown = true else cancel()
    }

    fun save() {
        val current = draft?.withTagsAdded() ?: return
        current.unnamedFieldIndex?.let { index ->
            model.itemEditor.updateDraft(current.copy(isUnnamedFieldShown = true))
            fieldNameFocus(index).requestFocus()
            return
        }
        model.itemEditor.updateDraft(current)
        scope.launch {
            val savedItem = model.itemEditor.saveItem(vaultID, current.buildItem()) ?: return@launch
            (request as? EditorRequest.New)?.onSaved?.invoke(vaultID, savedItem)
            onDone(savedItem.id)
        }
    }

    fun delete() {
        val original = draft?.original ?: return
        isDeleteConfirmationShown = false
        scope.launch {
            if (model.itemEditor.deleteItem(vaultID, original.id)) onDone(null)
        }
    }

    BackHandler(onBack = ::requestClose)

    val currentDraft = draft
    InlineTitleForm(
        title = currentDraft?.navigationTitle ?: "Edit item",
        leading = { GlassTextButton("Cancel", onClick = ::requestClose, testTag = "editor-cancel", enabled = !isBusy) },
        trailing = {
            if (isBusy) {
                ToolbarSpinner()
            } else {
                GlassTextButton("Save", onClick = ::save, testTag = "editor-save", enabled = currentDraft?.canSave == true && session?.recoveryMessage == null, isProminent = true)
            }
        },
    ) {
        errorMessage?.let { message -> FormSection { ErrorBanner(message) } }
        session?.recoveryMessage?.let { message ->
            FormSection(footer = { Text(message) }) {
                EditorButtonRow("Recover a copy", Icons.Outlined.ContentCopy, onClick = { model.itemEditor.recoverCopy() }, testTag = "editor-recover-copy", enabled = unlockedData.allVault.any(CredentialImportModel::isWritableVault))
                EditorButtonRow("Discard draft", Icons.Outlined.Delete, onClick = ::cancel, testTag = "editor-discard-draft")
            }
        }
        if (currentDraft == null) return@InlineTitleForm
        val item = currentDraft.item
        val onChange: (ItemEditorDraft) -> Unit = model.itemEditor::updateDraft
        val onRevealedChange: (RevealedSecrets) -> Unit = { revealed = it }
        FormSection {
            Row(Modifier.padding(start = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                Box(
                    Modifier
                        .clickable(role = Role.Button) { isIconPickerShown = true }
                        .semantics { contentDescription = "Change icon" }
                        .testTag("editor-icon"),
                ) {
                    AppItemIcon(model, currentDraft.buildItem(), size = 40.dp)
                    Icon(
                        Icons.Filled.Edit,
                        contentDescription = null,
                        tint = HisarTheme.colors.cellBackground,
                        modifier = Modifier
                            .align(Alignment.BottomEnd)
                            .offset(4.dp, 4.dp)
                            .size(16.dp)
                            .background(HisarTheme.colors.secondaryLabel, CircleShape)
                            .padding(3.dp),
                    )
                }
                EditorField(item.title, { value -> model.itemEditor.updateDraft(currentDraft.withItem { it.copy(title = value) }) }, "Title", "editor-title", label = "Title")
            }
            if (currentDraft.isNew) {
                val writableVaults = unlockedData.allVault.filter(CredentialImportModel::isWritableVault)
                MenuPickerRow("Vault", writableVaults.map { it.id }, vaultID, { id -> writableVaults.firstOrNull { it.id == id }?.name.orEmpty() }, model.itemEditor::chooseVault, "editor-vault")
            }
        }
        when {
            item.isSecureNote -> {
                FormSection(header = "Note") {
                    EditorField(
                        item.notes, { value -> model.itemEditor.updateDraft(currentDraft.withItem { it.copy(notes = value) }) }, "Recovery codes, door codes, account details…",
                        "editor-notes", kind = FieldKind.NOTES, minLines = 8, maxLines = 30,
                        label = "Notes",
                    )
                }
                OtherFieldsSection(currentDraft, onChange, fieldNameFocus)
            }
            item.isCreditCard -> {
                CardDescriptionEffect(model, currentDraft) { description -> model.itemEditor.session.value?.takeIf { it.request === request }?.draft?.let { model.itemEditor.updateDraft(it.withCardDescription(description)) } }
                CardSections(currentDraft, onChange, revealed, onRevealedChange)
                OtherFieldsSection(currentDraft, onChange, fieldNameFocus)
            }
            item.isIdentity -> {
                IdentitySections(currentDraft, onChange, revealed, onRevealedChange)
                OtherFieldsSection(currentDraft, onChange, fieldNameFocus)
            }
            item.isAPICredential -> {
                APICredentialSection(currentDraft, onChange, revealed, onRevealedChange)
                OtherFieldsSection(currentDraft, onChange, fieldNameFocus)
            }
            item.isAddress -> {
                AddressSection("Address", currentDraft, onChange)
                OtherFieldsSection(currentDraft, onChange, fieldNameFocus)
            }
            item.isSoftwareLicense -> {
                SoftwareLicenseSection(currentDraft, onChange, revealed, onRevealedChange)
                OtherFieldsSection(currentDraft, onChange, fieldNameFocus)
            }
            !item.isSSHKey -> {
                if (item.isServer) {
                    val allVaultSSHKey = unlockedData.listAllItemReference(vaultID).map { it.item }.filter { it.isSSHKey }
                    ServerSection(currentDraft, onChange, allVaultSSHKey)
                }
                if (item.isDatabase) DatabaseSection(currentDraft, onChange)
                LoginSections(currentDraft, onChange, revealed, onRevealedChange, onGeneratePassword = { isGeneratorShown = true })
                OtherFieldsSection(currentDraft, onChange, fieldNameFocus)
            }
        }
        if (!item.isSecureNote) {
            FormSection(header = "Notes") {
                EditorField(item.notes, { value -> model.itemEditor.updateDraft(currentDraft.withItem { it.copy(notes = value) }) }, "Recovery codes, door codes, anything else", "editor-notes", kind = FieldKind.NOTES, minLines = 3, maxLines = 10, label = "Notes")
            }
        }
        TagSection(currentDraft, onChange)
        if (!currentDraft.isNew) {
            FormSection {
                Box(
                    Modifier
                        .fillMaxWidth()
                        .clickable(role = Role.Button) { isDeleteConfirmationShown = true }
                        .testTag("editor-delete")
                        .padding(vertical = 15.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    Text("Delete item", style = HisarType.body, color = HisarTheme.colors.destructive, textAlign = TextAlign.Center)
                }
            }
        }
    }

    if (isIconPickerShown && currentDraft != null) {
        IconPickerSheet(
            model,
            currentDraft.buildItem(),
            onIconChange = { icon -> model.itemEditor.session.value?.takeIf { it.request === request }?.draft?.let { model.itemEditor.updateDraft(it.withItem { it.copy(icon = icon) }) } },
            onDismiss = { isIconPickerShown = false },
        )
    }
    if (isGeneratorShown) {
        PasswordGeneratorSheet(
            model,
            onUse = { password ->
                model.itemEditor.session.value?.takeIf { it.request === request }?.draft?.let { model.itemEditor.updateDraft(it.withItem { it.copy(password = password) }) }
                revealed = revealed.copy(isPassword = true)
                isGeneratorShown = false
            },
            onDismiss = { isGeneratorShown = false },
        )
    }
    if (isDeleteConfirmationShown && currentDraft != null) {
        DestructiveConfirmation(
            title = "Delete “${currentDraft.item.title}”?",
            message = "It is deleted on every device that can see this vault.",
            action = "Delete",
            onConfirm = ::delete,
            onDismiss = { isDeleteConfirmationShown = false },
            testTag = "editor-delete-confirmation",
        )
    }
    if (isDiscardConfirmationShown) {
        DestructiveConfirmation(
            title = "Discard changes?",
            message = "What you changed in this item is not saved.",
            action = "Discard changes",
            onConfirm = {
                isDiscardConfirmationShown = false
                cancel()
            },
            onDismiss = { isDiscardConfirmationShown = false },
            testTag = "editor-discard-confirmation",
            dismissLabel = "Keep editing",
        )
    }
}

/** Asks the backend about the card's number and expiry 250 ms after typing stops, as the iPhone editor does. */
@Composable
private fun CardDescriptionEffect(model: AppModel, draft: ItemEditorDraft, onDescription: (CardDescription) -> Unit) {
    val number = draft.item.card?.number.orEmpty()
    val expiry = draft.item.card?.expiry.orEmpty()
    LaunchedEffect(number, expiry) {
        delay(250)
        model.itemEditor.describeCard(number, expiry)?.let(onDescription)
    }
}
