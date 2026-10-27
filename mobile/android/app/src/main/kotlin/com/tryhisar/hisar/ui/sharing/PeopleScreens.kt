package com.tryhisar.hisar.ui.sharing

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Group
import androidx.compose.material.icons.outlined.Cancel
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material.icons.outlined.PersonAdd
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.SentShareInvitation
import com.tryhisar.hisar.model.ShareInvitations
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.PushedTopBar
import com.tryhisar.hisar.ui.root.TabBarHeight
import com.tryhisar.hisar.ui.settings.ConfirmationDialog
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.vaults.BarCapsuleButton
import com.tryhisar.hisar.ui.vaults.InvitationRow
import com.tryhisar.hisar.ui.vaults.SwipeActionRow
import kotlinx.coroutines.launch

/**
 * Settings → People: invitations to this user's email, the people they share vaults with, the invitations they sent and
 * Invite to Hisar. The iPhone's PeopleView, with PersonView pushed inside it.
 */
@Composable
fun PeopleScreen(model: AppModel, onBack: () -> Unit) {
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val remoteChangeCount by model.remoteChangeCount.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var people by remember { mutableStateOf<com.tryhisar.hisar.app.PeopleSnapshot?>(null) }
    val allPerson = people?.allPerson
    val shareInvitations = people?.invitations ?: ShareInvitations()
    var isInviteShown by rememberSaveable { mutableStateOf(false) }
    var invitationToCancel by remember { mutableStateOf<SentShareInvitation?>(null) }
    var openPerson by rememberSaveable(stateSaver = memberSaver) { mutableStateOf<Member?>(null) }
    var actionError by remember { mutableStateOf<String?>(null) }

    suspend fun load() {
        people = model.sharing.loadPeople(people)
    }

    LaunchedEffect(remoteChangeCount) { load() }

    val shownPerson = openPerson
    if (shownPerson != null) {
        BackHandler { openPerson = null }
        PersonScreen(shownPerson, onBack = { openPerson = null })
        return
    }

    RefreshablePage(bar = { PushedTopBar(if (it) "People" else "", onBack) }, onRefresh = { load() }, largeTitle = "People", bottomInset = TabBarHeight) {
        (actionError ?: errorMessage)?.let { FormSection { ErrorBanner(it) } }
        if (shareInvitations.allReceived.isNotEmpty() || shareInvitations.allWaiting.isNotEmpty()) {
            FormSection(header = "Invitations for you") {
                shareInvitations.allReceived.forEachIndexed { index, invitation ->
                    if (index > 0) SymbolRowDivider(50.dp)
                    InvitationRow(
                        text = "${invitation.inviterName} shared a vault with you as ${VaultLogic.describeVaultRole(invitation.role).lowercase()}",
                        email = invitation.invitedByEmail,
                        icon = Icons.Filled.Group,
                        testTagPrefix = "",
                        testTagName = "invitation",
                        onDecline = {
                            scope.launch {
                                actionError = null
                                if (model.invitations.declineShareInvitation(invitation.id)) {
                                    model.showNotice("Invitation declined.")
                                    load()
                                } else {
                                    actionError = model.errorMessage.value
                                }
                            }
                        },
                        onAccept = {
                            scope.launch {
                                actionError = null
                                if (model.invitations.acceptShareInvitation(invitation.id)) {
                                    model.showNotice("Accepted. The vault appears once ${invitation.inviterName}'s Hisar finishes sharing.")
                                    load()
                                } else {
                                    actionError = model.errorMessage.value
                                }
                            }
                        },
                    )
                }
                shareInvitations.allWaiting.forEach { invitation ->
                    LabelRow(androidx.compose.material.icons.Icons.Outlined.Schedule, HisarTheme.colors.secondaryLabel, testTag = "waiting-${invitation.id}") {
                        TitleAndDetail(invitation.describe(java.time.Instant.now()), invitation.invitedByEmail)
                    }
                }
            }
        }
        val loadedPeople = allPerson
        FormSection(
            header = "People you share vaults with",
            footer = { Text("People appear here once you share a vault with them. To compare codes, ask each person for the security code Hisar shows for their own account in Settings → People, the row marked you. It must match the code you see for them.") },
        ) {
            if (loadedPeople == null) {
                LoadingRow("Loading people…")
            } else {
                SwipeableRows {
                    loadedPeople.forEachIndexed { index, person ->
                        if (index > 0) SymbolRowDivider()
                        MemberRow(person, roleText = "", testTag = "person-${person.email}", onClick = { openPerson = person })
                    }
                }
            }
        }
        if (shareInvitations.allSent.isNotEmpty() || shareInvitations.completionError.isNotEmpty()) {
            FormSection(
                header = "Invitations you sent",
                footer = shareInvitations.completionError.takeIf { it.isNotEmpty() }?.let { completionError ->
                    { Text("Could not finish sharing: $completionError", color = HisarTheme.colors.warning) }
                },
            ) {
                SwipeableRows {
                    shareInvitations.allSent.forEachIndexed { index, invitation ->
                        if (index > 0) SymbolRowDivider(52.dp)
                        SwipeActionRow(
                            "Cancel", Icons.Outlined.Cancel, "cancel-sent-invitation-${invitation.email}", isLeading = false,
                            onAction = { invitationToCancel = invitation }, actionColor = HisarTheme.colors.destructive,
                        ) {
                            SentInvitationRow(
                                model, invitation, isVaultNameShown = true, testTag = "sent-invitation-${invitation.email}",
                                onCancel = { invitationToCancel = invitation },
                                onChanged = { load() },
                            )
                        }
                    }
                }
            }
        }
        if (loadedPeople != null) {
            FormSection(footer = {
                Text("Creates a Hisar account: the server emails them a sign-up link. To give them a vault, choose Share in the vault's actions menu.")
            }) {
                FormButtonRow("Invite to Hisar", { isInviteShown = true }, testTag = "invite-person", icon = Icons.Outlined.PersonAdd, color = HisarTheme.colors.tint)
            }
        }
    }

    invitationToCancel?.let { invitation ->
        ConfirmationDialog(
            title = "Cancel the invitation to ${invitation.email}?",
            message = null,
            confirmLabel = "Cancel invitation",
            confirmTestTag = "confirm-cancel-invitation",
            onConfirm = {
                scope.launch {
                    actionError = null
                    if (model.sharing.cancelShareInvitation(invitation.id)) {
                        model.showNotice("Invitation to ${invitation.email} cancelled.")
                        load()
                    } else {
                        actionError = model.errorMessage.value
                    }
                }
            },
            onDismiss = { invitationToCancel = null },
        )
    }
    if (isInviteShown) InvitePersonSheet(model, onDismiss = { isInviteShown = false })
}

/** Someone the user shares vaults with: their security code, to compare once. */
@Composable
private fun PersonScreen(person: Member, onBack: () -> Unit) {
    RefreshablePage(bar = { PushedTopBar(person.name, onBack) }, onRefresh = null, bottomInset = TabBarHeight) {
        PersonHeader(person)
        if (person.isKeyChanged) {
            FormSection {
                WarningRow("Their key changed since you last shared with them. Check the security code with them before sharing again.", testTag = "person-key-changed")
            }
        }
        SecurityCodeSection(
            person,
            footer = describeSecurityCode(person),
            testTag = "person-security-code",
            isShareable = false,
        )
    }
}

/** Invite to Hisar: the server emails a sign-up link to the address. */
@Composable
private fun InvitePersonSheet(model: AppModel, onDismiss: () -> Unit) {
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val focusRequester = remember { FocusRequester() }
    var email by rememberSaveable { mutableStateOf("") }
    val canInvite = "@" in email

    LaunchedEffect(Unit) {
        model.errorMessage.value = null
        focusRequester.requestFocus()
    }

    fun invite() {
        if (!canInvite || isBusy) return
        val trimmedEmail = email.trim().lowercase()
        scope.launch {
            if (model.sharing.invitePerson(trimmedEmail)) {
                model.showNotice("Invitation sent to $trimmedEmail.")
                onDismiss()
            }
        }
    }

    FormSheet(
        isBusy = isBusy,
        onDismiss = onDismiss,
        bar = {
            ModalSheetBar("Invite to Hisar", onCancel = {
                model.errorMessage.value = null
                onDismiss()
            }) {
                if (isBusy) {
                    CircularProgressIndicator(Modifier.padding(horizontal = 16.dp).size(22.dp), color = HisarTheme.colors.secondaryLabel, strokeWidth = 2.dp)
                } else {
                    BarCapsuleButton("Send", ::invite, testTag = "invite-submit", enabled = canInvite, isBold = true)
                }
            }
        },
    ) {
        errorMessage?.let { FormSection { ErrorBanner(it) } }
        FormSection(
            header = "Who",
            footer = { Text("The server emails them a sign-up link that works once and expires after 7 days. They sign up with this email address.") },
        ) {
            FormTextField(
                email,
                { email = it },
                placeholder = "them@example.com",
                testTag = "invite-email",
                modifier = Modifier.focusRequester(focusRequester),
                keyboardOptions = KeyboardOptions(
                    capitalization = KeyboardCapitalization.None,
                    autoCorrectEnabled = false,
                    keyboardType = KeyboardType.Email,
                    imeAction = ImeAction.Send,
                ),
                keyboardActions = KeyboardActions(onSend = { invite() }),
            )
        }
    }
}
