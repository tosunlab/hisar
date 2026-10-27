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
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.retain.retain
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
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
import com.tryhisar.hisar.model.CardDescription
import com.tryhisar.hisar.model.VaultSummary
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
    val vaultID = when (request) {
        is EditorRequest.New -> request.vaultID
        is EditorRequest.Edit -> request.vaultID
    }
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val unlockedData by model.unlockedData.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    // Secrets stay out of the saved instance state, which the system may write to disk; retained, they survive a rotation in memory.
    var draft by retain(request) { mutableStateOf((request as? EditorRequest.New)?.let { ItemEditorDraft.open(it.draft, isNew = true) }) }
    var revealed by retain(request) { mutableStateOf(RevealedSecrets()) }
    var isGeneratorShown by remember { mutableStateOf(false) }
    var isIconPickerShown by remember { mutableStateOf(false) }
    var isDeleteConfirmationShown by remember { mutableStateOf(false) }
    var isDiscardConfirmationShown by remember { mutableStateOf(false) }

    LaunchedEffect(request) {
        model.itemEditor.clearError()
        if (request is EditorRequest.Edit && draft == null) {
            model.itemEditor.loadItem(request.vaultID, request.itemID)?.let { draft = ItemEditorDraft.open(it, isNew = false) }
        }
    }

    fun cancel() {
        model.itemEditor.clearError()
        onDone(null)
    }

    fun requestClose() {
        if (isBusy) return
        if (draft?.hasChanges == true) isDiscardConfirmationShown = true else cancel()
    }

    fun save() {
        val current = draft?.withTagsAdded() ?: return
        draft = current
        scope.launch {
            val savedItem = model.itemEditor.saveItem(vaultID, current.buildItem()) ?: return@launch
            (request as? EditorRequest.New)?.onSaved?.invoke(savedItem)
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
                GlassTextButton("Save", onClick = ::save, testTag = "editor-save", enabled = currentDraft?.canSave == true, isProminent = true)
            }
        },
    ) {
        errorMessage?.let { message -> FormSection { ErrorBanner(message) } }
        if (currentDraft == null) return@InlineTitleForm
        val item = currentDraft.item
        val onChange: (ItemEditorDraft) -> Unit = { draft = it }
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
                EditorField(item.title, { value -> draft = currentDraft.withItem { it.copy(title = value) } }, "Title", "editor-title")
            }
        }
        when {
            item.isSecureNote -> {
                FormSection(header = "Note") {
                    EditorField(
                        item.notes, { value -> draft = currentDraft.withItem { it.copy(notes = value) } }, "Recovery codes, door codes, account details…",
                        "editor-notes", kind = FieldKind.NOTES, minLines = 8, maxLines = 30,
                    )
                }
                OtherFieldsSection(currentDraft, onChange)
            }
            item.isCreditCard -> {
                CardDescriptionEffect(model, currentDraft) { description -> draft = draft?.withCardDescription(description) }
                CardSections(currentDraft, onChange, revealed, onRevealedChange)
                OtherFieldsSection(currentDraft, onChange)
            }
            item.isIdentity -> {
                IdentitySections(currentDraft, onChange, revealed, onRevealedChange)
                OtherFieldsSection(currentDraft, onChange)
            }
            item.isAPICredential -> {
                APICredentialSection(currentDraft, onChange, revealed, onRevealedChange)
                OtherFieldsSection(currentDraft, onChange)
            }
            item.isAddress -> {
                AddressSection("Address", currentDraft, onChange)
                OtherFieldsSection(currentDraft, onChange)
            }
            item.isSoftwareLicense -> {
                SoftwareLicenseSection(currentDraft, onChange, revealed, onRevealedChange)
                OtherFieldsSection(currentDraft, onChange)
            }
            !item.isSSHKey -> {
                if (item.isServer) {
                    val allVaultSSHKey = unlockedData.listAllItemReference(vaultID).map { it.item }.filter { it.isSSHKey }
                    ServerSection(currentDraft, onChange, allVaultSSHKey)
                }
                if (item.isDatabase) DatabaseSection(currentDraft, onChange)
                LoginSections(currentDraft, onChange, revealed, onRevealedChange, onGeneratePassword = { isGeneratorShown = true })
                OtherFieldsSection(currentDraft, onChange)
            }
        }
        if (!item.isSecureNote) {
            FormSection(header = "Notes") {
                EditorField(item.notes, { value -> draft = currentDraft.withItem { it.copy(notes = value) } }, "Recovery codes, door codes, anything else", "editor-notes", kind = FieldKind.NOTES, minLines = 3, maxLines = 10)
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
            onIconChange = { icon -> draft = draft?.withItem { it.copy(icon = icon) } },
            onDismiss = { isIconPickerShown = false },
        )
    }
    if (isGeneratorShown) {
        PasswordGeneratorSheet(
            model,
            onUse = { password ->
                draft = draft?.withItem { it.copy(password = password) }
                revealed = revealed.copy(isPassword = true)
                isGeneratorShown = false
            },
            onDismiss = { isGeneratorShown = false },
        )
    }
    if (isDeleteConfirmationShown && currentDraft != null) {
        DestructiveConfirmation(
            title = "Delete “${currentDraft.item.title}”?",
            message = VaultSummary.itemDeletionMessage(unlockedData.allVault.firstOrNull { it.id == vaultID }),
            action = "Delete item",
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
