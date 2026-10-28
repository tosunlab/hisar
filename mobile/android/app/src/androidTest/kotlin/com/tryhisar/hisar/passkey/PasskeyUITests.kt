package com.tryhisar.hisar.passkey

import android.app.PendingIntent
import android.content.Intent
import android.util.Base64
import androidx.compose.ui.test.ComposeTimeoutException
import androidx.compose.ui.test.hasTestTag
import androidx.test.core.app.ApplicationProvider
import androidx.test.filters.SdkSuppress
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.services.storage.TestStorage
import androidx.test.uiautomator.By
import androidx.test.uiautomator.StaleObjectException
import androidx.test.uiautomator.UiDevice
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.autofill.AndroidApp
import com.tryhisar.hisar.autofill.InstalledApps
import com.tryhisar.hisar.autofill.testBackend
import com.tryhisar.hisar.model.ItemDetail
import java.security.KeyFactory
import java.security.MessageDigest
import java.security.PublicKey
import java.security.Signature
import java.security.spec.X509EncodedKeySpec
import java.util.regex.Pattern
import org.json.JSONObject
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Registers a passkey, signs in with it and fills a password through Credential Manager, the system's sheets and Hisar's
 * provider, from a native app in the test APK's package. It checks the answers as a relying party would: the client data
 * names the app's signing certificate, and the ES256 signature over authenticatorData || SHA-256(clientDataJSON)
 * verifies with the registered key. The relying party's Digital Asset Links come from
 * [HisarTestApplication.statementByRelyingParty].
 *
 * The compose rule runs every composition in this process on its own clock, Hisar's credential screens included, and
 * advances it only while the rule waits. So every wait here goes through the rule, also for the sheet and the caller.
 */
@SdkSuppress(minSdkVersion = 34)
class PasskeyUITests : HisarUITestCase() {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val device = UiDevice.getInstance(instrumentation)
    private val application get() = ApplicationProvider.getApplicationContext<HisarTestApplication>()
    private val callerPackage get() = instrumentation.context.packageName
    private val callerApp: AndroidApp get() = checkNotNull(InstalledApps.readAndroidApp(application, callerPackage))
    private val callerOutcome = By.desc("caller-result").text(Pattern.compile("(registered|signed-in|password|error) .*", Pattern.DOTALL))

    @Test
    fun testRegisterPasskeyThenSignInWithIt() {
        launch()
        signUp(readArgument("HISAR_UI_TEST_PASSKEY_INVITE_LINK"), readArgument("HISAR_UI_TEST_PASSKEY_EMAIL"), "Android Passkey")
        addLogin(title = "Example", username = "app-user", password = "passkey-password-1", website = "login.example.com")
        val shared = testBackend.createVault("Family")
        val sharedLogin = testBackend.saveItem(
            shared.id,
            ItemDetail(title = "AAA shared", username = "passkey-user", allURL = listOf("https://$RELYING_PARTY_ID")),
        )
        // The app's lock screen would share its tags with the one Credential Manager opens.
        closeApp()
        turnOnHisarForPasskeys()
        application.statementByRelyingParty[RELYING_PARTY_ID] = buildStatement(callerApp)

        // Without a screen lock this ceremony can establish presence, not user verification.
        val userHandle = "passkey-user-handle".toByteArray()
        testBackend.lock()
        val registration = callCredentialManager(CredentialCallerActivity.MODE_CREATE_PASSKEY, buildCreateRequest(userHandle, "discouraged")) {
            answerSheet("passkey-register-sheet", "Hisar") { device.hasObject(By.res("unlock-password")) }
            type(MASTER_PASSWORD, "unlock-password")
            tap("unlock-submit")
            waitFor("autofill-save-passkey")
            assertTrue("a shared login with the requested username does not become the default", isChoiceSelected("autofill-passkey-new-login"))
            assertFalse("shared save requires selection", isChoiceSelected("autofill-passkey-login-AAA shared"))
            assertFalse("a Personal login for another user is not the default", isChoiceSelected("autofill-passkey-login-Example"))
            saveScreenshot("passkey-save")
            tap("autofill-save-passkey")
        }
        assertTrue(registration, registration.startsWith("registered "))
        val registered = JSONObject(registration.removePrefix("registered "))
        val credentialID = decode(registered.getString("rawId"))
        val registrationResponse = registered.getJSONObject("response")
        checkClientData(registrationResponse, "webauthn.create", REGISTRATION_CHALLENGE)
        val authenticatorData = decode(registrationResponse.getString("authenticatorData"))
        assertFalse("unlocking Hisar is not verification for the final ceremony", isUserVerifiedFlagSet(authenticatorData))
        assertArrayEquals("the RP ID hash", sha256(RELYING_PARTY_ID.toByteArray()), authenticatorData.copyOfRange(0, 32))
        assertArrayEquals("the credential ID in the attested credential data", credentialID, authenticatorData.copyOfRange(55, 55 + credentialID.size))
        assertEquals(-7, registrationResponse.getInt("publicKeyAlgorithm"))
        val publicKey = KeyFactory.getInstance("EC").generatePublic(X509EncodedKeySpec(decode(registrationResponse.getString("publicKey"))))
        assertEquals("default saves no credential into the shared login", 0, testBackend.getItem(shared.id, sharedLogin.id).allPasskey.size)

        val sharedRegistration = callCredentialManager(
            CredentialCallerActivity.MODE_CREATE_PASSKEY,
            buildCreateRequest("shared-user-handle".toByteArray(), "discouraged"),
        ) {
            answerSheet("passkey-shared-register-sheet", "Hisar") { device.hasObject(By.res("autofill-save-passkey")) }
            assertTrue("shared membership scope is visible", device.hasObject(By.text(Pattern.compile(".*Other members can sign in.*"))))
            tap("autofill-passkey-login-AAA shared")
            tap("autofill-save-passkey")
        }
        assertTrue(sharedRegistration, sharedRegistration.startsWith("registered "))
        assertEquals("explicit selection preserves shared save", 1, testBackend.getItem(shared.id, sharedLogin.id).allPasskey.size)

        testBackend.lock()
        val assertion = callCredentialManager(CredentialCallerActivity.MODE_GET_PASSKEY, buildGetRequest(credentialID, "discouraged")) {
            answerSheet("passkey-unlock-sheet", "Unlock Hisar.*") { device.hasObject(By.res("unlock-password")) }
            saveScreenshot("passkey-unlock")
            type(MASTER_PASSWORD, "unlock-password")
            tap("unlock-submit")
            answerSheet("passkey-sign-in-sheet", "passkey-user") { device.hasObject(callerOutcome) }
        }
        assertTrue(assertion, assertion.startsWith("signed-in "))
        val signedIn = JSONObject(assertion.removePrefix("signed-in "))
        assertArrayEquals(credentialID, decode(signedIn.getString("rawId")))
        val assertionResponse = signedIn.getJSONObject("response")
        assertFalse("the signed assertion does not fabricate UV", isUserVerifiedFlagSet(decode(assertionResponse.getString("authenticatorData"))))
        assertArrayEquals(userHandle, decode(assertionResponse.getString("userHandle")))
        val clientDataJSON = checkClientData(assertionResponse, "webauthn.get", ASSERTION_CHALLENGE)
        val signedData = decode(assertionResponse.getString("authenticatorData")) + sha256(clientDataJSON)
        assertTrue("the signature verifies with the registered key", verify(publicKey, signedData, decode(assertionResponse.getString("signature"))))

        val personal = testBackend.listAllVault().single { it.name == "Personal" }
        val example = testBackend.listAllItem(personal.id).single { it.title == "Example" }
        testBackend.addAndroidApp(personal.id, example.id, callerApp.packageName, callerApp.certificateSHA256)
        val password = callCredentialManager(CredentialCallerActivity.MODE_GET_PASSWORD, "") {
            answerSheet("password-sheet", "app-user") { device.hasObject(callerOutcome) }
        }
        assertEquals("the login linked to the app, not the passkey's new login", "password app-user passkey-password-1", password)

        application.statementByRelyingParty[RELYING_PARTY_ID] = buildStatement(AndroidApp("com.example.other", callerApp.certificateSHA256))
        val refused = callCredentialManager(
            CredentialCallerActivity.MODE_GET_PASSKEY,
            """{"rpId":"$RELYING_PARTY_ID","challenge":"$ASSERTION_CHALLENGE","userVerification":"preferred"}""",
        ) {
            // With nothing to offer, Google Play services shows "Sign in another way"; dismissing it fails the request.
            val offered = By.pkg(SHEET_PACKAGE).text("passkey-user")
            val dismiss = By.pkg(SHEET_PACKAGE).desc("Dismiss")
            check(waitUntil(30_000) { device.hasObject(callerOutcome) || device.hasObject(dismiss) || device.hasObject(offered) }) {
                "neither a sheet nor an outcome; ${dumpWindow("passkey-unrelated-app")}"
            }
            saveScreenshot("passkey-unrelated-app")
            assertFalse("the sheet offers no passkey to an app the relying party does not name", device.hasObject(offered))
            device.findObject(dismiss)?.click()
        }
        assertTrue("an app the relying party does not name gets no passkey: $refused", refused.startsWith("error "))

        launch()
        tap("vault-Personal")
        tap("item-$RELYING_PARTY_ID")
        waitFor("passkey-$RELYING_PARTY_ID")
        saveScreenshot("passkey-item-detail")
        tap("delete-passkey-$RELYING_PARTY_ID")
        tap("confirm-delete-passkey")
        composeRule.waitUntil("the passkey leaves the item", 30_000) { !exists(hasTestTag("passkey-$RELYING_PARTY_ID")) }
        assertEquals("deleting the passkey keeps the login", "passkey-user", readText("username-value"))
    }

    // An earlier process of Hisar numbered its entries in Credential Manager's sheet from 1 too; a sheet it filled may still be open.
    @Test
    @SdkSuppress(minSdkVersion = 34)
    fun testEntryOfAnEarlierProcessIsNeverRebound() {
        val earlierEntry = PendingIntent.getActivity(application, 1, Intent(application, CredentialActivity::class.java), PendingIntent.FLAG_MUTABLE)
        try {
            val entry = CredentialPick.buildPendingIntent(application, CredentialPick.Password("personal/github"))
            assertNotEquals(earlierEntry, entry)
            entry.cancel()
        } finally {
            earlierEntry.cancel()
        }
    }

    // Compose's selected state does not reach UiAutomator's isSelected; ChoiceRow marks the chosen row with a check icon.
    private fun isChoiceSelected(testTag: String) = device.findObject(By.res(testTag)).hasObject(By.desc("Selected"))

    // The flags byte follows the 32-byte RP ID hash; UV is bit 2.
    private fun isUserVerifiedFlagSet(authenticatorData: ByteArray) = authenticatorData[32].toInt() and 0x04 != 0

    private fun buildCreateRequest(userHandle: ByteArray, userVerification: String) =
        """{"rp":{"id":"$RELYING_PARTY_ID","name":"Example"},"user":{"id":"${encode(userHandle)}","name":"passkey-user","displayName":"Passkey User"},"challenge":"$REGISTRATION_CHALLENGE","pubKeyCredParams":[{"type":"public-key","alg":-7}],"authenticatorSelection":{"residentKey":"required","userVerification":"$userVerification"}}"""

    private fun buildGetRequest(credentialID: ByteArray?, userVerification: String): String {
        val allowCredentials = credentialID?.let { """"allowCredentials":[{"type":"public-key","id":"${encode(it)}"}],""" }.orEmpty()
        return """{"rpId":"$RELYING_PARTY_ID","challenge":"$ASSERTION_CHALLENGE",$allowCredentials"userVerification":"$userVerification"}"""
    }

    // The provider list of Settings > Passwords, passkeys & accounts, written as Settings writes it.
    private fun turnOnHisarForPasskeys() {
        val service = "${application.packageName}/${HisarCredentialProviderService::class.java.name}"
        device.executeShellCommand("settings put secure credential_service $service")
        device.executeShellCommand("settings put secure credential_service_primary $service")
    }

    private fun buildStatement(app: AndroidApp) =
        """[{"relation":["delegate_permission/common.handle_all_urls","delegate_permission/common.get_login_creds"],"target":{"namespace":"android_app","package_name":"${app.packageName}","sha256_cert_fingerprints":["${app.certificateSHA256}"]}}]"""

    /** Starts the caller with a request, lets [answerSheets] drive the system's sheets, and returns the caller's outcome. */
    private fun callCredentialManager(mode: String, requestJSON: String, answerSheets: () -> Unit): String {
        val intent = Intent().setClassName(callerPackage, CredentialCallerActivity::class.java.name)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
            .putExtra(CredentialCallerActivity.EXTRA_MODE, mode)
            .putExtra(CredentialCallerActivity.EXTRA_REQUEST_JSON, requestJSON)
            .putExtra(CredentialCallerActivity.EXTRA_USER_NAME, "passkey-user")
        instrumentation.context.startActivity(intent)
        check(waitUntil(15_000) { !device.hasObject(callerOutcome) }) { "the last outcome stays; ${dumpWindow("caller-start")}" }
        answerSheets()
        check(waitUntil(30_000) { device.hasObject(callerOutcome) }) { "no outcome; ${dumpWindow("caller-outcome")}" }
        return device.findObject(callerOutcome).text
    }

    /**
     * Confirms the system's sheet, Google's on API 37 and the platform's on API 34, or taps the entry whose text matches
     * [choice] when the sheet has nothing to confirm yet, until [isDone]. Each sheet asks differently, and some ask twice.
     */
    private fun answerSheet(screenshot: String, choice: String, isDone: () -> Boolean) {
        val confirm = By.pkg(SHEET_PACKAGE).text(Pattern.compile("Continue|Create|Sign in"))
        val entry = By.pkg(SHEET_PACKAGE).text(Pattern.compile(choice))
        val options = By.pkg(SHEET_PACKAGE).text(Pattern.compile("More options|Sign-in options|Sign in options"))
        var isScreenshotSaved = false
        val deadline = System.currentTimeMillis() + 60_000
        while (!isDone()) {
            check(System.currentTimeMillis() < deadline) { "the sheet did not lead on; ${dumpWindow(screenshot)}" }
            if (!waitUntil(2_000) { isDone() || device.hasObject(confirm) || device.hasObject(entry) || choice == "Search Hisar" && device.hasObject(options) }) continue
            val button = if (choice == "Search Hisar") {
                device.findObject(entry) ?: device.findObject(options) ?: device.findObject(confirm)
            } else {
                device.findObject(confirm) ?: device.findObject(entry)
            } ?: continue
            if (!isScreenshotSaved) {
                saveScreenshot(screenshot)
                isScreenshotSaved = true
            }
            try {
                button.click()
            } catch (_: StaleObjectException) {
                continue
            }
            waitUntil(5_000) { isDone() || !device.hasObject(confirm) && !device.hasObject(entry) }
        }
    }

    private fun waitUntil(timeoutMillis: Long, condition: () -> Boolean): Boolean = try {
        composeRule.waitUntil(timeoutMillis = timeoutMillis, condition = condition)
        true
    } catch (_: ComposeTimeoutException) {
        false
    }

    private fun dumpWindow(name: String): String {
        TestStorage().openOutputFile("android-$name.xml").use { device.dumpWindowHierarchy(it) }
        return "window saved as android-$name.xml"
    }

    private fun checkClientData(response: JSONObject, type: String, challenge: String): ByteArray {
        val clientDataJSON = decode(response.getString("clientDataJSON"))
        val clientData = JSONObject(String(clientDataJSON))
        assertEquals(type, clientData.getString("type"))
        assertEquals(challenge, clientData.getString("challenge"))
        assertEquals("the app's origin from its signing certificate", "android:apk-key-hash:${encode(fingerprint(callerApp.certificateSHA256))}", clientData.getString("origin"))
        assertEquals(callerPackage, clientData.getString("androidPackageName"))
        return clientDataJSON
    }

    private fun verify(publicKey: PublicKey, data: ByteArray, signature: ByteArray): Boolean =
        Signature.getInstance("SHA256withECDSA").run {
            initVerify(publicKey)
            update(data)
            verify(signature)
        }

    private fun fingerprint(hex: String) = hex.split(":").map { it.toInt(16).toByte() }.toByteArray()

    private fun sha256(data: ByteArray): ByteArray = MessageDigest.getInstance("SHA-256").digest(data)

    private fun encode(data: ByteArray): String = Base64.encodeToString(data, Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP)

    private fun decode(text: String): ByteArray = Base64.decode(text, Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP)

    private companion object {
        const val RELYING_PARTY_ID = "example.com"
        const val REGISTRATION_CHALLENGE = "cmVnaXN0ZXI"
        const val ASSERTION_CHALLENGE = "c2lnbiBpbg"

        // Every package but Hisar and the caller, whose own screens show the same names.
        val SHEET_PACKAGE: Pattern = Pattern.compile("(?!com\\.tryhisar\\.hisar).*")
    }
}
