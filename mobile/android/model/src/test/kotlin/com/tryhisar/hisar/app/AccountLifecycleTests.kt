package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.AccountDeletionPreview
import com.tryhisar.hisar.model.AccountLifecycleLogic
import com.tryhisar.hisar.model.DeletionOrganization
import com.tryhisar.hisar.model.DeletionVault
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.backendJson
import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest

/** Changing the master password, replacing the Secret Key and deleting the account. */
class AccountLifecycleTests {
    @Test
    fun testNewPasswordRules() {
        assertNull(AccountLifecycleLogic.describeNewPasswordProblem("old password", "", ""))
        assertEquals("Use at least 10 characters.", AccountLifecycleLogic.describeNewPasswordProblem("old password", "short", ""))
        assertEquals(
            "The new master password is the same as the current one.",
            AccountLifecycleLogic.describeNewPasswordProblem("old password", "old password", ""),
        )
        assertEquals("The passwords do not match.", AccountLifecycleLogic.describeNewPasswordProblem("old password", "a new master password", "a new master"))
        assertFalse(AccountLifecycleLogic.canChangePassword("", "a new master password", "a new master password"))
        assertFalse(AccountLifecycleLogic.canChangePassword("old password", "a new master password", ""))
        assertTrue(AccountLifecycleLogic.canChangePassword("old password", "a new master password", "a new master password"))
    }

    @Test
    fun testEmailConfirmationIgnoresCaseAndSpaces() {
        assertTrue(AccountLifecycleLogic.isEmailMatching(" Alice@Example.com ", "alice@example.com"))
        assertFalse(AccountLifecycleLogic.isEmailMatching("bob@example.com", "alice@example.com"))
        assertFalse(AccountLifecycleLogic.isEmailMatching("", ""))
    }

    @Test
    fun testDeletionConsequences() {
        val preview = AccountDeletionPreview(
            email = "alice@example.com",
            allOwnedVault = listOf(
                DeletionVault(id = "personal", name = "Personal", type = "personal", role = "owner", otherMemberCount = 0),
                DeletionVault(id = "family", name = "Family", type = "shared", role = "owner", otherMemberCount = 2),
            ),
            allMemberVault = listOf(DeletionVault(id = "work", name = "Work", type = "shared", role = "editor", otherMemberCount = 0)),
            shareLinkCount = 1, sentInvitationCount = 1, receivedInvitationCount = 1, deviceCount = 3,
        )
        assertEquals(2, preview.peopleLosingAccessCount)
        assertEquals("Deleted with its items. 2 people lose access.", AccountLifecycleLogic.describeOwnedVault(preview.allOwnedVault[1]))
        assertEquals("Stays with its owners. Your access is removed.", AccountLifecycleLogic.describeMemberVault(preview.allMemberVault[0]))
        assertEquals(
            listOf("1 share link stops working.", "2 invitations are withdrawn.", "This device and 2 other devices are signed out."),
            AccountLifecycleLogic.describeAllOtherConsequence(preview),
        )
    }

    @Test
    fun testAccountDeletionSaysWhatHappensToEachOrganization() {
        assertEquals("Acme is deleted with your account", AccountLifecycleLogic.describeOrganization(DeletionOrganization(id = "a", name = "Acme", isDeleted = true)))
        assertEquals("Ada becomes an admin of Acme", AccountLifecycleLogic.describeOrganization(DeletionOrganization(id = "a", name = "Acme", newAdminName = "Ada")))
        assertEquals("You leave Acme", AccountLifecycleLogic.describeOrganization(DeletionOrganization(id = "a", name = "Acme")))
    }

    @Test
    fun testPreviewDecodesTheBackendJSON() {
        val json = """{"email":"a@example.com","ownedVaults":[{"id":"v1","name":"Family","type":"shared","role":"owner","otherMemberCount":2}],"memberVaults":[],"shareLinkCount":3,"sentInvitationCount":0,"receivedInvitationCount":0,"deviceCount":1}"""
        val preview = backendJson.decodeFromString<AccountDeletionPreview>(json)
        assertEquals(2, preview.allOwnedVault.first().otherMemberCount)
        assertEquals(3, preview.shareLinkCount)
        val status = backendJson.decodeFromString<VaultStatus>("""{"state":"signed-out","signedOutReason":"This device was signed out."}""")
        assertEquals("This device was signed out.", status.signedOutReason)
    }

    @Test
    fun testChangeMasterPassword() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        assertNull(model.settings.changeMasterPassword("wrong", "a new master password"))
        assertEquals("The master password is not correct", model.errorMessage.value)
        val result = model.settings.changeMasterPassword(FakeBackend.PASSWORD, "a new master password")
        assertEquals(1, result?.signedOutDeviceCount)
        assertEquals(Screen.Unlocked, model.screen.value)
        assertEquals("Master password changed. 1 other device was signed out.", model.notice.value)
        assertEquals("a new master password", harness.backend.currentPassword)
    }

    @Test
    fun testReplaceSecretKeyWritesAndRemovesTheNewEmergencyKit() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val result = assertNotNull(model.settings.replaceSecretKey(FakeBackend.PASSWORD))
        assertNotEquals(FakeBackend.SECRET_KEY, result.secretKey)
        assertTrue(File(result.emergencyKitPath).exists())
        model.settings.finishSecretKeyReplacement(result)
        assertFalse(File(result.emergencyKitPath).exists())
        runCurrent()
        assertTrue("forgetNewAccountSecretKey" in harness.backend.allCallName, "the new Secret Key was not forgotten")
    }

    @Test
    fun testDeleteAccountSignsOut() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        assertEquals("alice@example.com", model.settings.loadAccountDeletionPreview()?.email)
        assertFalse(model.settings.deleteAccount(FakeBackend.PASSWORD, "bob@example.com"))
        assertEquals(Screen.Unlocked, model.screen.value)
        assertTrue(model.settings.deleteAccount(FakeBackend.PASSWORD, "alice@example.com"))
        assertTrue(harness.backend.isAccountDeleted)
        assertEquals(Screen.SignedOut, model.screen.value)
        assertTrue(model.unlockedData.value.allVault.isEmpty())
        assertEquals("Your Hisar account was deleted.", model.notice.value)
    }
}
