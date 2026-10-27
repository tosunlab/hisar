package com.tryhisar.hisar.ui

import android.view.View
import android.view.ViewGroup
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputConnection
import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.ViewRootForTest
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextReplacement
import androidx.core.view.children
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.EditorRequest
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemField
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.ui.components.exposeTestTags
import com.tryhisar.hisar.ui.item.ItemEditorScreen
import com.tryhisar.hisar.ui.lock.LockScreen
import com.tryhisar.hisar.ui.settings.ChangeMasterPasswordScreen
import com.tryhisar.hisar.ui.signin.SignInScreen
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Secret fields across the sign-in, lock, editor and Settings screens: what the keyboard and an autofill service are
 * told about each. A secret asks the keyboard not to learn it, and autofill cannot fill it and gets its value only as
 * sensitive data; ordinary fields, such as an email, can be learned and filled.
 */
@RunWith(AndroidJUnit4::class)
class EditorFieldHygieneTests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private val backend = FakeBackend()
    private val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())
    private var typedCount = 0

    @After
    fun stopModel() {
        scope.cancel()
    }

    @Test
    fun testSignInAndCreateAccountSecretsStayOutOfAutofillAndKeyboardLearning() {
        show { SignInScreen(model) }
        assertOrdinary("sign-in-email")
        assertSecret("sign-in-secret-key")
        assertSecret("sign-in-password")
        composeRule.onNodeWithText("Create account").performClick()
        assertOrdinary("sign-up-email")
        assertSecret("sign-up-password")
        assertSecret("sign-up-repeated-password")
    }

    @Test
    fun testLockScreenPasswordStaysOutOfAutofillAndKeyboardLearning() {
        backend.seedSignedIn(VaultStatus.STATE_LOCKED)
        show { LockScreen(model) }
        assertSecret("unlock-password")
    }

    @Test
    fun testEditorSecretsStayOutOfAutofillAndKeyboardLearning() {
        backend.seedSignedIn()
        var request by mutableStateOf<EditorRequest>(EditorRequest.New("personal", ItemDetail(allField = listOf(ItemField("Door code", "", concealed = true)))))
        show { key(request) { ItemEditorScreen(model, request) {} } }
        assertOrdinary("editor-username")
        assertSecret("editor-password")
        assertSecret("editor-one-time-password")
        assertSecret("editor-field-0-value")
        assertSecret("editor-notes")

        val allSecretTagByCategory = mapOf(
            ItemDetail.CATEGORY_CREDIT_CARD to listOf("editor-card-number", "editor-card-security-code", "editor-card-pin"),
            ItemDetail.CATEGORY_IDENTITY to listOf("editor-identity-passport-number", "editor-identity-drivers-license-number", "editor-identity-national-id-number"),
            ItemDetail.CATEGORY_API_CREDENTIAL to listOf("editor-api-credential-secret"),
            ItemDetail.CATEGORY_SOFTWARE_LICENSE to listOf("editor-license-key"),
        )
        for ((category, allSecretTag) in allSecretTagByCategory) {
            composeRule.runOnUiThread { request = EditorRequest.New("personal", ItemDetail(category = category)) }
            composeRule.waitForTag(allSecretTag.first())
            allSecretTag.forEach(::assertSecret)
        }
        composeRule.runOnUiThread { request = EditorRequest.New("personal", ItemDetail(category = ItemDetail.CATEGORY_SECURE_NOTE)) }
        composeRule.waitForTag("editor-notes")
        assertSecret("editor-notes")
    }

    @Test
    fun testSavedSetupKeyRequiresRevealAndResetsOnReopening() {
        backend.seedSignedIn()
        val setupKey = "otpauth://totp/?secret=JBSWY3DPEHPK3PXP"
        var request by mutableStateOf<EditorRequest>(
            EditorRequest.New("personal", ItemDetail(password = "main-secret", oneTimePassword = setupKey)),
        )
        show { key(request) { ItemEditorScreen(model, request) {} } }
        for (category in listOf(ItemDetail.CATEGORY_LOGIN, ItemDetail.CATEGORY_SERVER, ItemDetail.CATEGORY_LOGIN)) {
            composeRule.runOnUiThread {
                request = EditorRequest.New("personal", ItemDetail(category = category, title = category, password = "main-secret", oneTimePassword = setupKey))
            }
            val field = composeRule.onNodeWithTag("editor-one-time-password")
            field.performScrollTo().assertTextEquals("•".repeat(setupKey.length))
            composeRule.onNodeWithContentDescription("Reveal setup key").performClick()
            field.assertTextEquals(setupKey)
            composeRule.onNodeWithTag("editor-password").assertTextEquals("•".repeat("main-secret".length))
            composeRule.onNodeWithContentDescription("Hide setup key").performClick()
            field.assertTextEquals("•".repeat(setupKey.length))
            composeRule.onNodeWithContentDescription("Reveal setup key").performClick()
        }
    }

    @Test
    fun testSettingsPasswordsStayOutOfAutofillAndKeyboardLearning() {
        backend.seedSignedIn()
        show { ChangeMasterPasswordScreen(model) {} }
        assertSecret("change-password-current")
        assertSecret("change-password-new")
        assertSecret("change-password-repeated")
    }

    private fun show(screen: @Composable () -> Unit) {
        runBlocking(Dispatchers.Main) {
            model.start { backend }
            if (model.status.value.isUnlocked) model.refreshAll()
        }
        composeRule.setContent {
            HisarTheme {
                Box(Modifier.fillMaxSize().exposeTestTags()) { screen() }
            }
        }
    }

    private fun assertSecret(tag: String) {
        val text = focusAndType(tag)
        val editorInfo = readFocusedEditorInfo()
        assertTrue("$tag: the keyboard is asked not to learn it", editorInfo.imeOptions and EditorInfo.IME_FLAG_NO_PERSONALIZED_LEARNING != 0)
        val node = readAutofillNode(tag, text)
        assertEquals("$tag: autofill cannot fill it", View.AUTOFILL_TYPE_NONE, node.autofillType)
        assertTrue("$tag: autofill treats its value as sensitive", node.isSensitive)
    }

    private fun assertOrdinary(tag: String) {
        val text = focusAndType(tag)
        val editorInfo = readFocusedEditorInfo()
        assertFalse("$tag: the keyboard may learn it", editorInfo.imeOptions and EditorInfo.IME_FLAG_NO_PERSONALIZED_LEARNING != 0)
        val node = readAutofillNode(tag, text)
        assertEquals("$tag: autofill can fill it", View.AUTOFILL_TYPE_TEXT, node.autofillType)
    }

    /** Taps the field with [tag], as a finger would, and replaces its text with a value no other field holds. */
    private fun focusAndType(tag: String): String {
        typedCount += 1
        val text = "1234$typedCount"
        composeRule.waitForTag(tag)
        composeRule.onNodeWithTag(tag).performScrollTo().performClick()
        composeRule.onNodeWithTag(tag).performTextReplacement(text)
        composeRule.waitForIdle()
        return text
    }

    /** What the focused field tells the keyboard when the keyboard connects to it. */
    private fun readFocusedEditorInfo(): EditorInfo {
        val editorInfo = EditorInfo()
        var connection: InputConnection? = null
        composeRule.runOnUiThread { connection = checkNotNull(composeRule.activity.currentFocus) { "no view has focus" }.onCreateInputConnection(editorInfo) }
        assertNotNull("the focused field takes text from the keyboard", connection)
        return editorInfo
    }

    /** The autofill node that holds [text], from the structure the screen gives an autofill service. */
    private fun readAutofillNode(tag: String, text: String): RecordedViewStructure {
        val structure = RecordedViewStructure()
        composeRule.runOnUiThread { findComposeView(composeRule.activity.window.decorView).onProvideAutofillVirtualStructure(structure, 0) }
        return checkNotNull(structure.findNodeWithValue(text)) {
            "$tag: no autofill node holds $text; the values are ${structure.listAllNode().mapNotNull { it.autofillValue?.takeIf { value -> value.isText }?.textValue }}"
        }
    }

    private fun findComposeView(view: View): View =
        checkNotNull(findComposeRoot(view)) { "the activity shows no Compose content" }

    private fun findComposeRoot(view: View): View? =
        view.takeIf { it is ViewRootForTest } ?: (view as? ViewGroup)?.children?.firstNotNullOfOrNull(::findComposeRoot)
}
