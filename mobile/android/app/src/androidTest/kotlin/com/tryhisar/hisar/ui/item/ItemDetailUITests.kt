package com.tryhisar.hisar.ui.item

import android.content.ClipboardManager
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.longClick
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTouchInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.app.EditorRequest
import com.tryhisar.hisar.model.ItemAPICredential
import com.tryhisar.hisar.model.ItemAddress
import com.tryhisar.hisar.model.ItemCard
import com.tryhisar.hisar.model.ItemDatabase
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemField
import com.tryhisar.hisar.model.ItemIdentity
import com.tryhisar.hisar.model.ItemServer
import com.tryhisar.hisar.model.ItemSoftwareLicense
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.platform.targetContext
import com.tryhisar.hisar.platform.waitUntil
import java.nio.ByteBuffer
import java.time.Instant
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** The item detail screen on the real HisarKit, against the server scripts/test-android-ui.sh starts. */
@RunWith(AndroidJUnit4::class)
class ItemDetailUITests : HisarUITestCase() {
    private val application get() = targetContext.applicationContext as HisarTestApplication
    private val clipboardManager get() = targetContext.getSystemService(ClipboardManager::class.java)
    private var host: ItemDetailHost? = null

    private val clipboardText: String?
        get() = clipboardManager.primaryClip?.takeIf { it.itemCount > 0 }?.getItemAt(0)?.text?.toString()

    @After
    fun closeHost() {
        host?.close()
    }

    /** Signs up with the account scripts/test-android-ui.sh made for [prefix], and gives the Personal vault's ID. */
    private fun signUp(prefix: String): String {
        val backend = application.backend
        val signedUp = backend.signUp(
            readArgument("HISAR_UI_TEST_${prefix}INVITE_LINK"), readArgument("HISAR_UI_TEST_${prefix}EMAIL"), "Android Item Detail", MASTER_PASSWORD,
        )
        assertTrue("signed up and unlocked: ${signedUp.status}", signedUp.status.isUnlocked)
        return backend.listAllVault().single { it.name == "Personal" }.id
    }

    private fun openHost(): ItemDetailHost = ItemDetailHost(application.backend, application).also { host = it }

    @Test
    fun testLoginDetailCopiesItsPasswordAndShowsItsCode() {
        val personalID = signUp("ITEM_DETAIL_")
        val login = application.backend.saveItem(
            personalID,
            ItemDetail(
                title = "GitHub", username = "octo-alice", password = PASSWORD, allURL = listOf("github.com"),
                notes = "Work account", allTag = listOf("work"), oneTimePassword = SETUP_KEY,
                allField = listOf(ItemField("Recovery code", "RC-1234", concealed = true)),
            ),
        )
        val host = openHost()
        host.show(personalID, login.id)

        waitFor("password-value")
        assertNotEquals(PASSWORD, readText("password-value"))
        tap("reveal-password")
        assertEquals(PASSWORD, readText("password-value"))
        assertTrue(exists(hasTestTag("item-tag-work")))
        assertEquals("Personal · Login", readText("item-subtitle"))

        tap("copy-password")
        waitUntil(10_000, "the clipboard holds the password") { clipboardText == PASSWORD }
        waitFor(hasText("Password copied. It clears in about 2 minutes."))
        saveScreenshot("item-detail-login-copied")

        // The test's frame clock drives the row's timers, so a code may show a moment after its window starts.
        val startedAt = Instant.now().minusSeconds(1)
        waitFor("one-time-code-value")
        composeRule.waitUntil("the current code shows", 10_000) {
            val shown = composeRule.onNode(hasTestTag("one-time-code-value")).fetchSemanticsNode().config[SemanticsProperties.Text].joinToString("")
            shown.replace(" ", "") in listOf(computeCode(Instant.now().minusSeconds(1)), computeCode(Instant.now()))
        }
        assertTrue(exists(hasTestTag("one-time-code-seconds")))
        tap("copy-one-time-code")
        waitUntil(10_000, "the clipboard holds the code") { clipboardText?.length == 6 }
        assertTrue("${clipboardText} is not the current code", clipboardText in listOf(computeCode(startedAt), computeCode(Instant.now())))

        tap("copy-secret-reference")
        waitUntil(10_000, "the clipboard holds the password's reference") { clipboardText == "hisar://Personal/GitHub/password" }
        waitFor(hasText("Secret reference copied."))

        tap("edit-item")
        assertEquals(listOf<EditorRequest>(EditorRequest.Edit(personalID, login.id)), host.allEditorRequest)
    }

    @Test
    fun testRemovingAnAndroidAppUnlinksTheLogin() {
        val personalID = signUp("ITEM_APP_LINK_")
        val backend = application.backend
        val login = backend.saveItem(personalID, ItemDetail(title = "Bank", username = "alice", password = PASSWORD, allURL = listOf("https://bank.example")))
        backend.addAndroidApp(personalID, login.id, "com.bank.app", "14".repeat(32))
        val host = openHost()
        host.show(personalID, login.id)

        waitFor("android-app-com.bank.app")
        saveScreenshot("item-detail-android-app")
        assertFalse("the row leaves out the certificate", exists(hasText("14".repeat(32), substring = true)))
        composeRule.onNodeWithTag("android-app-com.bank.app").performTouchInput { longClick() }
        composeRule.onNode(hasText("Copy certificate")).performClick()
        waitUntil(10_000, "the clipboard holds the certificate") { clipboardText == "14".repeat(32) }
        tap("remove-android-app-com.bank.app")
        confirm("Remove App", excludingTag = "remove-android-app-com.bank.app")
        // The notice shows for 2.5 seconds, so it is read while it shows, not after the slower wait for the row.
        waitFor(hasText("Android app removed."))
        composeRule.waitUntil("the app's row goes", 30_000) { !exists(hasTestTag("android-app-com.bank.app")) }
        assertEquals(emptyList<Any>(), backend.getItem(personalID, login.id).allAndroidApp)
        assertEquals(listOf("https://bank.example"), backend.getItem(personalID, login.id).allURL)
    }

    @Test
    fun testEveryCategoryShowsItsSections() {
        val personalID = signUp("ITEM_CATEGORIES_")
        val backend = application.backend
        val allItem = saveEveryCategory(backend, personalID)
        val host = openHost()

        for ((name, item) in allItem) {
            host.show(personalID, item.id)
            waitFor(hasText(item.title))
            when (name) {
                "secure-note" -> assertEquals("4821-9930", readText("secure-note-notes"))
                "credit-card" -> {
                    waitFor(hasText("Visa •••• 1111", substring = true))
                    assertFalse("the security code is shown before it is revealed", exists(hasText("123")))
                }
                "identity" -> {
                    assertFalse("the passport number is shown before it is revealed", exists(hasText("U1234567")))
                    tap("reveal-passport number")
                    waitFor(hasText("U1234567"))
                }
                "api-credential" -> {
                    assertFalse("the secret is shown before it is revealed", exists(hasText("sk_live_from_the_phone")))
                    assertEquals("This credential has expired", readText("card-warning-Expiry"))
                }
                "address" -> assertTrue("the street is not shown", exists(hasText("1 Main St")))
                "software-license" -> {
                    assertFalse("the license key is shown before it is revealed", exists(hasText("LICENSE-FROM-THE-PHONE")))
                    assertTrue(exists(hasTestTag("open-download-url")))
                }
                "server" -> {
                    waitFor(hasText("build.example.com:2222 · deploy", substring = true))
                    assertTrue(exists(hasText("Admin console")))
                }
                "database" -> {
                    waitFor("database-connection-string")
                    assertFalse("the connection string is shown before it is revealed", exists(hasText("database-password-from-the-phone", substring = true)))
                }
            }
            saveScreenshot("item-detail-$name")
        }
    }

    private fun saveEveryCategory(backend: VaultBackend, vaultID: String): List<Pair<String, ItemDetail>> = listOf(
        "secure-note" to ItemDetail(category = ItemDetail.CATEGORY_SECURE_NOTE, title = "Recovery codes", notes = "4821-9930", allTag = listOf("backup")),
        "credit-card" to ItemDetail(
            category = ItemDetail.CATEGORY_CREDIT_CARD, title = "Visa",
            card = ItemCard(cardholder = "Alice Example", number = "4111 1111 1111 1111", expiry = "12/29", securityCode = "123", city = "Springfield", country = "US"),
        ),
        "identity" to ItemDetail(
            category = ItemDetail.CATEGORY_IDENTITY, title = "Passport",
            identity = ItemIdentity(firstName = "Alice", lastName = "Example", birthDate = "1990-04-01", email = "alice@example.com", passportNumber = "U1234567"),
        ),
        "api-credential" to ItemDetail(
            category = ItemDetail.CATEGORY_API_CREDENTIAL, title = "Billing key",
            apiCredential = ItemAPICredential(service = "Stripe", keyID = "key_1", secret = "sk_live_from_the_phone", host = "api.stripe.com", expiry = "2020-01-01"),
        ),
        "address" to ItemDetail(
            category = ItemDetail.CATEGORY_ADDRESS, title = "Home",
            address = ItemAddress(street = "1 Main St", city = "Springfield", region = "IL", postalCode = "62701", country = "US"),
        ),
        "software-license" to ItemDetail(
            category = ItemDetail.CATEGORY_SOFTWARE_LICENSE, title = "Sketch",
            softwareLicense = ItemSoftwareLicense(product = "Sketch", version = "100", licenseKey = "LICENSE-FROM-THE-PHONE", licensedTo = "Alice Example", downloadURL = "https://www.sketch.com/downloads/"),
        ),
        "server" to ItemDetail(
            category = ItemDetail.CATEGORY_SERVER, title = "Build server", username = "deploy", password = "server-password-from-the-phone",
            allURL = listOf("https://build.example.com/admin"), server = ItemServer(host = "build.example.com", port = "2222"),
        ),
        "database" to ItemDetail(
            category = ItemDetail.CATEGORY_DATABASE, title = "Prod db", username = "app", password = "database-password-from-the-phone",
            database = ItemDatabase(engine = "postgresql", host = "db.example.com", port = "5432", name = "app", options = "sslmode=require"),
        ),
        "login" to ItemDetail(
            title = "Router", allField = listOf(ItemField("Admin PIN", "0000", concealed = true)), notes = "Behind the TV", canConvertToSecureNote = true,
        ),
    ).map { (name, item) -> name to backend.saveItem(vaultID, item) }

    companion object {
        private const val PASSWORD = "ui-test-password-1"

        // RFC 6238's test secret "12345678901234567890", typed the way a site shows a setup key.
        private const val SETUP_KEY = "gezd gnbv gy3t qojq gezd gnbv gy3t qojq"

        private fun computeCode(date: Instant): String {
            val mac = Mac.getInstance("HmacSHA1").apply { init(SecretKeySpec("12345678901234567890".toByteArray(), "HmacSHA1")) }
            val digest = mac.doFinal(ByteBuffer.allocate(8).putLong(date.epochSecond / 30).array())
            val offset = digest[19].toInt() and 0x0f
            val binary = ByteBuffer.wrap(digest, offset, 4).int and 0x7fffffff
            return "%06d".format(binary % 1_000_000)
        }
    }
}
