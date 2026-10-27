package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.ShareInvitations
import com.tryhisar.hisar.model.VaultSharing
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.backendJson
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runTest

/** The iPhone app's AppModelTests for sharing vaults with people and inviting them to Hisar. */
class SharingTests {
    @Test
    fun testAnyoneInvitesByEmailWithTrimmedAddress() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.seedSignedIn()
        harness.backend.allMember[0] = harness.backend.allMember[0].copy(role = Member.ROLE_MEMBER)
        val model = harness.startModel()
        assertTrue(model.sharing.invitePerson(" dave@example.com "))
        assertEquals(listOf("dave@example.com"), harness.backend.allInvitedEmail)
        assertNull(model.errorMessage.value)
        assertFalse(model.sharing.invitePerson("dave"))
        assertEquals("Enter the email address of the person", model.errorMessage.value)
    }

    @Test
    fun testPeopleAreOnlyThoseWhoShareAVault() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        assertEquals(listOf("alice@example.com", "bob@example.com"), model.sharing.loadAllPerson()?.map { it.email })
    }

    @Test
    fun testPeopleReloadKeepsWaitingAndLastSuccessfulData() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        var people = model.sharing.loadPeople(null)
        assertEquals(2, people?.allPerson?.size)
        val waiting = com.tryhisar.hisar.model.WaitingShareInvitation("waiting-1", "work", Member.VAULT_ROLE_READER, "Ada", "ada@example.com", "")
        harness.backend.shareInvitations = ShareInvitations(allWaiting = listOf(waiting))
        model.handleRemoteChange()
        people = model.sharing.loadPeople(people)
        assertEquals(listOf(waiting), people?.invitations?.allWaiting)
        model.errorMessage.value = "Could not decline the invitation."
        people = model.sharing.loadPeople(people)
        assertEquals("Could not decline the invitation.", model.errorMessage.value)
        harness.backend.peopleErrorMessage = "Offline"
        harness.backend.shareInvitations = ShareInvitations()
        people = model.sharing.loadPeople(people)
        assertEquals(listOf(waiting), people?.invitations?.allWaiting)
        harness.backend.peopleErrorMessage = null
        people = model.sharing.loadPeople(people)
        assertTrue(people?.invitations?.allWaiting?.isEmpty() == true)
    }

    @Test
    fun testOwnerSendsShareInvitationThenChangesRoleAndRemovesMember() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        val model = harness.startUnlockedModel()
        val vault = assertNotNull(model.vaults.createVault("Streaming"))
        assertTrue(model.sharing.shareVault(vault.id, " bob@example.com ", Member.VAULT_ROLE_READER))
        val shareInvitations = model.invitations.loadShareInvitations()
        assertEquals(listOf("bob@example.com"), shareInvitations?.allSent?.map { it.email })
        assertEquals("Streaming", shareInvitations?.allSent?.first()?.vaultName)
        assertEquals(listOf("alice@example.com"), model.sharing.loadAllVaultMember(vault.id)?.map { it.email }, "nobody gets access before accepting")

        backend.allVaultMemberByVaultID.getValue(vault.id) += Member("bob-id", "Bob", "bob@example.com", Member.VAULT_ROLE_READER, "3333 4444")
        assertTrue(model.sharing.changeVaultMemberRole(vault.id, "bob-id", Member.VAULT_ROLE_EDITOR))
        assertEquals(Member.VAULT_ROLE_EDITOR, model.sharing.loadAllVaultMember(vault.id)?.last()?.role)

        assertTrue(model.sharing.removeVaultMember(vault.id, "bob-id"))
        assertEquals(listOf("alice@example.com"), model.sharing.loadAllVaultMember(vault.id)?.map { it.email })
        val removeIndex = backend.allCallName.indexOf("removeVaultMember")
        assertTrue(removeIndex >= 0, "remove not called")
        assertEquals("listAllItem", backend.allCallName[removeIndex + 1], "items are loaded again after the vault key changed")

        assertTrue(model.sharing.cancelShareInvitation(shareInvitations?.allSent?.first()?.id.orEmpty()))
        assertEquals(0, model.invitations.loadShareInvitations()?.allSent?.size)
    }

    @Test
    fun testSharingConfirmationBindsReviewedRecipient() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val vault = assertNotNull(model.vaults.createVault("Team"))
        val invitation = com.tryhisar.hisar.model.SentShareInvitation("accepted", vault.id, "Team", "new@example.com", "reader", "accepted", "2027-01-22T08:00:00Z", true, "new-account", "3333 4444", "reviewed-identity")
        harness.backend.shareInvitations = ShareInvitations(allSent = listOf(invitation))
        assertFalse(model.sharing.confirmShareRecipient(invitation.copy(reviewToken = "other-identity")))
        assertEquals(1, harness.backend.shareInvitations.allSent.size)
        assertTrue(model.sharing.confirmShareRecipient(invitation))
        assertTrue(harness.backend.shareInvitations.allSent.isEmpty())
    }

    @Test
    fun testVaultSharingShowsMembersAndOnlyThisVaultsInvitations() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        val streaming = assertNotNull(model.vaults.createVault("Streaming"))
        val deploy = assertNotNull(model.vaults.createVault("Deploy keys"))
        model.sharing.shareVault(streaming.id, "bob@example.com", Member.VAULT_ROLE_READER)
        model.sharing.shareVault(deploy.id, "carol@example.com", Member.VAULT_ROLE_EDITOR)
        val sharing = model.sharing.loadVaultSharing(streaming.id)
        assertEquals("Streaming", sharing?.vault?.name)
        assertEquals(true, sharing?.isOwner)
        assertEquals(listOf("alice@example.com"), sharing?.allMember?.map { it.email })
        assertEquals(listOf("bob@example.com"), sharing?.allInvitation?.map { it.email })

        val editorSharing = model.sharing.loadVaultSharing("family")
        assertEquals(false, editorSharing?.isOwner, "an editor sees the members only")
        assertEquals(0, editorSharing?.allInvitation?.size)

        val personal = assertNotNull(model.unlockedData.value.allVault.firstOrNull { it.type == VaultSummary.TYPE_PERSONAL })
        assertNull(model.sharing.loadVaultSharing(personal.id))
        assertEquals("Personal vaults are private and cannot be shared; create a shared vault instead", model.errorMessage.value)
    }

    @Test
    fun testVaultSharingDecodesFromHisarKit() {
        val json = """{"vault":{"id":"v","name":"Deploy keys","type":"shared","role":"owner","itemCount":2},"isOwner":true,"members":[{"accountId":"a","name":"Ada","email":"ada@example.com","role":"owner","securityCode":"3F2A 91C0 7B44 E1D8","isCurrentUser":true,"isKeyChanged":false}],"invitations":null,"completionError":"bob@example.com: the signing key of bob@example.com changed since you last shared with them; stopping"}"""
        val decoded = backendJson.decodeFromString(VaultSharing.serializer(), json)
        assertEquals("Deploy keys", decoded.vault.name)
        assertTrue(decoded.isOwner)
        assertEquals("3F2A 91C0 7B44 E1D8", decoded.allMember.first().securityCode)
        assertEquals(0, decoded.allInvitation.size)
        assertTrue(decoded.completionError.contains("changed since you last shared"))
    }

    @Test
    fun testShareInvitationsDecodeFromHisarKit() {
        val json = """{"received":[{"id":"a","vaultId":"v","role":"editor","invitedByName":"Carol","invitedByEmail":"carol@example.com","expiresAt":"2027-01-22T08:00:00Z"}],"sent":null,"completionError":""}"""
        val decoded = backendJson.decodeFromString(ShareInvitations.serializer(), json)
        assertEquals("v", decoded.allReceived.first().vaultID)
        assertEquals(0, decoded.allSent.size)
    }

    @Test
    fun testSharingWithAStrangerOrAnEditedVaultFails() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        assertFalse(model.sharing.shareVault("family", "nobody@example.com", Member.VAULT_ROLE_EDITOR))
        assertEquals("Only owners of this vault can share it, change roles or remove members", model.errorMessage.value)
        assertFalse(model.sharing.shareVault("personal", "bob@example.com", Member.VAULT_ROLE_EDITOR))
        assertEquals("Personal vaults are private and cannot be shared; create a shared vault instead", model.errorMessage.value)
    }
}
