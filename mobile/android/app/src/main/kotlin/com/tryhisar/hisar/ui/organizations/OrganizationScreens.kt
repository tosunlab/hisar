package com.tryhisar.hisar.ui.organizations

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Logout
import androidx.compose.material.icons.automirrored.outlined.Undo
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material.icons.filled.Business
import androidx.compose.material.icons.outlined.AccountCircle
import androidx.compose.material.icons.outlined.AddCircleOutline
import androidx.compose.material.icons.outlined.Business
import androidx.compose.material.icons.outlined.Cancel
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Email
import androidx.compose.material.icons.outlined.Groups
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.MoreHoriz
import androidx.compose.material.icons.outlined.NoEncryption
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material.icons.outlined.PersonAdd
import androidx.compose.material.icons.outlined.PersonRemove
import androidx.compose.material.icons.outlined.VerifiedUser
import androidx.compose.material3.AlertDialog
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
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.Refusal
import com.tryhisar.hisar.app.takeRefusal
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.Organization
import com.tryhisar.hisar.model.OrganizationAccessChange
import com.tryhisar.hisar.model.OrganizationInvitation
import com.tryhisar.hisar.model.OrganizationLoad
import com.tryhisar.hisar.model.OrganizationLogic
import com.tryhisar.hisar.model.OrganizationMemberAction
import com.tryhisar.hisar.model.OrganizationRole
import com.tryhisar.hisar.model.SentOrganizationInvitation
import com.tryhisar.hisar.model.Team
import com.tryhisar.hisar.model.TeamMember
import com.tryhisar.hisar.model.TeamVault
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.LabeledRow
import com.tryhisar.hisar.ui.components.LargeTitle
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.PushedTopBar
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.components.ToolbarIconButton
import com.tryhisar.hisar.ui.root.TabBarHeight
import com.tryhisar.hisar.ui.settings.ConfirmationDialog
import com.tryhisar.hisar.ui.sharing.DestructiveButtonRow
import com.tryhisar.hisar.ui.sharing.FormSheet
import com.tryhisar.hisar.ui.sharing.InvitedRow
import com.tryhisar.hisar.ui.sharing.LabelRow
import com.tryhisar.hisar.ui.sharing.LoadingRow
import com.tryhisar.hisar.ui.sharing.MemberRow
import com.tryhisar.hisar.ui.sharing.ModalSheetBar
import com.tryhisar.hisar.ui.sharing.PersonHeader
import com.tryhisar.hisar.ui.sharing.RefreshablePage
import com.tryhisar.hisar.ui.sharing.SectionCard
import com.tryhisar.hisar.ui.sharing.SecurityCodeSection
import com.tryhisar.hisar.ui.sharing.SendRow
import com.tryhisar.hisar.ui.sharing.SentNote
import com.tryhisar.hisar.ui.sharing.SymbolRowDivider
import com.tryhisar.hisar.ui.sharing.TitleAndDetail
import com.tryhisar.hisar.ui.sharing.WarningRow
import com.tryhisar.hisar.ui.sharing.describeMemberName
import com.tryhisar.hisar.ui.sharing.memberSaver
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.vaults.BarCapsuleButton
import com.tryhisar.hisar.ui.vaults.HisarMenu
import com.tryhisar.hisar.ui.vaults.HisarMenuItem
import com.tryhisar.hisar.ui.vaults.InvitationRow
import com.tryhisar.hisar.ui.vaults.NavigationRow
import com.tryhisar.hisar.ui.vaults.PickerRow
import com.tryhisar.hisar.ui.vaults.RefusalAlert
import com.tryhisar.hisar.ui.vaults.RowSymbol
import com.tryhisar.hisar.ui.vaults.SwipeActionRow
import com.tryhisar.hisar.ui.vaults.TitleAndSubtitle
import kotlinx.coroutines.launch

private const val ADMIN_ROLE_NOTE = "Admins manage members, teams and which teams have which vaults; being an admin gives no vault."

/**
 * Settings → Organizations: invitations to this user's email, their organizations and Create organization…. The
 * iPhone's OrganizationsView, with OrganizationView pushed inside it.
 */
@Composable
fun OrganizationsScreen(model: AppModel, onBack: () -> Unit) {
    val data by model.unlockedData.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var isCreateShown by rememberSaveable { mutableStateOf(false) }
    var refusal by remember { mutableStateOf<Refusal?>(null) }
    var openOrganizationID by rememberSaveable { mutableStateOf<String?>(null) }

    // Offline the list keeps the organizations it last loaded; the Vaults screen already says the app is offline.
    suspend fun load() {
        model.invitations.loadOrganizations()
        model.errorMessage.value = null
    }

    LaunchedEffect(Unit) { load() }

    val shownOrganizationID = openOrganizationID
    if (shownOrganizationID != null) {
        BackHandler { openOrganizationID = null }
        OrganizationScreen(model, shownOrganizationID, onBack = { openOrganizationID = null })
        return
    }

    fun answer(invitation: OrganizationInvitation, isAccepted: Boolean) {
        scope.launch {
            val isAnswered = if (isAccepted) model.invitations.acceptOrganizationInvitation(invitation.id) else model.invitations.declineOrganizationInvitation(invitation.id)
            when {
                !isAnswered -> refusal = model.takeRefusal(if (isAccepted) "Accept" else "Decline")
                isAccepted -> model.showNotice("You joined ${invitation.organizationName}. Its vaults appear once an admin adds you to a team.")
                else -> model.showNotice("Invitation declined.")
            }
        }
    }

    RefreshablePage(bar = { PushedTopBar(if (it) "Organizations" else "", onBack) }, onRefresh = { load() }, largeTitle = "Organizations", bottomInset = TabBarHeight) {
        if (data.allOrganizationInvitation.isNotEmpty()) {
            FormSection(header = "Invitations for you") {
                data.allOrganizationInvitation.forEachIndexed { index, invitation ->
                    if (index > 0) SymbolRowDivider(50.dp)
                    InvitationRow(
                        text = invitation.message,
                        email = invitation.invitedByEmail,
                        icon = Icons.Filled.Business,
                        testTagPrefix = "",
                        testTagName = "organization-invitation",
                        onDecline = { answer(invitation, isAccepted = false) },
                        onAccept = { answer(invitation, isAccepted = true) },
                    )
                }
            }
        }
        if (data.allOrganization.isNotEmpty()) {
            SectionCard {
                data.allOrganization.forEachIndexed { index, organization ->
                    if (index > 0) SymbolRowDivider(52.dp)
                    NavigationRow(
                        testTag = "organization-${organization.name}",
                        onClick = { openOrganizationID = organization.id },
                        leading = { RowSymbol(Icons.Outlined.Business, tint = HisarTheme.colors.tint) },
                    ) {
                        TitleAndSubtitle(organization.name, "${OrganizationRole.describe(organization.role)} · ${OrganizationLogic.describeMemberCount(organization.memberCount)}")
                    }
                }
            }
        }
        FormSection(footer = { Text("An organization shares vaults through its teams instead of person by person. You become its first admin.") }) {
            FormButtonRow("Create organization…", { isCreateShown = true }, testTag = "create-organization", icon = Icons.Outlined.AddCircleOutline, color = HisarTheme.colors.tint)
        }
    }

    if (isCreateShown) {
        CreateOrganizationSheet(model, onDismiss = { isCreateShown = false }) { organization -> openOrganizationID = organization }
    }
    RefusalAlert(refusal) { refusal = null }
}

/** A new organization's name; creating it opens it. */
@Composable
private fun CreateOrganizationSheet(model: AppModel, onDismiss: () -> Unit, onCreated: (String) -> Unit) {
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val focusRequester = remember { FocusRequester() }
    var name by rememberSaveable { mutableStateOf("") }
    val canCreate = OrganizationLogic.isValidName(name) && !isBusy

    LaunchedEffect(Unit) {
        model.errorMessage.value = null
        focusRequester.requestFocus()
    }

    fun create() {
        if (!canCreate) return
        scope.launch {
            model.organizations.createOrganization(name)?.let { organization ->
                onDismiss()
                onCreated(organization.id)
            }
        }
    }

    FormSheet(
        isBusy = isBusy,
        onDismiss = onDismiss,
        bar = {
            ModalSheetBar("Create organization", onCancel = {
                model.errorMessage.value = null
                onDismiss()
            }) {
                if (isBusy) {
                    CircularProgressIndicator(Modifier.padding(horizontal = 16.dp).size(22.dp), color = HisarTheme.colors.secondaryLabel, strokeWidth = 2.dp)
                } else {
                    BarCapsuleButton("Create", ::create, testTag = "new-organization-create", enabled = canCreate, isBold = true)
                }
            }
        },
    ) {
        errorMessage?.let { FormSection { ErrorBanner(it) } }
        FormSection(footer = { Text("1 to 100 characters. The Hisar server sees the names of organizations and teams, but never the names of their vaults.") }) {
            FormTextField(
                name,
                { name = it },
                placeholder = "Acme",
                testTag = "new-organization-name",
                modifier = Modifier.focusRequester(focusRequester),
                keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Words, imeAction = ImeAction.Done),
                keyboardActions = KeyboardActions(onDone = { create() }),
            )
        }
    }
}

/** The dialogs the organization screen opens; one at a time. */
private sealed interface OrganizationDialog {
    data object NewTeam : OrganizationDialog

    data class RenameTeam(val team: Team) : OrganizationDialog

    data object RenameOrganization : OrganizationDialog

    data class CancelInvitation(val invitation: SentOrganizationInvitation) : OrganizationDialog

    data class RemoveMember(val member: Member) : OrganizationDialog

    data class RemoveTeamMember(val team: Team, val member: TeamMember) : OrganizationDialog

    data class TakeVault(val team: Team, val vault: TeamVault) : OrganizationDialog

    data class ChangeRole(val member: Member, val role: String) : OrganizationDialog

    data class DeleteTeam(val team: Team) : OrganizationDialog

    data object DeleteOrganization : OrganizationDialog

    data object Leave : OrganizationDialog
}

/** A long-pressed row's menu: which row, so one menu shows at a time. */
private sealed interface RowMenu {
    data class OfMember(val accountID: String) : RowMenu

    data class OfTeamMember(val teamID: String, val accountID: String) : RowMenu

    data class OfTeamVault(val teamID: String, val vaultID: String) : RowMenu

    data class AddMember(val teamID: String) : RowMenu

    data class TeamActions(val teamID: String) : RowMenu

    data object OrganizationActions : RowMenu
}

/**
 * One organization: its members, invitations and teams, and for admins every change to them. It goes back only when the
 * user is no longer in the organization; any other failure keeps it and says why. The iPhone's OrganizationView.
 */
@Composable
private fun OrganizationScreen(model: AppModel, organizationID: String, onBack: () -> Unit) {
    val data by model.unlockedData.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val remoteChangeCount by model.remoteChangeCount.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val focusManager = LocalFocusManager.current
    var organization by remember { mutableStateOf<Organization?>(null) }
    var refusal by remember { mutableStateOf<Refusal?>(null) }
    var inviteEmail by rememberSaveable { mutableStateOf("") }
    var inviteRole by rememberSaveable { mutableStateOf(OrganizationRole.MEMBER) }
    var sentEmail by rememberSaveable { mutableStateOf<String?>(null) }
    var isSending by remember { mutableStateOf(false) }
    var dialog by remember { mutableStateOf<OrganizationDialog?>(null) }
    var rowMenu by remember { mutableStateOf<RowMenu?>(null) }
    var teamGivenVault by remember { mutableStateOf<Team?>(null) }
    var openMemberID by rememberSaveable { mutableStateOf<String?>(null) }
    var roleEdit by remember { mutableStateOf<TeamVaultRoleEdit?>(null) }

    val name = organization?.name ?: data.allOrganization.firstOrNull { it.id == organizationID }?.name ?: "Organization"
    val isAdmin = organization?.isAdmin ?: false
    val trimmedEmail = inviteEmail.trim().lowercase()
    val canSend = "@" in trimmedEmail && !isSending

    suspend fun load() {
        when (val loaded = model.organizations.loadOrganizationScreen(organizationID)) {
            is OrganizationLoad.Loaded -> {
                organization = loaded.organization
                roleEdit = roleEdit?.refreshed(loaded.organization)
                if (openMemberID != null && loaded.organization.allMember.none { it.accountID == openMemberID }) {
                    openMemberID = null
                    model.showNotice("This person is no longer a member.")
                }
            }
            OrganizationLoad.Gone -> onBack()
            OrganizationLoad.Failed -> refusal = model.takeRefusal(name)
        }
    }

    suspend fun change(title: String, operation: suspend () -> Boolean): Boolean {
        if (!operation()) {
            refusal = model.takeRefusal(title)
            return false
        }
        load()
        model.invitations.loadOrganizations()
        return true
    }

    // Deleting or leaving ends the screen, so it goes back instead of loading an organization the user is no longer in.
    fun finishRemoval(title: String, operation: suspend () -> Boolean) {
        scope.launch { if (operation()) onBack() else refusal = model.takeRefusal(title) }
    }

    fun launchChange(title: String, operation: suspend () -> Boolean) {
        scope.launch { change(title, operation) }
    }

    LaunchedEffect(remoteChangeCount) { load() }

    val shownMember = organization?.allMember?.firstOrNull { it.accountID == openMemberID }
    if (shownMember != null) {
        BackHandler { openMemberID = null }
        organization?.let { OrganizationMemberScreen(model, organizationID, name, shownMember, isAdmin, it, onBack = { openMemberID = null }, reload = { load() }) }
        return
    }

    fun send() {
        if (!canSend) return
        val invitedEmail = trimmedEmail
        val role = inviteRole
        isSending = true
        scope.launch {
            try {
                if (change("Send invitation") { model.organizations.inviteToOrganization(organizationID, invitedEmail, role) }) {
                    sentEmail = invitedEmail
                    inviteEmail = ""
                    focusManager.clearFocus()
                }
            } finally {
                isSending = false
            }
        }
    }

    fun setRole(member: Member, role: String) {
        dialog = OrganizationDialog.ChangeRole(member, role)
    }

    fun removeFromTeam(member: TeamMember, team: Team) {
        dialog = OrganizationDialog.RemoveTeamMember(team, member)
    }

    fun takeBack(vault: TeamVault, team: Team) {
        dialog = OrganizationDialog.TakeVault(team, vault)
    }

    RefreshablePage(
        bar = { isLargeTitleScrolledAway ->
            PushedTopBar(if (isLargeTitleScrolledAway) name else "", onBack) {
                if (isAdmin) {
                    Box {
                        ToolbarIconButton(Icons.Outlined.MoreHoriz, "Organization actions", testTag = "organization-actions") { rowMenu = RowMenu.OrganizationActions }
                        HisarMenu(rowMenu == RowMenu.OrganizationActions, onDismiss = { rowMenu = null }) {
                            HisarMenuItem("Rename organization…", "rename-organization", icon = Icons.Outlined.Edit, onClick = {
                                rowMenu = null
                                dialog = OrganizationDialog.RenameOrganization
                            })
                            HisarMenuItem("Delete organization…", "delete-organization", icon = Icons.Outlined.Delete, isDestructive = true, onClick = {
                                rowMenu = null
                                dialog = OrganizationDialog.DeleteOrganization
                            })
                        }
                    }
                }
            }
        },
        onRefresh = { load() },
        bottomInset = TabBarHeight,
        largeTitle = name,
    ) {
        val loaded = organization
        if (loaded == null) {
            FormSection { LoadingRow("Loading $name…") }
            return@RefreshablePage
        }
        FormSection(footer = {
            Text(if (loaded.isAdmin) ADMIN_ROLE_NOTE else "Members get the vaults of the teams they are in. Admins of ${loaded.name} manage its teams.")
        }) {
            Box(Modifier.testTag("organization-role")) { LabeledRow("Your role", OrganizationRole.describe(loaded.role)) }
        }
        if (loaded.isAdmin) {
            FormSection(header = "Invite by email", footer = { Text("They accept in Hisar, or create an account from the email first. Every address gets the same answer.") }) {
                FormTextField(
                    inviteEmail,
                    { inviteEmail = it },
                    placeholder = "name@example.com",
                    testTag = "organization-invite-email",
                    keyboardOptions = KeyboardOptions(
                        capitalization = KeyboardCapitalization.None,
                        autoCorrectEnabled = false,
                        keyboardType = KeyboardType.Email,
                        imeAction = ImeAction.Send,
                    ),
                    keyboardActions = KeyboardActions(onSend = { send() }),
                )
                RowDivider()
                PickerRow("Role", inviteRole, OrganizationRole.all, OrganizationRole::describe, { inviteRole = it }, testTag = "organization-invite-role")
                RowDivider()
                SendRow("Send invitation", ::send, enabled = canSend, isSending = isSending, testTag = "organization-invite-submit")
                sentEmail?.let { sent ->
                    RowDivider()
                    SentNote(sent, testTag = "organization-invite-sent")
                }
            }
        }
        SectionCard(
            header = OrganizationLogic.describeMemberCount(loaded.allMember.size),
            footer = { Text("Tap a person to see their security code. Ask them for the code Hisar shows for their own account in Settings → People, the row marked you. It must match the code you see for them.") },
        ) {
            loaded.allMember.forEachIndexed { index, member ->
                if (index > 0) SymbolRowDivider()
                val allAction = OrganizationLogic.listAllMemberAction(loaded, member)
                val row = @Composable {
                    MemberRow(
                        member,
                        OrganizationRole.describe(member.role),
                        testTag = "organization-member-${member.email}",
                        onClick = { openMemberID = member.accountID },
                        onLongClick = if (allAction.isEmpty()) null else ({ rowMenu = RowMenu.OfMember(member.accountID) }),
                    ) {
                        HisarMenu(rowMenu == RowMenu.OfMember(member.accountID), onDismiss = { rowMenu = null }) {
                            for (action in allAction) {
                                when (action) {
                                    OrganizationMemberAction.MAKE_ADMIN -> HisarMenuItem("Make admin…", "make-admin-${member.email}", icon = Icons.Outlined.VerifiedUser, onClick = {
                                        rowMenu = null
                                        setRole(member, OrganizationRole.ADMIN)
                                    })
                                    OrganizationMemberAction.MAKE_MEMBER -> HisarMenuItem("Make member…", "make-member-${member.email}", icon = Icons.Outlined.Person, onClick = {
                                        rowMenu = null
                                        setRole(member, OrganizationRole.MEMBER)
                                    })
                                    OrganizationMemberAction.REMOVE -> HisarMenuItem(
                                        "Remove from ${loaded.name}…", "remove-${member.email}", icon = Icons.Outlined.PersonRemove, isDestructive = true,
                                        onClick = {
                                            rowMenu = null
                                            dialog = OrganizationDialog.RemoveMember(member)
                                        },
                                    )
                                }
                            }
                        }
                    }
                }
                if (OrganizationMemberAction.REMOVE in allAction) {
                    SwipeActionRow(
                        "Remove", Icons.Outlined.PersonRemove, "remove-organization-member-${member.email}", isLeading = false,
                        onAction = { dialog = OrganizationDialog.RemoveMember(member) }, actionColor = HisarTheme.colors.destructive, content = row,
                    )
                } else {
                    row()
                }
            }
        }
        if (loaded.isAdmin && loaded.allInvitation.isNotEmpty()) {
            SectionCard(header = "Invited") {
                loaded.allInvitation.forEachIndexed { index, invitation ->
                    if (index > 0) SymbolRowDivider(52.dp)
                    SwipeActionRow(
                        "Cancel", Icons.Outlined.Cancel, "cancel-organization-invitation-${invitation.email}", isLeading = false,
                        onAction = { dialog = OrganizationDialog.CancelInvitation(invitation) }, actionColor = HisarTheme.colors.destructive,
                    ) {
                        InvitedRow(
                            invitation.email, "Waiting for them to accept", OrganizationRole.describe(invitation.role), Icons.Outlined.Email,
                            testTag = "organization-invitation-${invitation.email}",
                            onCancel = { dialog = OrganizationDialog.CancelInvitation(invitation) },
                            cancelLabel = "Cancel invitation",
                        )
                    }
                }
            }
        }
        for (team in loaded.allTeam) {
            TeamSection(
                team = team,
                organization = loaded,
                allGivableVault = OrganizationLogic.listAllGivableVault(loaded, team, data.allVault),
                rowMenu = rowMenu,
                onMenu = { rowMenu = it },
                onRemoveMember = { removeFromTeam(it, team) },
                onTakeBack = { takeBack(it, team) },
                onChangeRole = { vault -> roleEdit = TeamVaultRoleEdit(organizationID, team.id, team.name, vault.vaultID, vault.displayName, vault.role) },
                onAddMember = { member -> launchChange("Add member") { model.organizations.addTeamMember(organizationID, team.id, member.accountID) } },
                onGiveVault = { teamGivenVault = team },
                onRename = { dialog = OrganizationDialog.RenameTeam(team) },
                onDelete = { dialog = OrganizationDialog.DeleteTeam(team) },
            )
        }
        if (loaded.isAdmin) {
            FormSection(footer = if (loaded.allTeam.isEmpty()) ({ Text("Teams get the organization's vaults. Create one, add members and give it a vault.") }) else null) {
                FormButtonRow("New team…", { dialog = OrganizationDialog.NewTeam }, testTag = "new-team", icon = Icons.Outlined.Groups, color = HisarTheme.colors.tint)
            }
        }
        FormSection {
            DestructiveButtonRow("Leave ${loaded.name}…", Icons.AutoMirrored.Outlined.Logout, { dialog = OrganizationDialog.Leave }, testTag = "leave-organization")
        }
    }
    roleEdit?.let { edit ->
        organization?.let { current ->
            ChangeTeamVaultRoleSheet(model, edit, current, onFinish = { load() }, onRefused = { refusal = it }, onDismiss = { roleEdit = null })
        }
    }

    when (val current = dialog) {
        is OrganizationDialog.TakeVault -> OrganizationAccessConfirmation(
            title = "Take ${current.vault.displayName} back from ${current.team.name}?",
            message = "Review who loses access or has a reduced role through the remaining teams.",
            consequences = organization?.let { OrganizationLogic.accessConsequences(it, OrganizationAccessChange.TakeVault(current.team.id, current.vault.vaultID)) }.orEmpty(),
            confirmLabel = "Take back", confirmTestTag = "confirm-take-back",
            onConfirm = { launchChange("Take back") { model.organizations.takeVaultFromTeam(organizationID, current.team.id, current.vault.vaultID) } }, onDismiss = { dialog = null },
        )
        is OrganizationDialog.RemoveTeamMember -> OrganizationAccessConfirmation(
            title = "Remove ${current.member.displayName} from ${current.team.name}?",
            message = if (current.member.isCurrentUser) "You lose access to vaults this team alone gives you." else "Their vault access changes only where no other team gives the same role.",
            consequences = organization?.let { OrganizationLogic.accessConsequences(it, OrganizationAccessChange.RemoveTeamMember(current.team.id, current.member.accountID)) }.orEmpty(),
            confirmLabel = "Remove from ${current.team.name}", confirmTestTag = "confirm-remove-team-member",
            onConfirm = { launchChange("Remove") { model.organizations.removeTeamMember(organizationID, current.team.id, current.member.accountID) } }, onDismiss = { dialog = null },
        )
        is OrganizationDialog.ChangeRole -> OrganizationRoleConfirmation(current.member, name,
            onConfirm = { launchChange(if (current.role == OrganizationRole.ADMIN) "Make admin" else "Make member") { model.organizations.setOrganizationMemberRole(organizationID, current.member.accountID, current.role) } },
            onDismiss = { dialog = null })
        OrganizationDialog.NewTeam -> NameDialog(
            title = "New team",
            message = "A name of 1 to 100 characters, unique in $name.",
            initialName = "",
            placeholder = "Team name",
            fieldTestTag = "new-team-name",
            confirmLabel = "Create",
            confirmTestTag = "new-team-create",
            onDismiss = { dialog = null },
        ) { teamName -> launchChange("New team") { model.organizations.createTeam(organizationID, teamName) != null } }
        is OrganizationDialog.RenameTeam -> NameDialog(
            title = "Rename team",
            message = null,
            initialName = current.team.name,
            placeholder = "Team name",
            fieldTestTag = "rename-team-name",
            confirmLabel = "Rename",
            confirmTestTag = "rename-team-confirm",
            onDismiss = { dialog = null },
        ) { teamName -> launchChange("Rename team") { model.organizations.renameTeam(organizationID, current.team.id, teamName) } }
        OrganizationDialog.RenameOrganization -> NameDialog(
            title = "Rename organization",
            message = "Everyone in $name sees the new name.",
            initialName = name,
            placeholder = "Organization name",
            fieldTestTag = "rename-organization-name",
            confirmLabel = "Rename",
            confirmTestTag = "rename-organization-confirm",
            onDismiss = { dialog = null },
        ) { organizationName -> launchChange("Rename organization") { model.organizations.renameOrganization(organizationID, organizationName) } }
        is OrganizationDialog.CancelInvitation -> ConfirmationDialog(
            title = "Cancel the invitation to ${current.invitation.email}?",
            message = null,
            confirmLabel = "Cancel invitation",
            confirmTestTag = "confirm-cancel-organization-invitation",
            onConfirm = { launchChange("Cancel invitation") { model.organizations.cancelOrganizationInvitation(organizationID, current.invitation.id) } },
            onDismiss = { dialog = null },
            dismissLabel = "Keep invitation",
        )
        is OrganizationDialog.RemoveMember -> OrganizationAccessConfirmation(
            title = "Remove ${current.member.name} from $name?",
            message = describeRemoval(current.member.name, name),
            consequences = organization?.let { OrganizationLogic.accessConsequences(it, OrganizationAccessChange.RemoveMember(current.member.accountID)) }.orEmpty(),
            confirmLabel = "Remove from $name",
            confirmTestTag = "confirm-remove-organization-member",
            onConfirm = { launchChange("Remove from $name") { model.organizations.removeOrganizationMember(organizationID, current.member.accountID) } },
            onDismiss = { dialog = null },
        )
        is OrganizationDialog.DeleteTeam -> OrganizationAccessConfirmation(
            title = "Delete team ${current.team.name}?",
            message = "Its members lose the vaults they have only through ${current.team.name}.",
            consequences = organization?.let { OrganizationLogic.accessConsequences(it, OrganizationAccessChange.RemoveTeam(current.team.id)) }.orEmpty(),
            confirmLabel = "Delete team",
            confirmTestTag = "confirm-delete-team",
            onConfirm = { launchChange("Delete team") { model.organizations.deleteTeam(organizationID, current.team.id) } },
            onDismiss = { dialog = null },
        )
        OrganizationDialog.DeleteOrganization -> ConfirmationDialog(
            title = "Delete $name?",
            message = "Its teams and pending invitations are deleted with it.",
            confirmLabel = "Delete organization",
            confirmTestTag = "confirm-delete-organization",
            onConfirm = { finishRemoval("Delete organization") { model.organizations.deleteOrganization(organizationID) } },
            onDismiss = { dialog = null },
        )
        OrganizationDialog.Leave -> OrganizationAccessConfirmation(
            title = "Leave $name?",
            message = "You lose the vaults you have through $name's teams.",
            consequences = organization?.let { org -> org.allMember.firstOrNull { it.isCurrentUser }?.let { OrganizationLogic.accessConsequences(org, OrganizationAccessChange.RemoveMember(it.accountID)) } }.orEmpty(),
            confirmLabel = "Leave $name",
            confirmTestTag = "confirm-leave-organization",
            onConfirm = { finishRemoval("Leave $name") { model.organizations.leaveOrganization(organizationID) } },
            onDismiss = { dialog = null },
        )
        null -> Unit
    }
    val givenTeam = teamGivenVault
    val loaded = organization
    if (givenTeam != null && loaded != null) {
        GiveVaultSheet(
            model = model,
            title = "Give a vault to ${givenTeam.name}",
            choiceTitle = "Vault",
            allChoice = OrganizationLogic.listAllGivableVault(loaded, givenTeam, data.allVault).map {
                GiveVaultChoice(id = it.vaultID, label = it.displayName, teamID = givenTeam.id, vaultID = it.vaultID)
            },
            organizationID = organizationID,
            onFinish = { load() },
            onDismiss = { teamGivenVault = null },
        )
    }
    RefusalAlert(refusal) { refusal = null }
    if (isBusy) BackHandler {}
}

/** One team: its members and vaults, and for admins Add member, Give a vault… and the team's menu. */
@Composable
private fun TeamSection(
    team: Team,
    organization: Organization,
    allGivableVault: List<TeamVault>,
    rowMenu: RowMenu?,
    onMenu: (RowMenu?) -> Unit,
    onRemoveMember: (TeamMember) -> Unit,
    onTakeBack: (TeamVault) -> Unit,
    onAddMember: (Member) -> Unit,
    onGiveVault: () -> Unit,
    onChangeRole: (TeamVault) -> Unit,
    onRename: () -> Unit,
    onDelete: () -> Unit,
) {
    val isAdmin = organization.isAdmin
    val allAddableMember = OrganizationLogic.listAllAddableMember(organization, team)
    val hasRows = isAdmin || team.allMember.isNotEmpty() || team.allVault.isNotEmpty()
    SectionCard(
        header = "Team ${team.name}",
        headerTestTag = "team-${team.name}",
        headerAction = if (!isAdmin) null else ({
            Box {
                ToolbarIconButtonSmall("Team actions", "team-${team.name}-actions") { onMenu(RowMenu.TeamActions(team.id)) }
                HisarMenu(rowMenu == RowMenu.TeamActions(team.id), onDismiss = { onMenu(null) }) {
                    HisarMenuItem("Rename…", "rename-team-${team.name}", icon = Icons.Outlined.Edit, onClick = {
                        onMenu(null)
                        onRename()
                    })
                    HisarMenuItem("Delete team…", "delete-team-${team.name}", icon = Icons.Outlined.Delete, isDestructive = true, onClick = {
                        onMenu(null)
                        onDelete()
                    })
                }
            }
        }),
        footer = if (team.allMember.isEmpty() && team.allVault.isEmpty()) ({ Text("No members or vaults yet.") }) else null,
        content = if (!hasRows) null else ({
            var isFirst = true
            val divider = @Composable {
                if (!isFirst) SymbolRowDivider(52.dp)
                isFirst = false
            }
            for (member in team.allMember) {
                divider()
                val menu = RowMenu.OfTeamMember(team.id, member.accountID)
                val row = @Composable {
                    LabelRow(
                        if (member.isCurrentUser) Icons.Filled.AccountCircle else Icons.Outlined.AccountCircle,
                        HisarTheme.colors.secondaryLabel,
                        testTag = "team-${team.name}-member-${member.email}",
                        onLongClick = if (isAdmin) ({ onMenu(menu) }) else null,
                        menu = {
                            HisarMenu(rowMenu == menu, onDismiss = { onMenu(null) }) {
                                HisarMenuItem("Remove", "remove-team-member-${member.email}", icon = Icons.Outlined.PersonRemove, isDestructive = true, onClick = {
                                    onMenu(null)
                                    onRemoveMember(member)
                                })
                            }
                        },
                    ) { TitleAndDetail(member.displayName, member.email) }
                }
                if (isAdmin) {
                    SwipeActionRow(
                        "Remove", Icons.Outlined.PersonRemove, "team-${team.name}-remove-${member.email}", isLeading = false,
                        onAction = { onRemoveMember(member) }, actionColor = HisarTheme.colors.destructive, content = row,
                    )
                } else {
                    row()
                }
            }
            for (vault in team.allVault) {
                divider()
                val menu = RowMenu.OfTeamVault(team.id, vault.vaultID)
                val row = @Composable {
                    LabelRow(
                        if (vault.canOpen) Icons.Outlined.Lock else Icons.Outlined.NoEncryption,
                        HisarTheme.colors.secondaryLabel,
                        testTag = "team-${team.name}-vault-${vault.displayName}",
                        onClick = if (isAdmin) ({ onChangeRole(vault) }) else null,
                        onLongClick = if (isAdmin) ({ onMenu(menu) }) else null,
                        menu = {
                            HisarMenu(rowMenu == menu, onDismiss = { onMenu(null) }) {
                                HisarMenuItem("Change role", "change-role-${vault.displayName}", onClick = {
                                    onMenu(null)
                                    onChangeRole(vault)
                                })
                                HisarMenuItem("Take back…", "take-back-${vault.displayName}", icon = Icons.AutoMirrored.Outlined.Undo, isDestructive = true, onClick = {
                                    onMenu(null)
                                    onTakeBack(vault)
                                })
                            }
                        },
                        trailing = { Text(VaultLogic.describeVaultRole(vault.role), color = HisarTheme.colors.secondaryLabel) },
                    ) {
                        Text(vault.displayName, color = if (vault.canOpen) HisarTheme.colors.label else HisarTheme.colors.secondaryLabel)
                    }
                }
                if (isAdmin) {
                    SwipeActionRow(
                        "Take back…", Icons.AutoMirrored.Outlined.Undo, "team-${team.name}-take-back-${vault.displayName}", isLeading = false,
                        onAction = { onTakeBack(vault) }, actionColor = HisarTheme.colors.destructive, content = row,
                    )
                } else {
                    row()
                }
            }
            if (isAdmin) {
                divider()
                Box {
                    FormButtonRow(
                        "Add member", { onMenu(RowMenu.AddMember(team.id)) }, testTag = "team-${team.name}-add-member",
                        icon = Icons.Outlined.PersonAdd, color = HisarTheme.colors.tint, enabled = allAddableMember.isNotEmpty(),
                    )
                    HisarMenu(rowMenu == RowMenu.AddMember(team.id), onDismiss = { onMenu(null) }) {
                        for (member in allAddableMember) {
                            HisarMenuItem(describeMemberName(member), "team-${team.name}-add-${member.email}", onClick = {
                                onMenu(null)
                                onAddMember(member)
                            })
                        }
                    }
                }
                divider()
                FormButtonRow(
                    "Give a vault…", onGiveVault, testTag = "team-${team.name}-give-vault",
                    icon = Icons.Outlined.AddCircleOutline, color = HisarTheme.colors.tint, enabled = allGivableVault.isNotEmpty(),
                )
            }
        }),
    )
}

/** The team header's menu button, as an icon-only Label with ellipsis.circle. */
@Composable
private fun ToolbarIconButtonSmall(contentDescription: String, testTag: String, onClick: () -> Unit) {
    Box(
        Modifier
            .size(40.dp)
            .clickable(role = Role.Button, onClick = onClick)
            .testTag(testTag),
        contentAlignment = Alignment.Center,
    ) {
        Icon(Icons.Outlined.MoreHoriz, contentDescription = contentDescription, tint = HisarTheme.colors.tint, modifier = Modifier.size(22.dp))
    }
}

/** A text field alert: the iPhone's `.alert` with a TextField, Cancel and the action. */
@Composable
private fun NameDialog(
    title: String,
    message: String?,
    initialName: String,
    placeholder: String,
    fieldTestTag: String,
    confirmLabel: String,
    confirmTestTag: String,
    onDismiss: () -> Unit,
    onConfirm: (String) -> Unit,
) {
    var name by rememberSaveable { mutableStateOf(initialName) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            Column {
                if (message != null) {
                    Text(message)
                    Spacer(Modifier.height(12.dp))
                }
                Box(Modifier.background(HisarTheme.colors.fill, RoundedCornerShape(10.dp))) {
                    FormTextField(name, { name = it }, placeholder = placeholder, testTag = fieldTestTag)
                }
            }
        },
        confirmButton = {
            PlainButton(confirmLabel, {
                onDismiss()
                onConfirm(name)
            }, testTag = confirmTestTag)
        },
        dismissButton = { PlainButton("Cancel", onDismiss, testTag = "$fieldTestTag-cancel") },
        containerColor = HisarTheme.colors.cellBackground,
    )
}

private fun describeRemoval(memberName: String, organizationName: String): String =
    "$memberName loses the vaults they have through $organizationName's teams."

/** A member of the organization: their security code and role, and for admins the role switch and Remove. */
@Composable
private fun OrganizationMemberScreen(
    model: AppModel,
    organizationID: String,
    organizationName: String,
    member: Member,
    isAdmin: Boolean,
    organization: Organization,
    onBack: () -> Unit,
    reload: suspend () -> Unit,
) {
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val role = member.role
    var isRemoveConfirmationShown by remember { mutableStateOf(false) }
    var refusal by remember { mutableStateOf<Refusal?>(null) }
    var isRoleConfirmationShown by remember { mutableStateOf(false) }
    val isAdminRole = role == OrganizationRole.ADMIN

    fun setRole(newRole: String) {
        scope.launch {
            if (model.organizations.setOrganizationMemberRole(organizationID, member.accountID, newRole)) {
                reload()
            } else {
                refusal = model.takeRefusal(if (newRole == OrganizationRole.ADMIN) "Make admin" else "Make member")
                reload()
            }
        }
    }

    RefreshablePage(bar = { PushedTopBar(member.name, onBack) }, onRefresh = reload, bottomInset = TabBarHeight) {
        PersonHeader(member)
        if (member.isKeyChanged) {
            FormSection {
                WarningRow(
                    "${member.name}'s key changed since this device first shared with them, so Hisar stopped sharing with them. Contact them over a channel you trust. Comparing security codes does not restore sharing.",
                    testTag = "organization-member-key-changed",
                )
            }
        }
        SecurityCodeSection(
            member,
            footer = com.tryhisar.hisar.ui.sharing.describeSecurityCode(member),
            testTag = "organization-member-security-code",
        )
        FormSection(footer = { Text(if (isAdminRole) ADMIN_ROLE_NOTE else "Members get the vaults of the teams they are in.") }) {
            Box(Modifier.testTag("organization-member-role")) { LabeledRow("Role", OrganizationRole.describe(role)) }
            if (isAdmin) {
                RowDivider()
                FormButtonRow(
                    if (isAdminRole) "Make member…" else "Make admin…",
                    { isRoleConfirmationShown = true },
                    testTag = "organization-member-toggle-role",
                    icon = if (isAdminRole) Icons.Outlined.Person else Icons.Outlined.VerifiedUser,
                    color = HisarTheme.colors.tint,
                    enabled = !isBusy,
                )
            }
        }
        if (isAdmin && !member.isCurrentUser) {
            FormSection {
                DestructiveButtonRow("Remove from $organizationName…", Icons.Outlined.PersonRemove, { isRemoveConfirmationShown = true }, testTag = "remove-organization-member", enabled = !isBusy)
            }
        }
    }

    if (isRoleConfirmationShown) {
        OrganizationRoleConfirmation(member.copy(role = role), organizationName,
            onConfirm = { setRole(if (isAdminRole) OrganizationRole.MEMBER else OrganizationRole.ADMIN) },
            onDismiss = { isRoleConfirmationShown = false })
    }

    if (isRemoveConfirmationShown) {
        OrganizationAccessConfirmation(
            title = "Remove ${member.name} from $organizationName?",
            message = describeRemoval(member.name, organizationName),
            consequences = OrganizationLogic.accessConsequences(organization, OrganizationAccessChange.RemoveMember(member.accountID)),
            confirmLabel = "Remove from $organizationName",
            confirmTestTag = "confirm-remove-organization-member",
            onConfirm = {
                scope.launch {
                    if (model.organizations.removeOrganizationMember(organizationID, member.accountID)) {
                        reload()
                        onBack()
                    } else {
                        refusal = model.takeRefusal("Remove from $organizationName")
                    }
                }
            },
            onDismiss = { isRemoveConfirmationShown = false },
        )
    }
    RefusalAlert(refusal) { refusal = null }
}

@Composable
private fun OrganizationRoleConfirmation(member: Member, organizationName: String, onConfirm: () -> Unit, onDismiss: () -> Unit) {
    val isDemotion = member.role == OrganizationRole.ADMIN
    ConfirmationDialog(
        title = if (!isDemotion) "Make ${member.name} an admin of $organizationName?" else if (member.isCurrentUser) "Make yourself a member of $organizationName?" else "Make ${member.name} a member of $organizationName?",
        message = if (!isDemotion) "Admins manage members, teams and which teams have which vaults. Being an admin gives no vault by itself."
            else if (member.isCurrentUser) "You can no longer manage members, teams or which teams have which vaults. Another admin can make you an admin again."
            else "${member.name} can no longer manage members, teams or which teams have which vaults. Another admin can make them an admin again.",
        confirmLabel = if (!isDemotion) "Make admin" else if (member.isCurrentUser) "Make me a member" else "Make member",
        confirmTestTag = "confirm-organization-role", onConfirm = onConfirm, onDismiss = onDismiss, isDestructive = isDemotion,
    )
}

@Composable
internal fun OrganizationAccessConfirmation(title: String, message: String, consequences: List<String>, confirmLabel: String, confirmTestTag: String, onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState())) {
                Text(message)
                consequences.forEach { line -> Spacer(Modifier.height(12.dp)); Text(line) }
            }
        },
        confirmButton = { PlainButton(confirmLabel, { onDismiss(); onConfirm() }, testTag = confirmTestTag, color = HisarTheme.colors.destructive) },
        dismissButton = { PlainButton("Cancel", onDismiss, testTag = "cancel-confirmation") },
        containerColor = HisarTheme.colors.cellBackground,
    )
}
