package com.tryhisar.hisar.passkey

import android.content.Intent
import android.util.Base64
import androidx.compose.ui.test.ComposeTimeoutException
import androidx.compose.ui.test.hasTestTag
import androidx.test.core.app.ApplicationProvider
import androidx.test.filters.SdkSuppress
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.services.storage.TestStorage
import androidx.test.uiautomator.By
import androidx.test.uiautomator.BySelector
import androidx.test.uiautomator.StaleObjectException
import androidx.test.uiautomator.UiDevice
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.autofill.AndroidApp
import com.tryhisar.hisar.autofill.InstalledApps
import com.tryhisar.hisar.autofill.testBackend
import com.tryhisar.hisar.platform.EmulatorFingerprints
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
        addLogin(title = "Example", username = "passkey-user", password = "passkey-password-1", website = "login.example.com")
        // The app's lock screen would share its tags with the one Credential Manager opens.
        closeApp()
        turnOnHisarForPasskeys()
        application.statementByRelyingParty[RELYING_PARTY_ID] = buildStatement(callerApp)

        // This emulator has no screen lock, so only unlocking Hisar in the request itself verifies the user: Hisar would
        // otherwise ask the device owner, and fail, before it creates or signs.
        val userHandle = "passkey-user-handle".toByteArray()
        testBackend.lock()
        val registration = callCredentialManager(CredentialCallerActivity.MODE_CREATE_PASSKEY, buildCreateRequest(userHandle, "preferred")) {
            answerSheet("passkey-register-sheet", "Hisar") { device.hasObject(By.res("unlock-password")) }
            type(MASTER_PASSWORD, "unlock-password")
            tap("unlock-submit")
            waitFor("autofill-save-passkey")
            tap("autofill-passkey-new-login")
            saveScreenshot("passkey-save")
            tap("autofill-save-passkey")
        }
        assertTrue(registration, registration.startsWith("registered "))
        val registered = JSONObject(registration.removePrefix("registered "))
        val credentialID = decode(registered.getString("rawId"))
        val registrationResponse = registered.getJSONObject("response")
        checkClientData(registrationResponse, "webauthn.create", REGISTRATION_CHALLENGE)
        val authenticatorData = decode(registrationResponse.getString("authenticatorData"))
        assertArrayEquals("the RP ID hash", sha256(RELYING_PARTY_ID.toByteArray()), authenticatorData.copyOfRange(0, 32))
        assertArrayEquals("the credential ID in the attested credential data", credentialID, authenticatorData.copyOfRange(55, 55 + credentialID.size))
        assertEquals(-7, registrationResponse.getInt("publicKeyAlgorithm"))
        val publicKey = KeyFactory.getInstance("EC").generatePublic(X509EncodedKeySpec(decode(registrationResponse.getString("publicKey"))))

        testBackend.lock()
        val assertion = callCredentialManager(CredentialCallerActivity.MODE_GET_PASSKEY, buildGetRequest(credentialID, "preferred")) {
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
        assertArrayEquals(userHandle, decode(assertionResponse.getString("userHandle")))
        val clientDataJSON = checkClientData(assertionResponse, "webauthn.get", ASSERTION_CHALLENGE)
        val signedData = decode(assertionResponse.getString("authenticatorData")) + sha256(clientDataJSON)
        assertTrue("the signature verifies with the registered key", verify(publicKey, signedData, decode(assertionResponse.getString("signature"))))

        val personal = testBackend.listAllVault().single { it.name == "Personal" }
        val example = testBackend.listAllItem(personal.id).single { it.title == "Example" }
        testBackend.addAndroidApp(personal.id, example.id, callerApp.packageName, callerApp.certificateSHA256)
        val password = callCredentialManager(CredentialCallerActivity.MODE_GET_PASSWORD, "") {
            answerSheet("password-sheet", "passkey-user") { device.hasObject(callerOutcome) }
        }
        assertEquals("the login linked to the app, not the passkey's new login", "password passkey-user passkey-password-1", password)

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

    /**
     * With Hisar unlocked, using or creating a passkey asks the device owner first, by fingerprint here, unless the site
     * discourages user verification or the user unlocked Hisar in this request; declining fails the request. API 37,
     * where enrolling a fingerprint through Settings is known to work.
     */
    @Test
    @SdkSuppress(minSdkVersion = 37)
    fun testPasskeyAsksTheDeviceOwnerWhileUnlocked() {
        val fingerprints = EmulatorFingerprints()
        fingerprints.setPin()
        try {
            fingerprints.enroll(1)
            launch()
            signUp(readArgument("HISAR_UI_TEST_PASSKEY_UV_INVITE_LINK"), readArgument("HISAR_UI_TEST_PASSKEY_UV_EMAIL"), "Android Passkey UV", isBiometricOfferExpected = true)
            tap("biometric-offer-turn-on")
            composeRule.waitUntil("Hisar shows its biometric prompt", 20_000) { fingerprints.isPromptShown(100) }
            fingerprints.touch()
            waitFor("vault-Personal", 20_000)
            closeApp()
            turnOnHisarForPasskeys()
            application.statementByRelyingParty[RELYING_PARTY_ID] = buildStatement(callerApp)

            val createPrompt = By.text("Create a passkey for $RELYING_PARTY_ID")
            val registration = callCredentialManager(CredentialCallerActivity.MODE_CREATE_PASSKEY, buildCreateRequest("uv-user-handle".toByteArray(), "preferred")) {
                answerSheet("passkey-uv-register-sheet", "Hisar") { device.hasObject(By.res("autofill-save-passkey")) }
                tap("autofill-passkey-new-login")
                tap("autofill-save-passkey")
                answerDeviceOwnerPrompt(fingerprints, createPrompt, "passkey-uv-create-prompt")
            }
            assertTrue(registration, registration.startsWith("registered "))
            val registered = JSONObject(registration.removePrefix("registered ")).getJSONObject("response")
            assertTrue("the creation says the user was verified", isUserVerifiedFlagSet(decode(registered.getString("authenticatorData"))))
            val publicKey = KeyFactory.getInstance("EC").generatePublic(X509EncodedKeySpec(decode(registered.getString("publicKey"))))

            val signInPrompt = By.text("Use your passkey for $RELYING_PARTY_ID")
            val assertion = callCredentialManager(CredentialCallerActivity.MODE_GET_PASSKEY, buildGetRequest(null, "preferred")) {
                answerSheet("passkey-uv-sign-in-sheet", "passkey-user") { device.hasObject(signInPrompt) || device.hasObject(callerOutcome) }
                answerDeviceOwnerPrompt(fingerprints, signInPrompt, "passkey-uv-sign-in-prompt")
            }
            checkSignedIn(assertion, publicKey)

            val declined = callCredentialManager(CredentialCallerActivity.MODE_GET_PASSKEY, buildGetRequest(null, "preferred")) {
                answerSheet("passkey-uv-decline-sheet", "passkey-user") { device.hasObject(signInPrompt) || device.hasObject(callerOutcome) }
                check(device.hasObject(signInPrompt)) { "no device owner prompt to decline; ${dumpWindow("passkey-uv-decline")}" }
                device.pressBack()
            }
            assertTrue("declining the device owner check cancels the request: $declined", declined.startsWith("error ") && "CANCEL" in declined.uppercase())

            val discouraged = callCredentialManager(CredentialCallerActivity.MODE_GET_PASSKEY, buildGetRequest(null, "discouraged")) {
                answerSheet("passkey-uv-discouraged-sheet", "passkey-user") { device.hasObject(callerOutcome) || device.hasObject(signInPrompt) }
                assertFalse("a site that discourages user verification gets no device owner prompt", device.hasObject(signInPrompt))
            }
            saveScreenshot("passkey-uv-discouraged-signed-in")
            checkSignedIn(discouraged, publicKey)

            testBackend.lock()
            val afterUnlock = callCredentialManager(CredentialCallerActivity.MODE_GET_PASSKEY, buildGetRequest(null, "preferred")) {
                answerSheet("passkey-uv-unlock-sheet", "Unlock Hisar.*") { fingerprints.isPromptShown(100) }
                fingerprints.touch()
                answerSheet("passkey-uv-after-unlock-sheet", "passkey-user") { device.hasObject(callerOutcome) || device.hasObject(signInPrompt) }
                assertFalse("the unlock in this request verified the user", device.hasObject(signInPrompt))
            }
            saveScreenshot("passkey-uv-after-unlock-signed-in")
            checkSignedIn(afterUnlock, publicKey)
        } finally {
            fingerprints.clearPin()
        }
    }

    // The prompt is a secure window, which screenshots show black, so a missing prompt saves the window hierarchy instead.
    private fun answerDeviceOwnerPrompt(fingerprints: EmulatorFingerprints, prompt: BySelector, name: String) {
        check(waitUntil(30_000) { device.hasObject(prompt) }) { "no device owner prompt; ${dumpWindow(name)}" }
        fingerprints.touch(prompt = prompt)
    }

    private fun checkSignedIn(outcome: String, publicKey: PublicKey) {
        assertTrue(outcome, outcome.startsWith("signed-in "))
        val response = JSONObject(outcome.removePrefix("signed-in ")).getJSONObject("response")
        val clientDataJSON = checkClientData(response, "webauthn.get", ASSERTION_CHALLENGE)
        val signedData = decode(response.getString("authenticatorData")) + sha256(clientDataJSON)
        assertTrue("the signature verifies with the registered key", verify(publicKey, signedData, decode(response.getString("signature"))))
    }

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
        check(waitUntil(10_000) { !device.hasObject(callerOutcome) }) { "the last outcome stays; ${dumpWindow("caller-start")}" }
        answerSheets()
        check(waitUntil(60_000) { device.hasObject(callerOutcome) }) { "no outcome; ${dumpWindow("caller-outcome")}" }
        return device.findObject(callerOutcome).text
    }

    /**
     * Confirms the system's sheet, Google's on API 37 and the platform's on API 34, or taps the entry whose text matches
     * [choice] when the sheet has nothing to confirm yet, until [isDone]. Each sheet asks differently, and some ask twice.
     */
    private fun answerSheet(screenshot: String, choice: String, isDone: () -> Boolean) {
        val confirm = By.pkg(SHEET_PACKAGE).text(Pattern.compile("Continue|Create|Sign in"))
        val entry = By.pkg(SHEET_PACKAGE).text(Pattern.compile(choice))
        var isScreenshotSaved = false
        val deadline = System.currentTimeMillis() + 60_000
        while (!isDone()) {
            check(System.currentTimeMillis() < deadline) { "the sheet did not lead on; ${dumpWindow(screenshot)}" }
            if (!waitUntil(2_000) { isDone() || device.hasObject(confirm) || device.hasObject(entry) }) continue
            val button = device.findObject(confirm) ?: device.findObject(entry) ?: continue
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
