package com.tryhisar.hisar.ui.organizations

import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Undo
import androidx.compose.material.icons.outlined.AddCircleOutline
import androidx.compose.material.icons.outlined.Groups
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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.Refusal
import com.tryhisar.hisar.app.takeRefusal
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.Organization
import com.tryhisar.hisar.model.OrganizationAccessChange
import com.tryhisar.hisar.model.OrganizationLogic
import com.tryhisar.hisar.model.Team
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.sharing.DoneSheetBar
import com.tryhisar.hisar.ui.sharing.FormSheet
import com.tryhisar.hisar.ui.sharing.LabelRow
import com.tryhisar.hisar.ui.sharing.LoadingRow
import com.tryhisar.hisar.ui.sharing.ModalSheetBar
import com.tryhisar.hisar.ui.sharing.RefreshablePage
import com.tryhisar.hisar.ui.sharing.SectionCard
import com.tryhisar.hisar.ui.sharing.SymbolRowDivider
import com.tryhisar.hisar.ui.sharing.TitleAndDetail
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.vaults.BarCapsuleButton
import com.tryhisar.hisar.ui.theme.HisarType
import com.tryhisar.hisar.ui.vaults.HisarMenu
import com.tryhisar.hisar.ui.vaults.HisarMenuItem
import com.tryhisar.hisar.ui.vaults.PickerRow
import com.tryhisar.hisar.ui.vaults.RefusalAlert
import com.tryhisar.hisar.ui.vaults.SwipeActionRow
import kotlinx.coroutines.launch

/** What a Give sheet offers: a vault for a team, or a team for a vault. */
data class GiveVaultChoice(val id: String, val label: String, val teamID: String, val vaultID: String, val canPreselect: Boolean = true)

/**
 * Gives a team an organization vault with a role: from a team (pick the vault) or from the vault's sheet (pick the
 * team). The iPhone's GiveVaultSheet.
 */
@Composable
fun GiveVaultSheet(
    model: AppModel,
    title: String,
    identity: String,
    choiceTitle: String,
    allChoice: List<GiveVaultChoice>,
    organizationID: String,
    onFinish: suspend () -> Unit,
    onDismiss: () -> Unit,
) {
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var choiceID by rememberSaveable { mutableStateOf(allChoice.firstOrNull()?.takeIf { it.canPreselect }?.id.orEmpty()) }
    var role by rememberSaveable { mutableStateOf(Member.VAULT_ROLE_EDITOR) }
    var refusal by remember { mutableStateOf<Refusal?>(null) }
    val choice = allChoice.firstOrNull { it.id == choiceID }

    LaunchedEffect(Unit) { model.errorMessage.value = null }

    fun give() {
        val chosen = choice ?: return
        val selectedRole = role
        scope.launch {
            if (model.organizations.giveVaultToTeam(organizationID, chosen.teamID, chosen.vaultID, selectedRole)) {
                onFinish()
                onDismiss()
            } else {
                refusal = model.takeRefusal(title)
            }
        }
    }

    FormSheet(
        isBusy = isBusy,
        onDismiss = onDismiss,
        bar = {
            ModalSheetBar("", onCancel = onDismiss) {
                if (isBusy) {
                    CircularProgressIndicator(Modifier.padding(horizontal = 16.dp).size(22.dp), color = HisarTheme.colors.secondaryLabel, strokeWidth = 2.dp)
                } else {
                    BarCapsuleButton("Give", ::give, testTag = "give-vault-confirm", enabled = choice != null, isBold = true)
                }
            }
        },
    ) {
        Text(title, style = HisarType.headline, color = HisarTheme.colors.label, modifier = Modifier.testTag("give-vault-heading"))
        Text(identity, style = HisarType.body, color = HisarTheme.colors.label, modifier = Modifier.padding(top = 8.dp, bottom = 16.dp).testTag("give-vault-identity"))
        FormSection(footer = { Text("${VaultLogic.describeTeamVaultRoleAbility(role)} Everyone in the team gets the vault with this role. Someone in several teams gets the strongest of their roles.") }) {
            PickerRow(choiceTitle, choice, allChoice, { it.label }, { choiceID = it.id }, testTag = "give-vault-choice")
            RowDivider()
            PickerRow("Role", role, Member.allVaultRole, VaultLogic::describeVaultRole, { role = it }, testTag = "give-vault-role")
        }
    }
    RefusalAlert(refusal) { refusal = null }
}

/**
 * An organization vault's sheet from its toolbar: the teams that have it, in place of Invite by email and the member
 * list. Admins take it back from a team or give it to another. The iPhone's OrganizationVaultSharingView.
 */
@Composable
fun OrganizationVaultSharingScreen(model: AppModel, vaultID: String, organizationID: String, onDone: () -> Unit) {
    val data by model.unlockedData.collectAsStateWithLifecycle()
    val remoteChangeCount by model.remoteChangeCount.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var organization by remember { mutableStateOf<Organization?>(null) }
    var isGiveShown by remember { mutableStateOf(false) }
    var refusal by remember { mutableStateOf<Refusal?>(null) }
    var teamWithMenu by remember { mutableStateOf<String?>(null) }
    var roleEdit by remember { mutableStateOf<TeamVaultRoleEdit?>(null) }
    var teamToTakeBack by remember { mutableStateOf<Team?>(null) }
    val vaultName = data.findVault(vaultID)?.name ?: "Vault"
    val sharing = organization?.let { OrganizationLogic.describeVaultSharing(it, vaultID) }

    suspend fun load() {
        val loaded = model.organizations.loadOrganization(organizationID)
        when {
            loaded != null -> {
                organization = loaded
                roleEdit = roleEdit?.refreshed(loaded)
            }
            organization == null -> refusal = model.takeRefusal("Shared through teams")
            else -> model.errorMessage.value = null
        }
    }

    fun takeBack(team: Team) {
        teamToTakeBack = team
    }

    LaunchedEffect(remoteChangeCount) { load() }

    RefreshablePage(bar = { DoneSheetBar(vaultName, onDone, doneTestTag = "vault-sharing-done") }, onRefresh = { load() }) {
        if (sharing == null) {
            FormSection { LoadingRow("Loading teams…") }
        } else {
            SectionCard(
                header = "Shared through teams in ${sharing.organizationName}",
                headerTestTag = "shared-through-teams",
                footer = sharing.askAdminNote?.let { note -> { Text(note, Modifier.testTag("ask-an-admin")) } },
                content = if (sharing.allTeamAccess.isEmpty()) null else ({
                    sharing.allTeamAccess.forEachIndexed { index, access ->
                        if (index > 0) SymbolRowDivider(52.dp)
                        val team = access.team
                        val row = @Composable {
                            LabelRow(
                                Icons.Outlined.Groups,
                                HisarTheme.colors.secondaryLabel,
                                testTag = "vault-team-${team.name}",
                                onClick = if (sharing.canChangeTeams) ({ roleEdit = TeamVaultRoleEdit(organizationID, team.id, team.name, vaultID, vaultName, access.role) }) else null,
                                onLongClick = if (sharing.canChangeTeams) ({ teamWithMenu = team.id }) else null,
                                menu = {
                                    HisarMenu(teamWithMenu == team.id, onDismiss = { teamWithMenu = null }) {
                                        HisarMenuItem("Change role", "change-role-${team.name}", onClick = {
                                            teamWithMenu = null
                                            roleEdit = TeamVaultRoleEdit(organizationID, team.id, team.name, vaultID, vaultName, access.role)
                                        })
                                        HisarMenuItem("Take back…", "take-back-${team.name}", icon = Icons.AutoMirrored.Outlined.Undo, isDestructive = true, onClick = {
                                            teamWithMenu = null
                                            takeBack(team)
                                        })
                                    }
                                },
                                trailing = { Text(VaultLogic.describeVaultRole(access.role), color = HisarTheme.colors.secondaryLabel) },
                            ) {
                                TitleAndDetail(team.name, if (team.allMember.isEmpty()) "No members yet" else team.allMember.joinToString(", ") { it.displayName })
                            }
                        }
                        if (sharing.canChangeTeams) {
                            SwipeActionRow(
                                "Take back…", Icons.AutoMirrored.Outlined.Undo, "vault-team-${team.name}-take-back", isLeading = false,
                                onAction = { takeBack(team) }, actionColor = HisarTheme.colors.destructive, content = row,
                            )
                        } else {
                            row()
                        }
                    }
                }),
            )
            if (sharing.canChangeTeams) {
                FormSection {
                    FormButtonRow(
                        "Give to a team…", { isGiveShown = true }, testTag = "give-vault-to-team",
                        icon = Icons.Outlined.AddCircleOutline, color = HisarTheme.colors.tint, enabled = sharing.allTeamWithoutVault.isNotEmpty(),
                    )
                }
            }
        }
    }
    teamToTakeBack?.let { team ->
        organization?.let { loaded ->
            OrganizationAccessConfirmation(
                title = "Take $vaultName back from ${team.name}?",
                message = "Review who loses access or has a reduced role through the remaining teams.",
                consequences = OrganizationLogic.accessConsequences(loaded, OrganizationAccessChange.TakeVault(team.id, vaultID)),
                confirmLabel = "Take back", confirmTestTag = "confirm-take-back",
                onConfirm = { scope.launch { if (model.organizations.takeVaultFromTeam(organizationID, team.id, vaultID)) load() else refusal = model.takeRefusal("Take back") } },
                onDismiss = { teamToTakeBack = null },
            )
        }
    }

    if (isGiveShown && sharing != null) {
        GiveVaultSheet(
            model = model,
            title = "Give to a team",
            identity = "Vault: “$vaultName”\nOrganization: ${sharing.organizationName}",
            choiceTitle = "Team",
            allChoice = sharing.allTeamWithoutVault.map { GiveVaultChoice(id = it.id, label = it.name, teamID = it.id, vaultID = vaultID) },
            organizationID = organizationID,
            onFinish = { load() },
            onDismiss = { isGiveShown = false },
        )
    }
    roleEdit?.let { edit ->
        organization?.let { current ->
            ChangeTeamVaultRoleSheet(model, edit, current, onFinish = { load() }, onRefused = { refusal = it }, onDismiss = { roleEdit = null })
        }
    }
    RefusalAlert(refusal) { refusal = null }
}

data class TeamVaultRoleEdit(val organizationID: String, val teamID: String, val teamName: String, val vaultID: String, val vaultName: String, val role: String) {
    fun refreshed(organization: Organization): TeamVaultRoleEdit? {
        if (!organization.isAdmin) return null
        val team = organization.allTeam.firstOrNull { it.id == teamID } ?: return null
        val vault = team.allVault.firstOrNull { it.vaultID == vaultID } ?: return null
        return copy(teamName = team.name, vaultName = vault.displayName, role = vault.role)
    }
}

@Composable
fun ChangeTeamVaultRoleSheet(model: AppModel, edit: TeamVaultRoleEdit, organization: Organization, onFinish: suspend () -> Unit, onRefused: (Refusal?) -> Unit, onDismiss: () -> Unit) {
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var role by rememberSaveable(edit.teamID, edit.vaultID) { mutableStateOf(edit.role) }
    var isOwnerConfirmationShown by remember { mutableStateOf(false) }
    var isSubmitting by remember { mutableStateOf(false) }

    LaunchedEffect(edit.role, isSubmitting) {
        if (!isSubmitting) role = edit.role
    }

    fun save() {
        if (isBusy || isSubmitting || role == edit.role) return
        isSubmitting = true
        scope.launch {
            val succeeded = model.organizations.giveVaultToTeam(edit.organizationID, edit.teamID, edit.vaultID, role)
            if (!succeeded) onRefused(model.takeRefusal("Change role"))
            onFinish()
            isSubmitting = false
            if (succeeded) onDismiss()
        }
    }

    FormSheet(
        isBusy = isBusy || isSubmitting,
        onDismiss = onDismiss,
        bar = {
            ModalSheetBar("Change role", onCancel = onDismiss) {
                BarCapsuleButton("Save", {
                    if (role == Member.VAULT_ROLE_OWNER || edit.role == Member.VAULT_ROLE_OWNER) isOwnerConfirmationShown = true else save()
                }, testTag = "save-team-role", enabled = role != edit.role && !isBusy && !isSubmitting, isBold = true)
            }
        },
    ) {
        FormSection(footer = { Text("${VaultLogic.describeTeamVaultRoleAbility(role)} Someone in several teams gets the strongest of their roles. Newly entitled members wait for an owner's Hisar to give them their keys.") }) {
            TitleAndDetail(edit.teamName, edit.vaultName)
            PickerRow("Role", role, Member.allVaultRole, VaultLogic::describeVaultRole, { role = it }, testTag = "change-team-role", enabled = !isBusy && !isSubmitting)
        }
    }
    if (isOwnerConfirmationShown) {
        val isPromotion = role == Member.VAULT_ROLE_OWNER
        com.tryhisar.hisar.ui.settings.ConfirmationDialog(
            title = if (isPromotion) "Make ${edit.teamName} an owner of “${edit.vaultName}”?" else "Change ${edit.teamName}'s role from Owner to ${VaultLogic.describeVaultRole(role)}?",
            message = (if (isPromotion) {
                "Owners view and change items, share, rename and delete the vault. Access is managed through the organization's teams. Someone in several teams gets the strongest of their roles. Newly entitled members wait for an owner's Hisar to give them their keys."
            } else {
                "${edit.teamName}'s members lose Owner powers unless another team gives them Owner. Someone in several teams gets the strongest of their roles. Access is managed through the organization's teams."
            }) + "\n\n" + OrganizationLogic.accessConsequences(organization, com.tryhisar.hisar.model.OrganizationAccessChange.TeamRole(edit.teamID, edit.vaultID, role)).joinToString("\n\n"),
            confirmLabel = if (isPromotion) "Make owner" else "Change role",
            confirmTestTag = "confirm-team-role",
            onConfirm = ::save,
            onDismiss = { isOwnerConfirmationShown = false },
            isDestructive = !isPromotion,
        )
    }
}
