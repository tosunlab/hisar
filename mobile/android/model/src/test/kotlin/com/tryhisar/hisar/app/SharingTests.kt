package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.ShareInvitations
import com.tryhisar.hisar.model.VaultSharing
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.backendJson
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ReceivedShareInvitation
import com.tryhisar.hisar.model.SentShareInvitation
import com.tryhisar.hisar.model.VaultStatus
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
    fun testInvitationLifetimeShowsExpiryAndClosesAcceptanceAtItsDeadline() {
        val now = java.time.Instant.ofEpochSecond(1_800_000_000)
        val lifetime = com.tryhisar.hisar.model.InvitationLifetime(now.plusSeconds(5 * 86400).toString())
        assertEquals("Expires in 5 days", lifetime.describe(now))
        assertFalse(lifetime.isExpired(now))
        assertTrue(lifetime.exactDate.isNotEmpty())
        assertEquals("Expires tomorrow", lifetime.describe(now.plusSeconds(4 * 86400)))
        assertEquals("Expired", lifetime.describe(now.plusSeconds(5 * 86400)))
        assertTrue(lifetime.isExpired(now.plusSeconds(5 * 86400)))
        assertNotNull(com.tryhisar.hisar.model.InvitationLifetime("2027-01-15T12:00:00.123Z").date)
    }

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
        assertEquals("Offline", model.sharing.peopleErrorMessage.value)
        model.refreshAll()
        assertEquals("Offline", model.sharing.peopleErrorMessage.value)
        harness.backend.peopleErrorMessage = null
        people = model.sharing.loadPeople(people)
        assertTrue(people?.invitations?.allWaiting?.isEmpty() == true)
        assertNull(model.sharing.peopleErrorMessage.value)
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
        model.invitations.loadShareInvitations()
        assertEquals(listOf(invitation), model.unlockedData.value.allReviewInvitation)
        assertFalse(model.sharing.confirmShareRecipient(invitation.copy(reviewToken = "other-identity")))
        assertEquals(1, harness.backend.shareInvitations.allSent.size)
        assertEquals(listOf(invitation), model.unlockedData.value.allReviewInvitation)
        assertTrue(model.sharing.confirmShareRecipient(invitation))
        assertTrue(harness.backend.shareInvitations.allSent.isEmpty())
        assertTrue(model.unlockedData.value.allReviewInvitation.isEmpty())
    }

    @Test
    fun testUnlockKeepsAcceptedRecipientsVisibleUntilReviewed() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val review = SentShareInvitation("review", "family", "Family", "ada@example.com", "reader", "accepted", "", requiresConfirmation = true, reviewToken = "reviewed")
        backend.shareInvitations = ShareInvitations(
            allReceived = listOf(ReceivedShareInvitation("received", "another", "editor", "Bob", "bob@example.com", "")),
            allSent = listOf(review, review.copy(id = "pending", state = "pending"), review.copy(id = "bound", requiresConfirmation = false)),
        )
        val model = harness.startModel()
        assertTrue(model.unlockedData.value.allReviewInvitation.isEmpty())
        assertTrue(model.lock.unlock(FakeBackend.PASSWORD))
        assertEquals(listOf(review), model.unlockedData.value.allReviewInvitation)
        assertEquals(1, model.unlockedData.value.allReceivedInvitation.size)
        assertEquals(2, model.unlockedData.value.peopleInvitationCount)
    }

    @Test
    fun testReviewRemindersRefreshAfterCancellationAndClearOnLock() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val review = SentShareInvitation("review", "family", "Family", "ada@example.com", "reader", "accepted", "", requiresConfirmation = true, reviewToken = "reviewed")
        val other = review.copy(id = "other")
        harness.backend.shareInvitations = ShareInvitations(allSent = listOf(review, other))
        model.invitations.loadShareInvitations()
        assertTrue(model.sharing.cancelShareInvitation(review.id))
        assertEquals(listOf(other), model.unlockedData.value.allReviewInvitation)
        assertEquals(1, model.unlockedData.value.peopleInvitationCount)
        model.lock()
        assertTrue(model.unlockedData.value.allReviewInvitation.isEmpty())
        assertEquals(0, model.unlockedData.value.peopleInvitationCount)
        assertTrue(model.lock.unlock(FakeBackend.PASSWORD))
        assertEquals(listOf(other), model.unlockedData.value.allReviewInvitation)
        assertTrue(model.lock.signOut())
        assertTrue(model.unlockedData.value.allReviewInvitation.isEmpty())
    }

    @Test
    fun testMismatchedRecipientCancellationNeverConfirmsAccess() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val invitation = com.tryhisar.hisar.model.SentShareInvitation("accepted", "family", "Family", "new@example.com", "reader", "accepted", "", true, "new-account", "3333 4444", "reviewed-identity")
        harness.backend.shareInvitations = ShareInvitations(allSent = listOf(invitation))
        harness.backend.cancelShareInvitationErrorMessage = "Offline"
        assertFalse(model.sharing.cancelShareInvitation(invitation.id))
        assertEquals(listOf(invitation), harness.backend.shareInvitations.allSent)
        assertEquals("Offline", model.errorMessage.value)
        harness.backend.cancelShareInvitationErrorMessage = null
        assertTrue(model.sharing.cancelShareInvitation(invitation.id))
        assertTrue(harness.backend.shareInvitations.allSent.isEmpty())
        assertFalse(harness.backend.allCallName.contains("confirmShareRecipient"))
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
