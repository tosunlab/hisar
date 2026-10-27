package com.tryhisar.hisar.ui.sharing

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.outlined.Cancel
import androidx.compose.material.icons.outlined.PersonRemove
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.SentShareInvitation
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.VaultSharing
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.LabeledRow
import com.tryhisar.hisar.ui.components.PushedTopBar
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.settings.ConfirmationDialog
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.vaults.HisarMenu
import com.tryhisar.hisar.ui.vaults.HisarMenuItem
import com.tryhisar.hisar.ui.vaults.PickerRow
import com.tryhisar.hisar.ui.vaults.SwipeActionRow
import kotlinx.coroutines.launch

private fun describeVaultMemberRemoval(member: Member, vaultName: String): String {
    val name = member.name.ifEmpty { member.email }
    return "$name is removed from “$vaultName”, and Hisar encrypts the vault again with a new key for the people who remain. $name can still read copies their devices already downloaded. Change any password or secret they should no longer know."
}

/**
 * A shared vault's sheet from the vault's actions menu: owners invite by email, see who is invited and manage members; others
 * see the members. The iPhone's VaultSharingView, with VaultMemberView pushed inside it.
 */
@Composable
fun VaultSharingScreen(model: AppModel, vaultID: String, onDone: () -> Unit) {
    val data by model.unlockedData.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val remoteChangeCount by model.remoteChangeCount.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val focusManager = LocalFocusManager.current
    var sharing by remember { mutableStateOf<VaultSharing?>(null) }
    var email by rememberSaveable { mutableStateOf("") }
    var role by rememberSaveable { mutableStateOf(Member.VAULT_ROLE_EDITOR) }
    var sentEmail by rememberSaveable { mutableStateOf<String?>(null) }
    var isSending by remember { mutableStateOf(false) }
    var invitationToCancel by remember { mutableStateOf<SentShareInvitation?>(null) }
    var memberToRemove by remember { mutableStateOf<Member?>(null) }
    var memberWithMenu by remember { mutableStateOf<Member?>(null) }
    var openMember by rememberSaveable(stateSaver = memberSaver) { mutableStateOf(null) }

    val vault = data.findVault(vaultID)
    val vaultName = sharing?.vault?.name ?: vault?.name ?: "Vault"
    val isOwner = sharing?.isOwner ?: (vault?.role == Member.VAULT_ROLE_OWNER)
    val trimmedEmail = email.trim().lowercase()
    val canSend = "@" in trimmedEmail && !isSending

    suspend fun load() {
        model.sharing.loadVaultSharing(vaultID)?.let { sharing = it }
    }

    LaunchedEffect(remoteChangeCount, openMember) { load() }
    BackHandler(enabled = isBusy) {}

    val shownMember = openMember
    if (shownMember != null) {
        BackHandler { openMember = null }
        VaultMemberScreen(model, vaultID, vaultName, shownMember, isOwner, onBack = { openMember = null }, reload = { load() })
        return
    }

    fun send() {
        if (!canSend) return
        val invitedEmail = trimmedEmail
        isSending = true
        scope.launch {
            try {
                if (model.sharing.shareVault(vaultID, invitedEmail, role)) {
                    sentEmail = invitedEmail
                    email = ""
                    focusManager.clearFocus()
                    load()
                }
            } finally {
                isSending = false
            }
        }
    }

    fun changeRole(member: Member, newRole: String) {
        if (newRole == member.role) return
        scope.launch { if (model.sharing.changeVaultMemberRole(vaultID, member.accountID, newRole)) load() }
    }

    RefreshablePage(
        bar = { DoneSheetBar(if (isOwner) "Share “$vaultName”" else "Members", onDone, doneTestTag = "vault-sharing-done") },
        onRefresh = { load() },
    ) {
        errorMessage?.let { FormSection { ErrorBanner(it) } }
        if (isOwner) {
            FormSection(
                header = "Invite by email",
                footer = {
                    Text("${VaultLogic.describeVaultRoleAbility(role)} They accept in Hisar, or create an account from the email first; your Hisar then shares the vault key with them.")
                },
            ) {
                FormTextField(
                    email,
                    { email = it },
                    placeholder = "name@example.com",
                    testTag = "share-email",
                    keyboardOptions = KeyboardOptions(
                        capitalization = KeyboardCapitalization.None,
                        autoCorrectEnabled = false,
                        keyboardType = KeyboardType.Email,
                        imeAction = ImeAction.Send,
                    ),
                    keyboardActions = KeyboardActions(onSend = { send() }),
                )
                RowDivider()
                PickerRow("Role", role, Member.allVaultRole, VaultLogic::describeVaultRole, { role = it }, testTag = "share-role")
                RowDivider()
                SendRow("Send invitation", ::send, enabled = canSend, isSending = isSending, testTag = "share-vault-submit")
                sentEmail?.let { sent ->
                    RowDivider()
                    SentNote(sent, testTag = "share-sent")
                }
            }
        }
        sharing?.completionError?.takeIf { it.isNotEmpty() }?.let { completionError ->
            FormSection(footer = {
                Text("Hisar shares the vault key only with the key it pinned for that person. Compare security codes with them before inviting them again.")
            }) {
                WarningRow("Could not finish sharing: $completionError", testTag = "share-completion-error")
            }
        }
        val loadedSharing = sharing
        FormSection(
            header = loadedSharing?.let { if (it.allMember.size == 1) "1 member" else "${it.allMember.size} members" } ?: "Members",
            footer = {
                Text(
                    if (isOwner) {
                        "Everyone here can open the items in this vault. Tap a person to compare their security code, change their role or remove them."
                    } else {
                        "Everyone here can open the items in this vault. Only its owners can share it or change who has access."
                    },
                )
            },
        ) {
            if (loadedSharing == null) {
                LoadingRow("Loading members…")
            } else {
                SwipeableRows {
                    loadedSharing.allMember.forEachIndexed { index, member ->
                        if (index > 0) SymbolRowDivider()
                        val isManageable = isOwner && !member.isCurrentUser
                        val row = @Composable {
                            MemberRow(
                                member,
                                VaultLogic.describeVaultRole(member.role),
                                testTag = "vault-member-${member.email}",
                                onClick = { openMember = member },
                                onLongClick = if (isManageable) ({ memberWithMenu = member }) else null,
                            ) {
                                HisarMenu(memberWithMenu == member, onDismiss = { memberWithMenu = null }) {
                                    for (choice in Member.allVaultRole) {
                                        HisarMenuItem(
                                            VaultLogic.describeVaultRole(choice),
                                            "member-menu-role-${VaultLogic.describeVaultRole(choice)}",
                                            onClick = {
                                                memberWithMenu = null
                                                changeRole(member, choice)
                                            },
                                            icon = if (choice == member.role) Icons.Filled.Check else null,
                                        )
                                    }
                                    HisarMenuItem("Remove from vault", "member-menu-remove", icon = Icons.Outlined.PersonRemove, isDestructive = true, onClick = {
                                        memberWithMenu = null
                                        memberToRemove = member
                                    })
                                }
                            }
                        }
                        if (isManageable) {
                            SwipeActionRow(
                                "Remove", Icons.Outlined.PersonRemove, "remove-vault-member-${member.email}", isLeading = false,
                                onAction = { memberToRemove = member }, actionColor = HisarTheme.colors.destructive, content = row,
                            )
                        } else {
                            row()
                        }
                    }
                }
            }
        }
        val allInvitation = loadedSharing?.allInvitation.orEmpty()
        if (isOwner && allInvitation.isNotEmpty()) {
            FormSection(header = "Invited") {
                SwipeableRows {
                    allInvitation.forEachIndexed { index, invitation ->
                        if (index > 0) SymbolRowDivider(52.dp)
                        SwipeActionRow(
                            "Cancel", Icons.Outlined.Cancel, "cancel-invitation-${invitation.email}", isLeading = false,
                            onAction = { invitationToCancel = invitation }, actionColor = HisarTheme.colors.destructive,
                        ) {
                            SentInvitationRow(
                                invitation, isVaultNameShown = false, testTag = "vault-invitation-${invitation.email}",
                                onCancel = { invitationToCancel = invitation },
                                onConfirm = { snapshot -> scope.launch { if (model.sharing.confirmShareRecipient(snapshot)) load() } },
                            )
                        }
                    }
                }
            }
        }
    }

    invitationToCancel?.let { invitation ->
        ConfirmationDialog(
            title = "Cancel the invitation to ${invitation.email}?",
            message = "Their invitation link stops working. You can invite them again later.",
            confirmLabel = "Cancel invitation",
            confirmTestTag = "confirm-cancel-invitation",
            onConfirm = { scope.launch { if (model.sharing.cancelShareInvitation(invitation.id)) load() } },
            onDismiss = { invitationToCancel = null },
            dismissLabel = "Keep invitation",
        )
    }
    memberToRemove?.let { member ->
        ConfirmationDialog(
            title = "Remove ${member.name.ifEmpty { member.email }} from “$vaultName”?",
            message = describeVaultMemberRemoval(member, vaultName),
            confirmLabel = "Remove",
            confirmTestTag = "confirm-remove-member",
            onConfirm = { scope.launch { if (model.sharing.removeVaultMember(vaultID, member.accountID)) load() } },
            onDismiss = { memberToRemove = null },
        )
    }
}

/** One person with access to the vault: their security code, and for owners their role and Remove from Vault. */
@Composable
private fun VaultMemberScreen(
    model: AppModel,
    vaultID: String,
    vaultName: String,
    member: Member,
    isOwner: Boolean,
    onBack: () -> Unit,
    reload: suspend () -> Unit,
) {
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var selectedRole by rememberSaveable(member.accountID) { mutableStateOf(member.role) }
    var savedRole by rememberSaveable(member.accountID) { mutableStateOf(member.role) }
    var isRemoveConfirmationShown by remember { mutableStateOf(false) }
    val isManageable = isOwner && !member.isCurrentUser

    fun chooseRole(newRole: String) {
        selectedRole = newRole
        if (newRole == savedRole) return
        scope.launch {
            if (model.sharing.changeVaultMemberRole(vaultID, member.accountID, newRole)) {
                savedRole = newRole
                reload()
            } else {
                selectedRole = savedRole
            }
        }
    }

    RefreshablePage(bar = { PushedTopBar(member.name, onBack) }, onRefresh = reload) {
        errorMessage?.let { FormSection { ErrorBanner(it) } }
        PersonHeader(member)
        if (member.isKeyChanged) {
            FormSection {
                WarningRow(
                    "${member.name}'s key changed since you last shared with them. Hisar refuses to share with them until you compare security codes again.",
                    testTag = "member-key-changed",
                )
            }
        }
        SecurityCodeSection(
            member,
            footer = describeSecurityCode(member),
            testTag = "member-security-code",
        )
        FormSection(footer = { Text(VaultLogic.describeVaultRoleAbility(selectedRole)) }) {
            if (isManageable) {
                PickerRow("Role", selectedRole, Member.allVaultRole, VaultLogic::describeVaultRole, ::chooseRole, testTag = "member-role", enabled = !isBusy)
            } else {
                LabeledRow("Role", VaultLogic.describeVaultRole(savedRole))
            }
        }
        if (isManageable) {
            FormSection {
                DestructiveButtonRow("Remove from vault", Icons.Outlined.PersonRemove, { isRemoveConfirmationShown = true }, testTag = "remove-member", enabled = !isBusy)
            }
        }
    }

    if (isRemoveConfirmationShown) {
        ConfirmationDialog(
            title = "Remove ${member.name.ifEmpty { member.email }} from “$vaultName”?",
            message = describeVaultMemberRemoval(member, vaultName),
            confirmLabel = "Remove",
            confirmTestTag = "confirm-remove-member",
            onConfirm = {
                scope.launch {
                    if (model.sharing.removeVaultMember(vaultID, member.accountID)) {
                        reload()
                        onBack()
                    }
                }
            },
            onDismiss = { isRemoveConfirmationShown = false },
        )
    }
}
