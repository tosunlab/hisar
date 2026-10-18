package com.tryhisar.hisar.passkey

import android.content.Intent
import android.util.Base64
import androidx.compose.ui.test.hasTestTag
import androidx.test.core.app.ApplicationProvider
import androidx.test.filters.SdkSuppress
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.services.storage.TestStorage
import androidx.test.uiautomator.By
import androidx.test.uiautomator.BySelector
import androidx.test.uiautomator.StaleObjectException
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.UiObject2
import androidx.test.uiautomator.Until
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.autofill.AndroidApp
import com.tryhisar.hisar.autofill.InstalledApps
import com.tryhisar.hisar.autofill.testBackend
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
 * Registers a passkey and signs in with it through Credential Manager, the system's sheets and Hisar's provider, from a
 * native app in the test APK's package. It checks the answers as a relying party would: the client data names the app's
 * signing certificate, and the ES256 signature over authenticatorData || SHA-256(clientDataJSON) verifies with the
 * registered key. The relying party's Digital Asset Links come from [HisarTestApplication.statementByRelyingParty].
 */
@SdkSuppress(minSdkVersion = 34)
class PasskeyUITests : HisarUITestCase() {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val device = UiDevice.getInstance(instrumentation)
    private val application get() = ApplicationProvider.getApplicationContext<HisarTestApplication>()
    private val callerPackage get() = instrumentation.context.packageName
    private val callerApp: AndroidApp get() = checkNotNull(InstalledApps.readAndroidApp(application, callerPackage))

    @Test
    fun testRegisterPasskeyThenSignInWithIt() {
        launch()
        signUp(readArgument("HISAR_UI_TEST_PASSKEY_INVITE_LINK"), readArgument("HISAR_UI_TEST_PASSKEY_EMAIL"), "Android Passkey")
        addLogin(title = "Example", username = "passkey-user", password = "passkey-password-1", website = "login.example.com")
        turnOnHisarForPasskeys()
        application.statementByRelyingParty[RELYING_PARTY_ID] = buildStatement(callerApp)

        val userHandle = "passkey-user-handle".toByteArray()
        val registration = callCredentialManager(
            "create",
            """{"rp":{"id":"$RELYING_PARTY_ID","name":"Example"},"user":{"id":"${encode(userHandle)}","name":"passkey-user","displayName":"Passkey User"},"challenge":"$REGISTRATION_CHALLENGE","pubKeyCredParams":[{"type":"public-key","alg":-7}],"authenticatorSelection":{"residentKey":"required","userVerification":"preferred"}}""",
        ) {
            chooseHisarInTheSheet("passkey-register-sheet")
            waitForApp("autofill-save-passkey")
            saveScreenshot("passkey-save")
            device.findObject(By.res("autofill-save-passkey")).click()
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
        val assertion = callCredentialManager(
            "get",
            """{"rpId":"$RELYING_PARTY_ID","challenge":"$ASSERTION_CHALLENGE","allowCredentials":[{"type":"public-key","id":"${encode(credentialID)}"}],"userVerification":"preferred"}""",
        ) {
            tapInTheSheet("passkey-unlock-sheet", By.text(Pattern.compile("Unlock Hisar.*")))
            waitForApp("unlock-password")
            saveScreenshot("passkey-unlock")
            device.findObject(By.res("unlock-password")).text = MASTER_PASSWORD
            device.findObject(By.res("unlock-submit")).click()
            tapInTheSheet("passkey-sign-in-sheet", By.text("passkey-user"))
        }
        assertTrue(assertion, assertion.startsWith("signed-in "))
        val signedIn = JSONObject(assertion.removePrefix("signed-in "))
        assertArrayEquals(credentialID, decode(signedIn.getString("rawId")))
        val assertionResponse = signedIn.getJSONObject("response")
        assertArrayEquals(userHandle, decode(assertionResponse.getString("userHandle")))
        val clientDataJSON = checkClientData(assertionResponse, "webauthn.get", ASSERTION_CHALLENGE)
        val signedData = decode(assertionResponse.getString("authenticatorData")) + sha256(clientDataJSON)
        assertTrue("the signature verifies with the registered key", verify(publicKey, signedData, decode(assertionResponse.getString("signature"))))

        application.statementByRelyingParty[RELYING_PARTY_ID] = buildStatement(AndroidApp("com.example.other", callerApp.certificateSHA256))
        val refused = callCredentialManager("get", """{"rpId":"$RELYING_PARTY_ID","challenge":"$ASSERTION_CHALLENGE","userVerification":"preferred"}""") {
            saveScreenshot("passkey-unrelated-app")
        }
        assertTrue("an app the relying party does not name gets no passkey: $refused", refused.startsWith("error "))

        launch()
        tap("vault-Personal")
        tap("item-Example")
        waitFor("passkey-$RELYING_PARTY_ID")
        saveScreenshot("passkey-item-detail")
        tap("delete-passkey-$RELYING_PARTY_ID")
        tap("confirm-delete-passkey")
        composeRule.waitUntil("the passkey leaves the item", 30_000) { !exists(hasTestTag("passkey-$RELYING_PARTY_ID")) }
        assertEquals("deleting the passkey keeps the login", "passkey-user", readText("username-value"))
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
        val intent = Intent().setClassName(callerPackage, PasskeyCallerActivity::class.java.name)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
            .putExtra(PasskeyCallerActivity.EXTRA_MODE, mode)
            .putExtra(PasskeyCallerActivity.EXTRA_REQUEST_JSON, requestJSON)
            .putExtra(PasskeyCallerActivity.EXTRA_USER_NAME, "passkey-user")
        instrumentation.context.startActivity(intent)
        answerSheets()
        val result = By.desc("caller-result").text(Pattern.compile("(registered|signed-in|error) .*", Pattern.DOTALL))
        return checkNotNull(device.wait(Until.findObject(result), 60_000)) { "no outcome; ${dumpWindow("caller-outcome")}" }.text
    }

    /** API 37 shows Google's sheet, API 34 the platform's; either way the user picks Hisar, and the sheet may ask again. */
    private fun chooseHisarInTheSheet(screenshot: String) {
        val hisarScreen = By.res("autofill-save-passkey")
        val choices = By.text(Pattern.compile("Continue|Create|Hisar"))
        var isScreenshotSaved = false
        val deadline = System.currentTimeMillis() + 60_000
        while (System.currentTimeMillis() < deadline && !device.hasObject(hisarScreen)) {
            val choice = device.wait(Until.findObject(choices), 2_000) ?: continue
            if (!isScreenshotSaved) {
                saveScreenshot(screenshot)
                isScreenshotSaved = true
            }
            dumpWindow("$screenshot-${System.currentTimeMillis()}")
            try {
                choice.click()
                device.wait(Until.gone(choices), 10_000)
            } catch (_: StaleObjectException) {
            }
        }
    }

    private fun tapInTheSheet(screenshot: String, selector: BySelector) {
        val target: UiObject2 = checkNotNull(device.wait(Until.findObject(selector), 30_000)) { "$selector is not in the sheet; ${dumpWindow(screenshot)}" }
        saveScreenshot(screenshot)
        target.click()
    }

    private fun waitForApp(tag: String) {
        checkNotNull(device.wait(Until.findObject(By.res(tag)), 30_000)) { "$tag did not appear; ${dumpWindow(tag)}" }
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
    }
}
