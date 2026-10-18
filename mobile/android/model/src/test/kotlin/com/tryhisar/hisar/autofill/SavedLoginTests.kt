package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.app.AppModelHarness
import com.tryhisar.hisar.app.EditorRequest
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemAndroidApp
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.VaultStatus
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest

/** AutoFill's save prompt opens the app's editor with what the user typed (D15). */
class SavedLoginTests {
    private val app = AndroidApp("com.example.bank", "AB".repeat(32))

    @Test
    fun testSavePromptOpensTheEditorWithTheTypedLoginInThePersonalVault() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        SavedLogin("carol", "typed-password", SavedLogin.Place.Website("www.example.com")).openEditor(model)
        val request = model.requestedEditor.value as EditorRequest.New
        assertEquals("personal", request.vaultID)
        assertEquals(
            ItemDetail(category = ItemDetail.CATEGORY_LOGIN, title = "example.com", username = "carol", password = "typed-password", allURL = listOf("https://www.example.com")),
            request.draft,
        )
        assertNull(request.onSaved, "a website login needs nothing after saving")
    }

    @Test
    fun testSavePromptWaitsForTheUnlock() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = harness.startModel()
        launch { SavedLogin("carol", "typed-password", SavedLogin.Place.Website("example.com")).openEditor(model) }
        runCurrent()
        assertNull(model.requestedEditor.value, "the editor waits while Hisar is locked")
        model.lock.unlock(FakeBackend.PASSWORD)
        runCurrent()
        assertEquals("personal", (model.requestedEditor.value as EditorRequest.New).vaultID)
    }

    @Test
    fun testLoginSavedFromAnAppIsLinkedToItOnceSaved() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        SavedLogin("carol", "typed-password", SavedLogin.Place.App(app, "Example Bank")).openEditor(model)
        val request = model.requestedEditor.value as EditorRequest.New
        assertEquals("Example Bank", request.draft.title)
        assertEquals(emptyList(), request.draft.allURL)

        val saved = harness.backend.saveItem(request.vaultID, request.draft)
        request.onSaved?.invoke(saved)
        assertEquals(listOf(ItemAndroidApp(app.packageName, app.certificateSHA256)), harness.backend.getItem("personal", saved.id).allAndroidApp)
    }
}
