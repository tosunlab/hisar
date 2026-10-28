package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemAndroidApp
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemField
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.VaultDestinations
import com.tryhisar.hisar.model.OrganizationSummary
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runTest

class ItemModelTests {
    private fun AppModel.findSummary(vaultID: String, itemID: String): ItemDetail? =
        unlockedData.value.listAllItemReference(vaultID).firstOrNull { it.item.id == itemID }?.item

    /** A login the user saved earlier, listed by the refresh that unlocks the model. */
    private fun AppModelHarness.seedLogin(item: ItemDetail): ItemDetail {
        backend.seedSignedIn()
        return backend.saveItem("personal", item)
    }

    @Test
    fun testMovePreservesIdentityContentsFavoritesAndAutoFill() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.favorites.setItemFavorite("personal", "github", true)
        val original = model.item.loadItem("personal", "github")
        val moved = model.item.moveItem("personal", "github", "family")
        assertEquals(original, moved)
        assertFalse(model.unlockedData.value.listAllItemReference("personal").any { it.item.id == "github" })
        assertTrue(model.unlockedData.value.listAllItemReference("family").any { it.item.id == "github" && it.item.isFavorite })
        assertTrue(model.unlockedData.value.allPasskey.any { it.itemID == "github" && it.vaultID == "family" })
    }

    @Test
    fun testMoveFiltersPermissionsAndKeepsTheSourceOnFailure() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.seedSignedIn()
        harness.backend.allVault += VaultSummary(id = "reader", name = "Read only", type = "shared", role = "reader")
        val model = harness.startModel().also { it.refreshAll() }
        assertEquals(listOf("family"), model.item.moveDestinations("personal").map { it.id })
        assertTrue(model.item.moveDestinations("reader").isEmpty())
        harness.backend.moveErrorMessage = "The server could not be reached."
        assertNull(model.item.moveItem("personal", "github", "family"))
        assertEquals("The server could not be reached.", model.errorMessage.value)
        assertTrue(model.unlockedData.value.listAllItemReference("personal").any { it.item.id == "github" })
        harness.backend.moveErrorMessage = null
        assertNotNull(model.item.moveItem("personal", "github", "family"))
    }

    @Test
    fun testMoveDestinationsKeepPrivacyContextAndDuplicateNames() {
        val groups = VaultDestinations.groups(listOf(
            VaultSummary(id = "shared-a", name = "Production", type = "shared"),
            VaultSummary(id = "shared-b", name = "Production", type = "shared"),
            VaultSummary(id = "org", name = "Production", type = "organization", organizationID = "northwind"),
        ), listOf(OrganizationSummary(id = "northwind", name = "Northwind")))
        assertEquals(listOf("Northwind / Production", "Shared vault / Production (shared-a)", "Shared vault / Production (shared-b)"),
            groups.flatMap { it.destinations }.map { it.label })
        assertEquals("Shared through teams in Northwind.", groups.first().destinations.first().context)
    }

    @Test
    fun testConvertToSecureNoteRefusesALoginWithAPassword() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val converted = model.item.convertToSecureNote("personal", "github")
        assertNull(converted)
        assertNotNull(model.errorMessage.value)
        assertEquals(true, model.findSummary("personal", "github")?.isLogin)
    }

    @Test
    fun testConvertToSecureNoteTakesALoginWithOnlyCustomFields() = runTest {
        val harness = AppModelHarness(this)
        val login = harness.seedLogin(ItemDetail(title = "Router", allField = listOf(ItemField("PIN", "0000", concealed = true)), canConvertToSecureNote = true))
        val model = harness.startModel().also { it.refreshAll() }
        val converted = model.item.convertToSecureNote("personal", login.id)
        assertEquals(true, converted?.isSecureNote)
        assertEquals(1, converted?.allField?.size)
    }

    @Test
    fun testConvertToSecureNoteUpdatesTheItem() = runTest {
        val harness = AppModelHarness(this)
        val login = harness.seedLogin(ItemDetail(title = "Door codes", notes = "Front 1234", canConvertToSecureNote = true))
        val model = harness.startModel().also { it.refreshAll() }
        val converted = assertNotNull(model.item.convertToSecureNote("personal", login.id), "not converted")
        assertTrue(converted.isSecureNote)
        assertFalse(converted.canConvertToSecureNote)
        val summary = model.findSummary("personal", login.id)
        assertEquals(ItemDetail.CATEGORY_SECURE_NOTE, summary?.category)
        assertEquals(false, summary?.canConvertToSecureNote)
        assertEquals("Converted to a secure note.", model.notice.value?.message)
    }

    @Test
    fun testTheItemDetailCarriesTheStar() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.seedSignedIn()
        harness.backend.allFavoriteRecordID = mutableSetOf("personal/github")
        val model = harness.startModel().also { it.refreshAll() }
        val item = model.item.loadItem("personal", "github")
        assertEquals(true, item?.isFavorite)
    }

    @Test
    fun testAppDeletesPasskeys() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        assertEquals(listOf(FakeBackend.gitHubPasskey), model.unlockedData.value.allPasskey)

        val loaded = assertNotNull(model.itemEditor.loadItem("personal", "github"))
        val draft = ItemEditorDraft.open(loaded, isNew = false).withItem { it.copy(title = "GitHub work") }
        val saved = assertNotNull(model.itemEditor.saveItem("personal", draft.buildItem()))
        assertEquals("GitHub work", saved.title)
        assertEquals(listOf(FakeBackend.gitHubPasskey.credentialID), saved.allPasskey.map { it.credentialID }, "editing keeps the same credential")
        assertEquals(loaded.allPasskey, saved.allPasskey)
        assertEquals(saved.allPasskey, model.item.loadItem("personal", "github")?.allPasskey)
        assertEquals(saved.allPasskey.map { it.credentialID }, model.unlockedData.value.allPasskey.map { it.credentialID })
        assertEquals(saved.allPasskey, model.findSummary("personal", "github")?.allPasskey)

        val afterDelete = model.item.deletePasskey("personal", "github", FakeBackend.gitHubPasskey.credentialID)
        assertEquals(emptyList(), afterDelete?.allPasskey)
        assertEquals(emptyList(), model.unlockedData.value.allPasskey)
        assertEquals(emptyList(), model.findSummary("personal", "github")?.allPasskey)
        assertEquals("Passkey deleted.", model.notice.value?.message)

        val failedDelete = model.item.deletePasskey("personal", "github", "unknown")
        assertNull(failedDelete)
        assertEquals("passkey not found", model.errorMessage.value)
    }

    @Test
    fun testOneTimePasswordIsSavedLoadedAndCopied() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        assertEquals(true, model.findSummary("personal", "github")?.hasOneTimePassword)
        assertEquals("", model.findSummary("personal", "github")?.oneTimePassword, "summaries never carry the secret")
        assertEquals(FakeBackend.oneTimeCode, model.item.loadOneTimeCode("personal", "github"))
        assertNull(model.item.loadOneTimeCode("personal", "bank"))
        model.copyField("personal", "github", "one-time code", "One-time code")
        assertEquals(listOf("github.one-time code"), harness.backend.allCopiedField)
        assertEquals("One-time code copied. It clears in about 2 minutes.", model.notice.value?.message)

        val draft = ItemDetail(
            title = "Mastodon",
            username = "alice",
            allURL = listOf("mastodon.social"),
            password = "secret",
            oneTimePassword = "jbsw y3dp ehpk 3pxp",
        )
        val saved = assertNotNull(model.itemEditor.saveItem("personal", draft))
        assertEquals("otpauth://totp/?secret=JBSWY3DPEHPK3PXP", saved.oneTimePassword)
        val summary = assertNotNull(model.findSummary("personal", saved.id))
        assertEquals("", summary.oneTimePassword, "new summaries never carry the secret")
        assertTrue(summary.hasOneTimePassword)
        assertEquals(FakeBackend.oneTimeCode, model.item.loadOneTimeCode("personal", saved.id))
    }

    @Test
    fun testRemovingAnAndroidAppUnlinksItAndKeepsTheOthers() = runTest {
        val harness = AppModelHarness(this)
        val bankApp = ItemAndroidApp("com.bank.app", "14:6D:E9")
        val otherApp = ItemAndroidApp("com.bank.business", "AA:BB:CC")
        harness.backend.seedSignedIn()
        harness.backend.addAndroidApp("personal", "bank", bankApp.packageName, bankApp.certificateSHA256)
        harness.backend.addAndroidApp("personal", "bank", otherApp.packageName, otherApp.certificateSHA256)
        val model = harness.startModel().also { it.refreshAll() }

        val updated = model.item.removeAndroidApp("personal", "bank", bankApp)
        assertEquals(listOf(otherApp), updated?.allAndroidApp)
        assertEquals(listOf(otherApp), model.item.loadItem("personal", "bank")?.allAndroidApp)
        assertEquals("Android app removed.", model.notice.value?.message)
    }
}
