package com.tryhisar.hisar.ui

import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeDown
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.model.Device
import com.tryhisar.hisar.model.FakeBackend
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** Android only: Settings → Devices reloads on a pull, as the iPhone's `.refreshable`, and keeps its list when that fails. */
@RunWith(AndroidJUnit4::class)
class DevicesUITests : HisarUITestCase() {
    private fun pullToRefresh() {
        composeRule.onAllNodes(hasScrollAction()).onFirst().performTouchInput { swipeDown() }
    }

    @Test
    fun testPullingDownReloadsTheDevicesAndKeepsThemWhenLoadingFails() {
        val backend = FakeBackend().apply {
            seedSignedIn()
            allDevice = mutableListOf(
                Device("this-phone", "Hisar on Android", "android/mobile", isCurrent = true),
                Device("mac", "MacBook", "macos/desktop", lastSeenAt = "2026-10-18T06:00:00Z"),
            )
        }
        launchApp(backend).use {
            waitFor("vaults-list")
            openSettings("open-devices")
            waitFor("device-MacBook")

            backend.allDevice.add(Device("laptop", "Work laptop", "linux/cli", lastSeenAt = "2026-10-18T07:00:00Z"))
            pullToRefresh()
            waitFor("device-Work laptop", 10_000)
            saveScreenshot("devices-refreshed")

            backend.listAllDeviceErrorMessage = "Hisar is offline."
            pullToRefresh()
            waitFor(hasText("Hisar is offline.", substring = true), 10_000)
            assertTrue("a failed refresh keeps the list", exists(hasTestTag("device-MacBook")) && exists(hasTestTag("device-Work laptop")))
            saveScreenshot("devices-refresh-failed")
        }
    }

    @Test
    fun testOpenedDeviceShowsSignOutFailureAndAllowsRetry() {
        val backend = FakeBackend().apply {
            seedSignedIn()
            allDevice = mutableListOf(Device("mac", "MacBook", "macos/desktop"))
            revokeDeviceErrorMessage = "Could not reach the Hisar server."
        }
        launchApp(backend).use {
            waitFor("vaults-list")
            openSettings("open-devices")
            tap("device-MacBook")
            tap("revoke-device")
            waitFor(hasText("Until then, it can still be unlocked while offline.", substring = true))
            tap("confirm-revoke-device")
            waitFor(hasText("Could not reach the Hisar server."))
            assertTrue(exists(hasTestTag("revoke-device")))
            assertTrue(backend.allDevice.any { it.id == "mac" })
            backend.revokeDeviceErrorMessage = null
            tap("revoke-device")
            tap("confirm-revoke-device")
            waitFor(hasText("No other device is signed in to your account."))
        }
    }

    @Test
    fun testMacApprovalsShowsFailureAndAllowsRetry() {
        val backend = FakeBackend().apply {
            seedSignedIn()
            macApprovalErrorMessage = "Could not save Mac approvals."
        }
        launchApp(backend).use {
            waitFor("vaults-list")
            openSettings("open-mac-approvals")
            val previous = backend.isMacApprovalAllowedValue
            tap("mac-approval-toggle")
            waitFor(hasText("Could not save Mac approvals."))
            assertTrue(backend.isMacApprovalAllowedValue == previous)
            backend.macApprovalErrorMessage = null
            tap("mac-approval-toggle")
            composeRule.waitUntil { backend.isMacApprovalAllowedValue != previous }
            assertTrue(!exists(hasText("Could not save Mac approvals.")))
        }
    }
}
