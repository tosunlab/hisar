package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemDetail
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest

/** Android only: the Vaults and Search tabs' screens live in the app model, so AutoFill can open the editor on Vaults. */
class VaultsNavigationTests {
    @Test
    fun testARequestedEditorOpensOnTheVaultsTabOnceUnlocked() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.seedSignedIn(state = "locked")
        val model = harness.startModel()
        model.selectedTab.value = Tab.SETTINGS
        val request = EditorRequest.New("personal", ItemDetail(title = "Example", username = "alice"))
        model.navigation.requestEditor(request)
        runCurrent()
        assertTrue(model.navigation.vaults.path.value.isEmpty(), "a locked app opens nothing")

        model.lock.unlock(FakeBackend.PASSWORD)
        runCurrent()
        assertEquals(Tab.VAULTS, model.selectedTab.value)
        assertEquals(listOf<VaultsDestination>(VaultsDestination.Editor(request, opensSavedItem = true)), model.navigation.vaults.path.value)

        val editor = model.navigation.vaults.path.value.single() as VaultsDestination.Editor
        model.navigation.finishEditor(model.navigation.vaults, editor, savedItemID = "item-1")
        assertEquals(listOf<VaultsDestination>(VaultsDestination.Item("personal", "item-1")), model.navigation.vaults.path.value)
        assertEquals(emptyList(), model.navigation.allRequestedEditor.value, "a closed editor's request is done")
    }

    @Test
    fun testAVaultThatGoesAwayTakesTheVaultsTabToItsRoot() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val vaults = model.navigation.vaults
        vaults.push(VaultsDestination.Vault("family"))
        vaults.push(VaultsDestination.Item("family", "netflix"))
        model.vaults.createVault("Streaming")
        runCurrent()
        assertEquals(2, vaults.path.value.size, "a new vault keeps the screens")

        model.vaults.leaveVault("family")
        runCurrent()
        assertTrue(vaults.path.value.isEmpty())
    }

    @Test
    fun testLockingClosesThePushedScreens() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        model.navigation.vaults.push(VaultsDestination.Vault("personal"))
        model.navigation.search.push(VaultsDestination.Item("personal", "github"))
        model.lock()
        assertTrue(model.navigation.vaults.path.value.isEmpty())
        assertTrue(model.navigation.search.path.value.isEmpty())
    }
}
