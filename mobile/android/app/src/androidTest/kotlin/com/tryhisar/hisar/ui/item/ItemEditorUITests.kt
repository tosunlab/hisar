package com.tryhisar.hisar.ui.item

import android.graphics.Bitmap
import androidx.activity.ComponentActivity
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isEnabled
import androidx.compose.ui.test.isSelected
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.test.core.app.ApplicationProvider
import androidx.test.espresso.Espresso
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.services.storage.TestStorage
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.HisarUITestCase.Companion.MASTER_PASSWORD
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.EditorRequest
import com.tryhisar.hisar.model.CalendarDate
import com.tryhisar.hisar.model.ChosenIcon
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemField
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.platform.requireArgument
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * The editor against the harness's real server: what is typed into each category's editor is what HisarKit then reads
 * back from the server, and edits and deletes reach it too.
 */
@RunWith(AndroidJUnit4::class)
class ItemEditorUITests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private lateinit var backend: VaultBackend
    private lateinit var model: AppModel
    private lateinit var vaultID: String
    private var request by mutableStateOf<EditorRequest?>(null)
    private var allFinished = mutableListOf<String?>()

    @After
    fun stopModel() {
        scope.cancel()
    }

    private fun signUp(accountPrefix: String) {
        val application = ApplicationProvider.getApplicationContext<HisarTestApplication>()
        backend = application.backend
        backend.signUp(requireArgument("HISAR_UI_TEST_${accountPrefix}INVITE_LINK"), requireArgument("HISAR_UI_TEST_${accountPrefix}EMAIL"), "Android Item Editor", MASTER_PASSWORD)
        model = AppModel(scope, Dispatchers.IO, application, application.events)
        runBlocking(Dispatchers.Main) {
            model.start { backend }
            model.refreshAll()
        }
        vaultID = model.unlockedData.value.allVault.single { it.name == "Personal" }.id
        composeRule.setContent {
            HisarTheme {
                request?.let { shown ->
                    key(shown) {
                        ItemEditorScreen(model, shown) { savedItemID ->
                            allFinished += savedItemID
                            request = null
                        }
                    }
                }
            }
        }
    }

    /** Opens a new item's editor, runs [fill], saves, and gives the saved item as HisarKit reads it from the server. */
    private fun create(category: String, screenshot: String, fill: () -> Unit): ItemDetail {
        open(EditorRequest.New(vaultID, ItemDetail(category = category)))
        fill()
        saveScreenshot(screenshot)
        val savedItemID = finishAfterTapping("editor-save")
        return backend.getItem(vaultID, checkNotNull(savedItemID) { "$category was not saved" })
    }

    private fun open(editorRequest: EditorRequest) {
        composeRule.runOnUiThread { request = editorRequest }
        composeRule.waitUntil(10_000) { exists("editor-title") }
    }

    /** Taps [tag] and gives what the editor finished with: the saved item's ID, or null. */
    private fun finishAfterTapping(tag: String): String? {
        val count = allFinished.size
        tap(tag)
        try {
            composeRule.waitUntil(30_000) { request == null && allFinished.size > count }
        } catch (error: Throwable) {
            saveScreenshot("failure-$tag")
            throw error
        }
        return allFinished.last()
    }

    /** Waits until the sheet or dialog holding [tag] has stopped moving, so a tap lands where the node is drawn. */
    private fun waitUntilStill(tag: String) {
        var lastBounds: Rect? = null
        composeRule.waitUntil(10_000) {
            val bounds = composeRule.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().firstOrNull()?.boundsInWindow
            val isStill = bounds != null && bounds == lastBounds
            lastBounds = bounds
            // Still means two equal samples, so the samples need real time between them.
            if (!isStill) Thread.sleep(100)
            isStill
        }
    }

    private fun exists(tag: String) = composeRule.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()

    /** The node tagged [tag], scrolled into view when it is in the editor's scrolling form. */
    private fun find(tag: String): SemanticsNodeInteraction {
        composeRule.waitUntil(10_000) { exists(tag) }
        val node = composeRule.onNodeWithTag(tag)
        if (composeRule.onAllNodes(hasTestTag(tag) and hasAnyAncestor(hasScrollAction())).fetchSemanticsNodes().isNotEmpty()) node.performScrollTo()
        return node
    }

    private fun tap(tag: String) {
        find(tag).performClick()
    }

    /** Closes the keyboard the field asked for, as HisarUITestCase.type does, so it cannot cover the next tap's target. */
    private fun type(tag: String, text: String) {
        find(tag).performTextInput(text)
        Espresso.closeSoftKeyboard()
    }

    private fun saveScreenshot(name: String) {
        composeRule.waitForIdle()
        val screenshot = checkNotNull(InstrumentationRegistry.getInstrumentation().uiAutomation.takeScreenshot()) { "no screenshot of $name" }
        TestStorage().openOutputFile("android-$name.png").use { screenshot.compress(Bitmap.CompressFormat.PNG, 100, it) }
    }

    @Test
    fun testEditorCreatesEveryCategoryOnTheServer() {
        signUp("ITEM_EDITOR_")

        val login = create(ItemDetail.CATEGORY_LOGIN, "editor-login") {
            type("editor-title", "GitHub")
            type("editor-username", "octo-alice")
            tap("editor-generate-password")
            composeRule.waitUntil(10_000) { composeRule.onAllNodes(hasTestTag("generator-use") and isEnabled()).fetchSemanticsNodes().isNotEmpty() }
            waitUntilStill("generator-use")
            saveScreenshot("editor-password-generator")
            tap("generator-use")
            composeRule.waitUntil(10_000) { !exists("generator-use") }
            type("editor-website-0", "github.com")
            tap("editor-add-website")
            type("editor-website-1", "gist.github.com")
            tap("editor-website-1-remove")
            type("editor-one-time-password", "jbsw y3dp ehpk 3pxp")
            tap("editor-add-field")
            type("editor-field-0-name", "Recovery PIN")
            type("editor-field-0-value", "2468")
            tap("editor-field-0-secret")
            tap("editor-add-field")
            type("editor-field-1-name", "Temporary")
            type("editor-field-1-value", "remove-me")
            tap("editor-field-1-remove")
            composeRule.waitUntil(5_000) { !exists("editor-field-1-name") }
            type("editor-new-tag", "work, code, draft")
            composeRule.onNodeWithTag("editor-new-tag").performImeAction()
            tap("editor-tag-draft-remove")
            tap("editor-icon")
            waitUntilStill("icon-done")
            tap("icon-color-blue")
            tap("icon-preset-code")
            composeRule.waitUntil(5_000) { composeRule.onAllNodes(hasTestTag("icon-preset-code") and isSelected()).fetchSemanticsNodes().isNotEmpty() }
            saveScreenshot("editor-icon-picker")
            tap("icon-done")
            composeRule.waitUntil(10_000) { !exists("icon-done") }
        }
        assertEquals("GitHub", login.title)
        assertEquals("octo-alice", login.username)
        assertEquals("the generated password is the default length", 24, login.password.length)
        assertEquals(listOf("github.com"), login.allURL)
        assertTrue(login.hasOneTimePassword)
        assertEquals(listOf(ItemField("Recovery PIN", "2468", concealed = true)), login.allField)
        assertEquals(listOf("work", "code"), login.allTag)
        assertEquals(ChosenIcon(preset = "code", color = "blue"), login.icon)

        val note = create(ItemDetail.CATEGORY_SECURE_NOTE, "editor-secure-note") {
            type("editor-title", "Recovery codes")
            type("editor-notes", "4821-9930")
        }
        assertTrue(note.isSecureNote)
        assertEquals("4821-9930", note.notes)

        val card = create(ItemDetail.CATEGORY_CREDIT_CARD, "editor-credit-card") {
            type("editor-title", "Visa")
            type("editor-cardholder", "Alice Example")
            type("editor-card-number", "4111111111111111")
            type("editor-card-expiry", "1229")
            type("editor-card-security-code", "123")
            type("editor-address-city", "Springfield")
        }
        assertTrue(card.isCreditCard)
        assertEquals("4111 1111 1111 1111", card.card?.number)
        assertEquals("Visa", card.card?.type)
        assertEquals("12/29", card.card?.expiry)
        assertEquals("123", card.card?.securityCode)
        assertEquals("Springfield", card.card?.city)

        val identity = create(ItemDetail.CATEGORY_IDENTITY, "editor-identity") {
            type("editor-title", "Passport")
            type("editor-identity-first-name", "Alice")
            type("editor-identity-last-name", "Example")
            tap("editor-identity-birth-date-add")
            type("editor-identity-passport-number", "U1234567")
        }
        assertTrue(identity.isIdentity)
        assertEquals("Alice Example", identity.identity?.fullName)
        assertEquals(CalendarDate.formatToday(), identity.identity?.birthDate)
        assertEquals("U1234567", identity.identity?.passportNumber)
        assertEquals("", identity.identity?.driversLicenseNumber)

        val credential = create(ItemDetail.CATEGORY_API_CREDENTIAL, "editor-api-credential") {
            type("editor-title", "Billing key")
            type("editor-api-credential-service", "Stripe")
            type("editor-api-credential-key-id", "key_1")
            type("editor-api-credential-secret", "sk_live_from_the_phone")
            type("editor-api-credential-host", "api.stripe.com")
            tap("editor-api-credential-expiry-add")
        }
        assertTrue(credential.isAPICredential)
        assertEquals("Stripe · api.stripe.com", credential.apiCredential?.summary)
        assertEquals("key_1", credential.apiCredential?.keyID)
        assertEquals("sk_live_from_the_phone", credential.apiCredential?.secret)
        assertEquals(CalendarDate.formatToday(), credential.apiCredential?.expiry)

        val address = create(ItemDetail.CATEGORY_ADDRESS, "editor-address") {
            type("editor-title", "Home")
            type("editor-address-street", "1 Main St")
            type("editor-address-city", "Springfield")
            type("editor-address-country", "US")
        }
        assertTrue(address.isAddress)
        assertEquals("Springfield, US", address.address?.summary)
        assertEquals("1 Main St", address.address?.street)

        val license = create(ItemDetail.CATEGORY_SOFTWARE_LICENSE, "editor-software-license") {
            type("editor-title", "Sketch")
            type("editor-license-product", "Sketch")
            type("editor-license-version", "100")
            type("editor-license-key", "LICENSE-FROM-THE-PHONE")
            type("editor-license-licensed-to", "Alice Example")
        }
        assertTrue(license.isSoftwareLicense)
        assertEquals("Version 100 · Alice Example", license.softwareLicense?.summary)
        assertEquals("LICENSE-FROM-THE-PHONE", license.softwareLicense?.licenseKey)

        val server = create(ItemDetail.CATEGORY_SERVER, "editor-server") {
            type("editor-title", "Build server")
            type("editor-server-host", "build.example.com")
            type("editor-server-port", "2222")
            type("editor-username", "deploy")
            type("editor-password", "server-password-from-the-phone")
            type("editor-website-0", "https://console.example.com")
        }
        assertTrue(server.isServer)
        assertEquals("build.example.com:2222 · deploy", server.server?.summary(server.username))
        assertEquals("server-password-from-the-phone", server.password)
        assertEquals(listOf("https://console.example.com"), server.allURL)

        val database = create(ItemDetail.CATEGORY_DATABASE, "editor-database") {
            assertFalse("a database asks for a one-time password", exists("editor-one-time-password"))
            type("editor-title", "Prod db")
            type("editor-database-host", "db.example.com")
            type("editor-database-port", "5432")
            type("editor-database-name", "app")
            type("editor-username", "app")
            type("editor-password", "database-password-from-the-phone")
        }
        assertTrue(database.isDatabase)
        assertEquals("PostgreSQL · db.example.com:5432/app", database.database?.summary)
        assertEquals("postgresql://app:database-password-from-the-phone@db.example.com:5432/app", database.database?.connectionString)

        assertEquals(
            listOf("API credential", "Address", "Credit card", "Database", "Identity", "Login", "Secure note", "Server", "Software license"),
            backend.listAllItem(vaultID).map { it.category }.map(::describeCategory).sorted(),
        )
    }

    @Test
    fun testEditorEditsDiscardsDeletesAndSavesAPrefilledLoginOnTheServer() {
        signUp("ITEM_EDITOR_EDIT_")
        val saved = backend.saveItem(vaultID, ItemDetail(title = "Bank", username = "alice", password = "bank-password", allURL = listOf("https://bank.example")))
        runBlocking(Dispatchers.Main) { model.refreshAll() }

        open(EditorRequest.Edit(vaultID, saved.id))
        composeRule.waitUntil(10_000) { composeRule.onAllNodes(hasTestTag("editor-title") and hasText("Bank")).fetchSemanticsNodes().isNotEmpty() }
        assertTrue("an existing item can be deleted", exists("editor-delete"))
        composeRule.onNodeWithTag("editor-title").performTextReplacement("Bank of Example")
        saveScreenshot("editor-edit")
        tap("editor-cancel")
        composeRule.waitUntil(5_000) { exists("editor-discard-confirmation") }
        waitUntilStill("editor-discard-confirmation-dismiss")
        saveScreenshot("editor-discard")
        tap("editor-discard-confirmation-dismiss")
        assertTrue("Keep editing stays in the editor", exists("editor-title"))
        assertEquals(saved.id, finishAfterTapping("editor-save"))
        assertEquals("Bank of Example", backend.getItem(vaultID, saved.id).title)
        assertEquals("bank-password", backend.getItem(vaultID, saved.id).password)

        open(EditorRequest.Edit(vaultID, saved.id))
        composeRule.waitUntil(10_000) { exists("editor-delete") }
        tap("editor-delete")
        composeRule.waitUntil(5_000) { exists("editor-delete-confirmation") }
        waitUntilStill("editor-delete-confirmation-confirm")
        saveScreenshot("editor-delete")
        assertEquals(null, finishAfterTapping("editor-delete-confirmation-confirm"))
        assertTrue(backend.listAllItem(vaultID).none { it.id == saved.id })
        assertTrue(model.unlockedData.value.listAllItemReference(vaultID).none { it.item.id == saved.id })

        val allLinked = mutableListOf<ItemDetail>()
        val typedLogin = ItemDetail(title = "example.com", username = "ada", password = "typed-in-a-form", allURL = listOf("https://example.com"))
        open(EditorRequest.New(vaultID, typedLogin, onSaved = { allLinked += it }))
        assertTrue("AutoFill's typed login opens as it was typed", exists("editor-title") && composeRule.onAllNodes(hasTestTag("editor-website-0") and hasText("https://example.com")).fetchSemanticsNodes().isNotEmpty())
        saveScreenshot("editor-autofill-draft")
        assertEquals("an untouched draft closes without asking", null, finishAfterTapping("editor-cancel"))
        open(EditorRequest.New(vaultID, typedLogin, onSaved = { allLinked += it }))
        val typedLoginID = checkNotNull(finishAfterTapping("editor-save"))
        assertEquals(listOf(typedLoginID), allLinked.map { it.id })
        val savedLogin = backend.getItem(vaultID, typedLoginID)
        assertEquals(listOf("example.com", "ada", "typed-in-a-form"), listOf(savedLogin.title, savedLogin.username, savedLogin.password))
        assertEquals(listOf("https://example.com"), savedLogin.allURL)
    }

    private fun describeCategory(category: String) = when (category) {
        ItemDetail.CATEGORY_SECURE_NOTE -> "Secure note"
        ItemDetail.CATEGORY_CREDIT_CARD -> "Credit card"
        ItemDetail.CATEGORY_IDENTITY -> "Identity"
        ItemDetail.CATEGORY_API_CREDENTIAL -> "API credential"
        ItemDetail.CATEGORY_ADDRESS -> "Address"
        ItemDetail.CATEGORY_SOFTWARE_LICENSE -> "Software license"
        ItemDetail.CATEGORY_SERVER -> "Server"
        ItemDetail.CATEGORY_DATABASE -> "Database"
        else -> "Login"
    }
}
