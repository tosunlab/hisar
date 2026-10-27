package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.Organization
import com.tryhisar.hisar.model.OrganizationInvitation
import com.tryhisar.hisar.model.OrganizationLogic
import com.tryhisar.hisar.model.OrganizationRole
import com.tryhisar.hisar.model.OrganizationSummary
import com.tryhisar.hisar.model.ReceivedShareInvitation
import com.tryhisar.hisar.model.SentOrganizationInvitation
import com.tryhisar.hisar.model.ShareInvitations
import com.tryhisar.hisar.model.Team
import com.tryhisar.hisar.model.TeamMember
import com.tryhisar.hisar.model.TeamVault
import com.tryhisar.hisar.model.ReadIssue
import com.tryhisar.hisar.model.VaultLocation
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.WaitingShareInvitation
import com.tryhisar.hisar.model.WaitingVaultKey
import com.tryhisar.hisar.model.WaitingVaultKeys
import com.tryhisar.hisar.model.formatTimeAgo
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runTest

/** The Vaults screen's sections: invitations, new vaults with their Where, organization groups and key warnings. */
class VaultsHomeTests {
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

    private val carolInvitation = ReceivedShareInvitation("from-carol", "carol-vault", Member.VAULT_ROLE_EDITOR, "Carol", "carol@example.com", "2027-01-22T08:00:00Z")
    private val daveInvitation = ReceivedShareInvitation("from-dave", "dave-vault", Member.VAULT_ROLE_READER, "", "dave@example.com", "2027-01-22T08:00:00Z")

    @Test
    fun testCreateVaultAddsASharedVaultOwnedByTheUser() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val vault = model.vaults.createVault("  Streaming ")
        assertEquals("Streaming", vault?.name)
        assertEquals("shared", vault?.type)
        assertEquals("owner", vault?.role)
        assertEquals(listOf("Personal", "Family", "Streaming"), model.unlockedData.value.allVault.map { it.name }, "personal first, then shared vaults by name")
        assertEquals(0, model.unlockedData.value.countItem(vault?.id.orEmpty()))
        val allMember = harness.backend.listAllVaultMember(vault?.id.orEmpty())
        assertEquals(listOf("alice@example.com"), allMember.map { it.email })
        assertEquals("owner", allMember.first().role)
    }

    @Test
    fun testCreateVaultWithoutNameShowsError() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        val vault = model.vaults.createVault("   ")
        assertNull(vault)
        assertEquals("Enter a vault name", model.errorMessage.value)
        assertEquals(2, model.unlockedData.value.allVault.size)
    }

    @Test
    fun testReceivedShareInvitationsAreAcceptedOrDeclined() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.seedSignedIn()
        harness.backend.shareInvitations = ShareInvitations(allReceived = listOf(carolInvitation, daveInvitation))
        val model = harness.startModel()
        val shareInvitations = model.invitations.loadShareInvitations()
        assertEquals(listOf("Carol", "dave@example.com"), shareInvitations?.allReceived?.map { it.inviterName })
        assertTrue(model.invitations.acceptShareInvitation("from-carol"))
        assertTrue(model.invitations.declineShareInvitation("from-dave"))
        assertEquals(listOf("from-carol accept", "from-dave decline"), harness.backend.allAnsweredInvitation)
        assertFalse(model.invitations.acceptShareInvitation("from-carol"))
        assertEquals("This invitation no longer exists", model.errorMessage.value)
    }

    @Test
    fun testRefreshLoadsReceivedInvitationsForTheBannerAndAnsweringRemovesThem() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.seedSignedIn()
        harness.backend.shareInvitations = ShareInvitations(allReceived = listOf(carolInvitation, daveInvitation))
        val model = harness.startModel()
        model.refreshAll()
        assertEquals(listOf("from-carol", "from-dave"), model.unlockedData.value.allReceivedInvitation.map { it.id })
        val allCallName = harness.backend.allCallName
        assertTrue(allCallName.indexOf("listAllShareInvitation") < allCallName.indexOf("listAllVault"), "accepted shares complete before the vaults load")

        assertTrue(model.invitations.acceptShareInvitation("from-carol"))
        assertEquals(listOf("from-dave"), model.unlockedData.value.allReceivedInvitation.map { it.id })
        assertTrue(model.invitations.declineShareInvitation("from-dave"))
        assertTrue(model.unlockedData.value.allReceivedInvitation.isEmpty())

        model.lock()
        assertTrue(model.unlockedData.value.allReceivedInvitation.isEmpty())
    }

    @Test
    fun testRecipientSeesWhoTheAcceptedShareIsWaitingFor() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.seedSignedIn()
        harness.backend.shareInvitations = ShareInvitations(
            allWaiting = listOf(WaitingShareInvitation("from-carol", "carol-vault", Member.VAULT_ROLE_EDITOR, "Carol", "carol@example.com", "2027-01-15T06:00:00Z")),
        )
        val model = harness.startModel()
        model.refreshAll()
        val allWaiting = model.unlockedData.value.allWaitingInvitation
        assertEquals(listOf("from-carol"), allWaiting.map { it.id })
        assertEquals("Waiting for Carol's Hisar to finish sharing · accepted 2 h ago", allWaiting.first().describe(Instant.parse("2027-01-15T08:05:00Z")))
        model.lock()
        assertTrue(model.unlockedData.value.allWaitingInvitation.isEmpty())
    }

    @Test
    fun testFormatTimeAgo() {
        val now = Instant.parse("2027-01-15T12:00:00Z")
        assertEquals("just now", formatTimeAgo("2027-01-15T11:59:40Z", now))
        assertEquals("5 min ago", formatTimeAgo("2027-01-15T11:55:00Z", now))
        assertEquals("yesterday", formatTimeAgo("2027-01-14T09:00:00Z", now))
        assertEquals("3 days ago", formatTimeAgo("2027-01-12T12:00:00Z", now))
        assertEquals("", formatTimeAgo("", now))
    }

    @Test
    fun testOrganizationVaultsAreGroupedUnderTheirOrganizationWithWaitingKeys() {
        val personal = VaultSummary("personal", "Personal", VaultSummary.TYPE_PERSONAL, Member.VAULT_ROLE_OWNER, 0)
        val family = VaultSummary("family", "Family", VaultSummary.TYPE_SHARED, Member.VAULT_ROLE_EDITOR, 0)
        val deploy = VaultSummary("deploy", "Deploy", VaultSummary.TYPE_ORGANIZATION, Member.VAULT_ROLE_EDITOR, 0, organizationID = "northwind")
        val audit = VaultSummary("audit", "Audit", VaultSummary.TYPE_ORGANIZATION, Member.VAULT_ROLE_READER, 0, organizationID = "northwind")
        val ledger = VaultSummary("ledger", "Ledger", VaultSummary.TYPE_ORGANIZATION, Member.VAULT_ROLE_OWNER, 0, organizationID = "acme")
        val unlisted = VaultSummary("unlisted", "Unlisted", VaultSummary.TYPE_ORGANIZATION, Member.VAULT_ROLE_OWNER, 0, organizationID = "initech")
        val allOrganization = listOf(
            OrganizationSummary("northwind", "Northwind", OrganizationRole.MEMBER, 2),
            OrganizationSummary("acme", "acme", OrganizationRole.ADMIN, 4),
        )
        val allWaiting = listOf(
            WaitingVaultKey("ops", "northwind", "Northwind", Member.VAULT_ROLE_EDITOR),
            WaitingVaultKey("deploy", "northwind", "Northwind", Member.VAULT_ROLE_EDITOR),
            WaitingVaultKey("plans", "globex", "Globex", Member.VAULT_ROLE_READER),
            WaitingVaultKey("notes", "globex", "Globex", Member.VAULT_ROLE_READER),
        )

        val grouping = OrganizationLogic.groupVaults(listOf(personal, family, audit, deploy, ledger, unlisted), allOrganization, allWaiting)

        assertEquals(listOf("personal", "family", "unlisted"), grouping.allOtherVault.map { it.id }, "a vault of an organization not listed yet stays with the others")
        assertEquals(listOf("acme", "Globex", "Northwind"), grouping.allGroup.map { it.name })
        assertEquals(listOf("ledger"), grouping.allGroup[0].allVault.map { it.id })
        assertEquals("", grouping.allGroup[0].waitingMessage)
        assertEquals(emptyList(), grouping.allGroup[1].allVault, "an organization whose only vaults wait for keys still gets its section")
        assertEquals("Waiting for the owners of 2 vaults in Globex to give you their keys", grouping.allGroup[1].waitingMessage)
        assertEquals(listOf("audit", "deploy"), grouping.allGroup[2].allVault.map { it.id })
        assertEquals("Waiting for an owner of a vault in Northwind to give you its key", grouping.allGroup[2].waitingMessage, "a vault that arrived no longer waits")
    }

    @Test
    fun testNewItemsGoIntoTheVaultShownElseThePersonalVault() {
        val personal = VaultSummary("personal", "Personal", VaultSummary.TYPE_PERSONAL, Member.VAULT_ROLE_OWNER, 0)
        val family = VaultSummary("family", "Family", VaultSummary.TYPE_SHARED, Member.VAULT_ROLE_EDITOR, 0)
        val audit = VaultSummary("audit", "Audit", VaultSummary.TYPE_ORGANIZATION, Member.VAULT_ROLE_READER, 0, organizationID = "northwind")
        val locked = VaultSummary("locked", "Locked", VaultSummary.TYPE_SHARED, Member.VAULT_ROLE_OWNER, 0, readIssues = listOf(ReadIssue(vaultId = "locked", kind = "vault-key")))
        val allVault = listOf(audit, locked, family, personal)

        assertEquals(listOf("family", "personal"), allVault.filter(VaultLogic::isWritable).map { it.id })
        assertEquals("personal", VaultLogic.chooseNewItemVault(allVault, currentVaultID = null)?.id, "from the Vaults screen")
        assertEquals("family", VaultLogic.chooseNewItemVault(allVault, currentVaultID = "family")?.id)
        assertEquals("personal", VaultLogic.chooseNewItemVault(allVault, currentVaultID = "audit")?.id, "a reader's vault takes no new items")
        assertEquals("personal", VaultLogic.chooseNewItemVault(allVault, currentVaultID = "locked")?.id, "nor one whose key did not open")
        assertEquals("family", VaultLogic.chooseNewItemVault(listOf(audit, family), currentVaultID = null)?.id)
        assertNull(VaultLogic.chooseNewItemVault(listOf(audit), currentVaultID = null))
    }

    @Test
    fun testKeyWarningsSayWhatHisarCouldNotDo() {
        assertEquals(emptyList(), OrganizationLogic.describeKeyWarnings(WaitingVaultKeys()))
        assertEquals(
            listOf(
                "Hisar couldn't give every vault key: the signing key of bob@example.com changed",
                "Hisar couldn't re-encrypt every vault that needs a new key: cannot reach the Hisar server",
            ),
            OrganizationLogic.describeKeyWarnings(WaitingVaultKeys(deliveryError = "the signing key of bob@example.com changed", rotationError = "cannot reach the Hisar server")),
        )
    }

    @Test
    fun testWhereOffersASharedVaultAndTheTeamsTheUserIsIn() {
        val northwind = Organization(
            id = "northwind", name = "Northwind", role = OrganizationRole.MEMBER, isAdmin = false,
            allTeam = listOf(Team(id = "ops", name = "Ops", allMember = listOf(TeamMember("alice-id", "Alice", "alice@example.com", isCurrentUser = true)))),
        )
        val allLocation = OrganizationLogic.listAllVaultLocation(listOf(northwind, buildAcme(isAdmin = true)))
        assertEquals(listOf("Shared vault", "Acme › Platform", "Northwind › Ops"), allLocation.map { it.label }, "Audit is left out: Alice is not in it")
        assertTrue(allLocation[0].isSharedVault)
        assertEquals("acme", allLocation[1].organizationID)
        assertEquals("platform", allLocation[1].teamID)
        assertEquals(listOf(VaultLocation.sharedVault), OrganizationLogic.listAllVaultLocation(emptyList()))
    }

    @Test
    fun testAcceptingAnInvitationJoinsTheOrganization() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.allOrganizationInvitation = mutableListOf(
            OrganizationInvitation("invitation-9", "globex", "Globex", OrganizationRole.MEMBER, "Ada", "ada@example.com", ""),
            OrganizationInvitation("invitation-10", "initech", "Initech", OrganizationRole.ADMIN, "", "bill@example.com", ""),
        )
        val model = harness.startUnlockedModel()
        assertEquals(
            listOf("Ada invited you to Globex as a member", "bill@example.com invited you to Initech as an admin"),
            model.unlockedData.value.allOrganizationInvitation.map { it.message },
        )

        assertTrue(model.invitations.acceptOrganizationInvitation("invitation-9"))
        assertEquals(listOf("invitation-10"), model.unlockedData.value.allOrganizationInvitation.map { it.id })
        assertEquals(listOf("Globex"), model.unlockedData.value.allOrganization.map { it.name })

        assertTrue(model.invitations.declineOrganizationInvitation("invitation-10"))
        assertTrue(model.unlockedData.value.allOrganizationInvitation.isEmpty())
        assertEquals(listOf("Globex"), harness.backend.allOrganization.map { it.name })
    }

    @Test
    fun testCreatingAVaultForATeamMakesAnOrganizationVault() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.allOrganization = mutableListOf(buildAcme(isAdmin = false))
        val model = harness.startUnlockedModel()
        val allLocation = model.vaults.loadVaultLocations()
        assertEquals(listOf("Shared vault", "Acme › Platform"), allLocation?.map { it.label })

        val vault = model.vaults.createOrganizationVault("acme", "platform", "Runbooks")
        assertEquals("acme", vault?.organizationID)
        assertEquals(true, model.unlockedData.value.findVault(vault?.id.orEmpty())?.isOrganization)
        val data = model.unlockedData.value
        val grouping = OrganizationLogic.groupVaults(data.allVault, data.allOrganization, emptyList())
        assertEquals(listOf("Runbooks"), grouping.allGroup.firstOrNull()?.allVault?.map { it.name }, "the new vault shows under Acme")

        val refused = model.vaults.createOrganizationVault("acme", "audit", "Elsewhere")
        assertNull(refused, "only members of a team create its vaults")
        assertEquals("Only members of the team can create a vault for it", model.errorMessage.value)
    }

    @Test
    fun testANewVaultWaitsForWhereAndNeverTakesAFailedLookupForASharedVault() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        backend.allOrganization = mutableListOf(buildAcme(isAdmin = false))
        val model = harness.startUnlockedModel()
        assertNull(OrganizationLogic.findVaultLocation(VaultLocation.sharedVault.id, null), "Create waits until Where is loaded")

        val sentence = "cannot reach the Hisar server"
        backend.getOrganizationErrorMessage = sentence
        val failed = model.vaults.loadVaultLocations()
        assertNull(failed, "a failed team lookup offers no shared vault in its place")
        assertNull(OrganizationLogic.findVaultLocation(VaultLocation.sharedVault.id, failed), "Create stays disabled")
        assertEquals(sentence, model.errorMessage.value, "the new vault screen's banner says why")

        backend.getOrganizationErrorMessage = null
        backend.listAllOrganizationErrorMessage = sentence
        val unlisted = model.vaults.loadVaultLocations()
        assertNull(unlisted, "nor does a failed list of the organizations")
        assertEquals(sentence, model.errorMessage.value)

        backend.listAllOrganizationErrorMessage = null
        val allLocation = model.vaults.loadVaultLocations()
        assertNull(model.errorMessage.value)
        assertEquals(VaultLocation.sharedVault, OrganizationLogic.findVaultLocation(VaultLocation.sharedVault.id, allLocation))
        assertEquals("Acme › Platform", OrganizationLogic.findVaultLocation("acme/platform", allLocation)?.label)
        assertNull(OrganizationLogic.findVaultLocation("acme/audit", allLocation), "Alice is not in Audit")
    }
}
