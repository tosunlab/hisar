package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.AccountDeletionPreview
import com.tryhisar.hisar.model.DeletionOrganization
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.OrganizationAccessChange
import com.tryhisar.hisar.model.Organization
import com.tryhisar.hisar.model.OrganizationInvitation
import com.tryhisar.hisar.model.OrganizationLoad
import com.tryhisar.hisar.model.OrganizationLogic
import com.tryhisar.hisar.model.OrganizationMemberAction
import com.tryhisar.hisar.model.OrganizationRole
import com.tryhisar.hisar.model.Organizations
import com.tryhisar.hisar.model.SentOrganizationInvitation
import com.tryhisar.hisar.model.Team
import com.tryhisar.hisar.model.TeamMember
import com.tryhisar.hisar.model.TeamVault
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.WaitingVaultKey
import com.tryhisar.hisar.model.WaitingVaultKeys
import com.tryhisar.hisar.model.backendJson
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runTest

/** The iPhone app's OrganizationTests that the Vaults lane left: the Settings → Organizations screens' rules. */
class OrganizationTests {
    private val alice = Member("alice-id", "Alice", "alice@example.com", OrganizationRole.ADMIN, "1111 2222", isCurrentUser = true)
    private val bob = Member("bob-id", "Bob", "bob@example.com", OrganizationRole.MEMBER, "3333 4444")
    private val carol = Member("carol-id", "Carol", "carol@example.com", OrganizationRole.ADMIN, "5555 6666")

    private fun buildAcme(isAdmin: Boolean): Organization {
        val platform = Team(
            id = "platform", name = "Platform",
            allMember = listOf(TeamMember("alice-id", "Alice", "alice@example.com", isCurrentUser = true), TeamMember("bob-id", "Bob", "bob@example.com")),
            allVault = listOf(TeamVault("deploy", "Deploy", Member.VAULT_ROLE_EDITOR, canOpen = true)),
        )
        val audit = Team(
            id = "audit", name = "Audit",
            allMember = listOf(TeamMember("carol-id", "Carol", "carol@example.com")),
            allVault = listOf(TeamVault("deploy", "Deploy", Member.VAULT_ROLE_READER, canOpen = true), TeamVault("ledger", "", Member.VAULT_ROLE_OWNER, canOpen = false)),
        )
        val role = if (isAdmin) OrganizationRole.ADMIN else OrganizationRole.MEMBER
        return Organization(
            id = "acme", name = "Acme", role = role, isAdmin = isAdmin,
            allMember = listOf(carol, alice.copy(role = role), bob), allTeam = listOf(audit, platform),
            allInvitation = if (isAdmin) listOf(SentOrganizationInvitation("invitation-1", "dave@example.com", OrganizationRole.MEMBER, "", "")) else emptyList(),
        )
    }

    @Test
    fun testRemovingTeamMembershipPreviewsOnlyThatPersonsLosses() {
        val original = buildAcme(isAdmin = true)
        val organization = original.copy(allTeam = listOf(original.allTeam[0].copy(
            allMember = original.allTeam[0].allMember + TeamMember("bob-id", "Bob", "bob@example.com"),
        ), original.allTeam[1]))
        assertEquals(listOf("Deploy: Bob's role changes from Editor to Reader."),
            OrganizationLogic.accessConsequences(organization, OrganizationAccessChange.RemoveTeamMember("platform", "bob-id")))
        assertEquals(listOf("Deploy: You lose access too."),
            OrganizationLogic.accessConsequences(organization, OrganizationAccessChange.RemoveTeamMember("platform", "alice-id")))
    }

    @Test
    fun testTeamRoleChangeKeepsTheStrongestOtherGrant() {
        val original = buildAcme(isAdmin = true)
        val audit = original.allTeam[0]
        val organization = original.copy(allTeam = listOf(audit.copy(
            allMember = audit.allMember + TeamMember("bob-id", "Bob", "bob@example.com"),
            allVault = audit.allVault.map { if (it.vaultID == "deploy") it.copy(role = "owner") else it },
        ), original.allTeam[1]))
        val lines = OrganizationLogic.accessConsequences(organization, OrganizationAccessChange.TeamRole("audit", "deploy", "reader"))
        assertTrue(lines.joinToString().contains("Bob's role changes from Owner to Editor."))
        assertFalse(lines.joinToString().contains("Alice"))
    }

    @Test
    fun testDecodesWhatCoreSends() {
        val organizationJSON = """
            {"id":"acme","name":"Acme","role":"member","isAdmin":false,
             "members":[{"accountId":"bob-id","name":"Bob","email":"bob@example.com","role":"admin","securityCode":"3333 4444","isCurrentUser":false,"isKeyChanged":true}],
             "teams":[{"id":"platform","name":"Platform","members":[{"accountId":"alice-id","name":"Alice","email":"alice@example.com","isCurrentUser":true}],
                       "vaults":[{"vaultId":"ledger","name":"","role":"owner","canOpen":false}]}],
             "invitations":[]}
        """
        val organization = backendJson.decodeFromString(Organization.serializer(), organizationJSON)
        assertEquals(true, organization.allMember.firstOrNull()?.isKeyChanged)
        assertEquals("3333 4444", organization.allMember.firstOrNull()?.securityCode)
        assertEquals(true, organization.allTeam.firstOrNull()?.hasCurrentUser)
        assertEquals("A vault you can't open", organization.allTeam.firstOrNull()?.allVault?.firstOrNull()?.displayName)

        val vault = backendJson.decodeFromString(
            VaultSummary.serializer(),
            """{"id":"ledger","name":"Ledger","type":"organization","role":"reader","itemCount":3,"organizationId":"acme"}""",
        )
        assertEquals("acme", vault.organizationID)
        assertTrue(vault.isOrganization)

        val keys = backendJson.decodeFromString(
            WaitingVaultKeys.serializer(),
            """{"waiting":[{"vaultId":"ledger","organizationId":"acme","organizationName":"Acme","role":"reader"}],"deliveryError":"","rotationError":"the signing key of b@example.com changed"}""",
        )
        assertEquals(listOf("Acme"), keys.allWaiting.map { it.organizationName })
        assertEquals("the signing key of b@example.com changed", keys.rotationError)

        val organizations = backendJson.decodeFromString(
            Organizations.serializer(),
            """{"organizations":[{"id":"acme","name":"Acme","role":"admin","memberCount":3}],"invitations":[{"id":"i","organizationId":"globex","organizationName":"Globex","role":"admin","invitedByName":"Ada","invitedByEmail":"ada@example.com","expiresAt":"2027-01-22T08:00:00Z"}]}""",
        )
        assertEquals(3, organizations.allOrganization.firstOrNull()?.memberCount)
        assertEquals("Ada invited you to Globex as an admin", organizations.allInvitation.firstOrNull()?.message)

        val preview = backendJson.decodeFromString(
            AccountDeletionPreview.serializer(),
            """{"email":"a@example.com","ownedVaults":[],"memberVaults":[],"organizations":[{"id":"acme","name":"Acme","isDeleted":false,"newAdminName":"Bob"}]}""",
        )
        assertEquals(listOf(DeletionOrganization(id = "acme", name = "Acme", newAdminName = "Bob")), preview.allOrganization)
    }

    @Test
    fun testOnlyAdminsChangeWhichTeamsHaveAnOrganizationVault() {
        val asMember = OrganizationLogic.describeVaultSharing(buildAcme(isAdmin = false), "deploy")
        assertEquals(listOf("Audit", "Platform"), asMember.allTeamAccess.map { it.team.name })
        assertEquals(listOf(Member.VAULT_ROLE_READER, Member.VAULT_ROLE_EDITOR), asMember.allTeamAccess.map { it.role })
        assertFalse(asMember.canChangeTeams)
        assertEquals("Ask an admin of Acme to change who has this vault.", asMember.askAdminNote)
        assertEquals(emptyList(), asMember.allTeamWithoutVault)

        val asAdmin = OrganizationLogic.describeVaultSharing(buildAcme(isAdmin = true), "ledger")
        assertTrue(asAdmin.canChangeTeams)
        assertNull(asAdmin.askAdminNote)
        assertEquals(listOf("Audit"), asAdmin.allTeamAccess.map { it.team.name })
        assertEquals(listOf("Platform"), asAdmin.allTeamWithoutVault.map { it.name }, "Give to a team… offers the teams without the vault")
    }

    @Test
    fun testOnlyAdminsManageMembersAndTeams() {
        val asMember = buildAcme(isAdmin = false)
        val platform = asMember.allTeam[1]
        assertEquals(emptyList(), OrganizationLogic.listAllMemberAction(asMember, bob))
        assertEquals(emptyList(), OrganizationLogic.listAllAddableMember(asMember, platform))
        assertEquals(emptyList(), OrganizationLogic.listAllGivableVault(asMember, platform, emptyList()))

        val asAdmin = buildAcme(isAdmin = true)
        assertEquals(listOf(OrganizationMemberAction.MAKE_ADMIN, OrganizationMemberAction.REMOVE), OrganizationLogic.listAllMemberAction(asAdmin, bob))
        assertEquals(listOf(OrganizationMemberAction.MAKE_MEMBER, OrganizationMemberAction.REMOVE), OrganizationLogic.listAllMemberAction(asAdmin, carol))
        assertEquals(
            listOf(OrganizationMemberAction.MAKE_MEMBER),
            OrganizationLogic.listAllMemberAction(asAdmin, asAdmin.allMember[1]),
            "admins leave instead of removing themselves",
        )
        assertEquals(listOf("Carol"), OrganizationLogic.listAllAddableMember(asAdmin, asAdmin.allTeam[1]).map { it.name })

        val onlyOnThisDevice = VaultSummary("runbooks", "Runbooks", VaultSummary.TYPE_ORGANIZATION, Member.VAULT_ROLE_OWNER, 0, organizationID = "acme")
        val elsewhere = VaultSummary("other", "Other", VaultSummary.TYPE_ORGANIZATION, Member.VAULT_ROLE_OWNER, 0, organizationID = "globex")
        val allGivable = OrganizationLogic.listAllGivableVault(asAdmin, asAdmin.allTeam[1], listOf(onlyOnThisDevice, elsewhere))
        assertEquals(
            listOf("Runbooks", "A vault you can't open"),
            allGivable.map { it.displayName },
            "Deploy is already Platform's; another organization's vault is not offered",
        )
    }

    @Test
    fun testRefreshListsOrganizationsInvitationsAndWaitingKeysBeforeTheVaults() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        backend.allOrganization = mutableListOf(buildAcme(isAdmin = true))
        backend.allOrganizationInvitation = mutableListOf(
            OrganizationInvitation("invitation-9", "globex", "Globex", OrganizationRole.MEMBER, "Ada", "ada@example.com", ""),
        )
        backend.waitingVaultKeys = WaitingVaultKeys(allWaiting = listOf(WaitingVaultKey("ledger", "acme", "Acme", Member.VAULT_ROLE_OWNER)))
        val model = harness.startUnlockedModel()

        val data = model.unlockedData.value
        assertEquals(listOf("Acme"), data.allOrganization.map { it.name })
        assertEquals(3, data.allOrganization.firstOrNull()?.memberCount)
        assertEquals(1, data.allOrganizationInvitation.size)
        assertEquals(listOf("ledger"), data.waitingVaultKeys.allWaiting.map { it.vaultID })
        val keysIndex = backend.allCallName.indexOf("listAllWaitingVaultKey")
        val vaultsIndex = backend.allCallName.indexOf("listAllVault")
        assertTrue(keysIndex in 0..<vaultsIndex, "keys are given and rotated before the vaults load")

        model.lock()
        val lockedData = model.unlockedData.value
        assertEquals(emptyList(), lockedData.allOrganization)
        assertEquals(0, lockedData.allOrganizationInvitation.size)
        assertEquals(WaitingVaultKeys(), lockedData.waitingVaultKeys)
    }

    @Test
    fun testTheOrganizationScreenGoesBackOnlyWhenTheOrganizationIsGone() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        backend.allOrganization = mutableListOf(buildAcme(isAdmin = true))
        val model = harness.startUnlockedModel()
        val loaded = assertIs<OrganizationLoad.Loaded>(model.organizations.loadOrganizationScreen("acme"))
        assertEquals("Acme", loaded.organization.name)

        val sentence = "Hisar could not read a vault of Acme"
        backend.getOrganizationErrorMessage = sentence
        assertEquals(OrganizationLoad.Failed, model.organizations.loadOrganizationScreen("acme"), "Alice is still in Acme, so the screen stays")
        assertEquals(sentence, model.takeRefusal("Acme")?.message, "the alert says why")
        assertNull(model.errorMessage.value)

        backend.listAllOrganizationErrorMessage = "cannot reach the Hisar server"
        assertEquals(OrganizationLoad.Failed, model.organizations.loadOrganizationScreen("acme"), "a failed list does not say Alice left")
        assertEquals(sentence, model.takeRefusal("Acme")?.message, "the alert gives the organization's sentence, not the list's")
        assertEquals(listOf("acme"), model.unlockedData.value.allOrganization.map { it.id })

        backend.getOrganizationErrorMessage = null
        backend.listAllOrganizationErrorMessage = null
        backend.allOrganization = mutableListOf()
        assertEquals(OrganizationLoad.Gone, model.organizations.loadOrganizationScreen("acme"), "deleted, or Alice was removed: the screen goes back")
        assertNull(model.errorMessage.value)
        assertEquals(emptyList(), model.unlockedData.value.allOrganization)
    }

    @Test
    fun testARefusalKeepsTheServersSentenceForTheAlert() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        backend.allOrganization = mutableListOf(buildAcme(isAdmin = true))
        val model = harness.startUnlockedModel()
        val sentence = "Nobody who can open this vault would be its owner. Give owner access to a team whose members already have the vault first."
        backend.organizationRefusal = sentence

        assertFalse(model.organizations.takeVaultFromTeam("acme", "audit", "ledger"))
        val refusal = model.takeRefusal("Take back")
        assertEquals("Take back", refusal?.title)
        assertEquals(sentence, refusal?.message)
        assertNull(model.errorMessage.value, "the alert shows the sentence, not the screen's error banner")
        assertNull(model.takeRefusal("Take back"))
        assertEquals(listOf("deploy", "ledger"), backend.allOrganization.firstOrNull()?.allTeam?.firstOrNull()?.allVault?.map { it.vaultID })
    }

    @Test
    fun testLeavingAnOrganizationDropsItsVaults() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.allOrganization = mutableListOf(buildAcme(isAdmin = false))
        val model = harness.startUnlockedModel()
        model.vaults.createOrganizationVault("acme", "platform", "Runbooks")
        assertEquals(listOf("Runbooks"), model.unlockedData.value.allVault.filter { it.isOrganization }.map { it.name })

        assertTrue(model.organizations.leaveOrganization("acme"))
        assertEquals(emptyList(), model.unlockedData.value.allOrganization)
        assertEquals(emptyList(), model.unlockedData.value.allVault.filter { it.isOrganization })
        assertEquals("You left Acme.", model.notice.value?.message)
    }
}
