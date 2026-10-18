package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.Device
import com.tryhisar.hisar.model.DeviceIcon
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.HelpLogic
import com.tryhisar.hisar.model.PhoneApproval
import com.tryhisar.hisar.model.ThemeAppearanceName
import com.tryhisar.hisar.model.ThemeColorName
import com.tryhisar.hisar.model.VaultTheme
import com.tryhisar.hisar.model.backendJson
import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runTest

/** The Settings tab: the theme, devices, the Emergency Kit and Help. */
class SettingsModelTests {
    @Test
    fun testThemeStartsFromTheBackendAndIsSaved() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.theme = VaultTheme(ThemeAppearanceName.DARK, ThemeColorName.PURPLE)
        val model = harness.startUnlockedModel()
        assertEquals(VaultTheme(ThemeAppearanceName.DARK, ThemeColorName.PURPLE), model.theme.value)
        model.settings.setTheme(VaultTheme(ThemeAppearanceName.LIGHT, ThemeColorName.GREEN))
        assertEquals(VaultTheme(ThemeAppearanceName.LIGHT, ThemeColorName.GREEN), model.theme.value)
        assertEquals(VaultTheme(ThemeAppearanceName.LIGHT, ThemeColorName.GREEN), harness.backend.theme)
        assertTrue("setTheme" in harness.backend.allCallName)
    }

    @Test
    fun testThemeGoesBackWhenSavingFails() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        harness.backend.isThemeSaveFailing = true
        model.settings.setTheme(VaultTheme(ThemeAppearanceName.DARK, ThemeColorName.BLUE))
        assertEquals(VaultTheme.standard, model.theme.value)
        assertEquals("Could not save hisar.json", model.errorMessage.value)
    }

    @Test
    fun testThemeDecodesAndFallsBackToTheDefaults() {
        val theme = backendJson.decodeFromString<VaultTheme>("""{"appearance":"light","color":"blue"}""")
        assertEquals(VaultTheme(ThemeAppearanceName.LIGHT, ThemeColorName.BLUE), theme)
        val unknownTheme = backendJson.decodeFromString<VaultTheme>("""{"appearance":"sepia"}""")
        assertEquals(VaultTheme.standard, unknownTheme)
        assertTrue(VaultTheme.standard.isDark(isSystemDark = true), "System follows the device")
        assertFalse(VaultTheme.standard.isDark(isSystemDark = false), "System follows the device")
        assertTrue(VaultTheme(ThemeAppearanceName.DARK, ThemeColorName.NEUTRAL).isDark(isSystemDark = false))
    }

    @Test
    fun testDevicesAreListedAndAnotherOneSignsOut() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.allDevice = mutableListOf(
            Device(id = "this", name = "Hisar on Pixel", platform = "android/mobile", lastSeenAt = "2027-01-15T08:00:00Z", isCurrent = true),
            Device(id = "mac", name = "MacBook Pro", platform = "darwin/desktop", lastSeenAt = "2027-01-14T08:00:00Z"),
        )
        val model = harness.startUnlockedModel()
        assertEquals(listOf("this", "mac"), model.settings.loadAllDevice()?.map { it.id })
        val isMacSignedOut = model.settings.revokeDevice(harness.backend.allDevice[1])
        assertTrue(isMacSignedOut)
        assertEquals(listOf("this"), harness.backend.allDevice.map { it.id })
        assertEquals("MacBook Pro is signed out.", model.notice.value)
        val isThisPhoneSignedOut = model.settings.revokeDevice(harness.backend.allDevice[0])
        assertFalse(isThisPhoneSignedOut, "this device signs out from Settings instead")
    }

    @Test
    fun testDeviceDescribesItsPlatform() {
        val device = backendJson.decodeFromString<Device>(
            """{"id":"1","name":"build","platform":"linux/cli","lastSeenAt":"2027-01-15T08:00:00Z","isCurrent":false}""",
        )
        assertEquals("Command line on Linux", device.platformDescription)
        assertEquals(DeviceIcon.TERMINAL, device.icon)
        assertNotNull(device.lastSeenDate)
        assertEquals("macOS app", Device(id = "2", name = "Mac", platform = "darwin/desktop").platformDescription)
        assertEquals(DeviceIcon.LAPTOP, Device(id = "2", name = "Mac", platform = "darwin/desktop").icon)
        assertEquals(DeviceIcon.PHONE, Device(id = "3", name = "iPhone", platform = "ios/mobile").icon)
        val android = Device(id = "4", name = "Pixel", platform = "android/mobile")
        assertEquals("Android app", android.platformDescription)
        assertEquals(DeviceIcon.PHONE, android.icon)
        assertEquals(DeviceIcon.EXTENSION, Device(id = "5", name = "Chrome", platform = "darwin/extension").icon)
    }

    @Test
    fun testEmergencyKitNeedsTheMasterPasswordAndIsRemoved() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        assertNull(model.settings.exportEmergencyKit("wrong"))
        assertEquals("Incorrect master password.", model.errorMessage.value)
        val export = assertNotNull(model.settings.exportEmergencyKit(FakeBackend.PASSWORD))
        assertEquals(FakeBackend.SECRET_KEY, export.secretKey)
        assertTrue(File(export.emergencyKitPath).exists())
        assertTrue(
            export.emergencyKitPath.startsWith(System.getProperty("java.io.tmpdir")),
            "the kit is written to the app's temporary folder",
        )
        model.settings.removeEmergencyKit(export)
        assertFalse(File(export.emergencyKitPath).exists())
    }

    @Test
    fun testApprovalDetailLineNamesTheKeyOrCommand() {
        val sshApproval = PhoneApproval(
            requestID = "first", requesterDeviceName = "MacBook Pro", kind = PhoneApproval.KIND_SSH, appName = "Terminal",
            purpose = "push to github.com", keyName = "GitHub deploy key",
        )
        assertEquals("with “GitHub deploy key”", sshApproval.detailLine)
        val commandLine = PhoneApproval(
            requestID = "cli", requesterDeviceName = "Mac", kind = PhoneApproval.KIND_COMMAND_LINE, appName = "hisar",
            purpose = "use Hisar", commandLine = "hisar read",
        )
        assertEquals("hisar read", commandLine.detailLine)
    }

    @Test
    fun testDiagnosticsHoldVersionsServerAndDeviceOnly() {
        val diagnostics = HelpLogic.buildDiagnostics("1.2 (34)", "16", "Pixel 9", "https://api.tryhisar.com", "device-1")
        assertEquals("Hisar 1.2 (34)\nAndroid 16 on Pixel 9\nServer https://api.tryhisar.com\nDevice ID device-1", diagnostics)
        assertEquals(
            "Hisar 1.2 (34)\nAndroid 16 on Pixel 9\nServer not signed in",
            HelpLogic.buildDiagnostics("1.2 (34)", "16", "Pixel 9", "", null),
        )
        assertEquals("support@tryhisar.com", HelpLogic.SUPPORT_EMAIL)
    }
}
