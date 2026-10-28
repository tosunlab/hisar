package com.tryhisar.hisar.passkey

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.Density
import com.tryhisar.hisar.autofill.AutoFillModel
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.PasskeyRegistrationRequest
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.runBlocking
import kotlinx.io.bytestring.ByteString
import org.junit.After
import org.junit.Rule
import org.junit.Test

class PasskeyLayoutTests {
    @get:Rule
    val composeRule = createComposeRule()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)

    @After
    fun close() = scope.cancel()

    @Test
    fun testManyDestinationsKeepTheSelectionAndSaveVisible() {
        val backend = FakeBackend().apply {
            seedSignedIn()
            repeat(30) { index ->
                allItemByVaultID.getValue("personal").add(ItemDetail(id = "choice-$index", title = "Account $index", username = "user-$index", allURL = listOf("github.com")))
            }
            allItemByVaultID.getValue("family").add(ItemDetail(id = "shared", title = "ZZ Shared", username = "shared-user", allURL = listOf("github.com")))
        }
        val model = AutoFillModel(
            scope, Dispatchers.IO, { backend }, MutableSharedFlow(), { _, _ -> emptyList() },
            completeRequest = {}, completeOneTimeCodeRequest = {}, copyOneTimeCode = {}, cancelRequest = {},
            passkeyCompletion = AutoFillModel.PasskeyCompletion({}, {}, {}, {}),
        )
        runBlocking(Dispatchers.Main) {
            model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest("github.com", "new-user", ByteString(5), ByteString(*ByteArray(32))))
        }
        composeRule.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale = 1.5f)) {
                HisarTheme { PasskeyRegistrationView(model) }
            }
        }
        composeRule.onNodeWithTag("autofill-passkey-new-login").assertIsDisplayed()
        composeRule.onNodeWithTag("autofill-save-passkey").assertIsDisplayed()
        composeRule.onNodeWithTag("autofill-passkey-destination").assertTextContains("New login in Personal")
        composeRule.onNodeWithTag("autofill-passkey-login-ZZ Shared").performScrollTo().performClick()
        composeRule.onNodeWithTag("autofill-save-passkey").assertIsDisplayed()
        composeRule.onNodeWithTag("autofill-passkey-destination").assertTextContains("Family", substring = true)
        composeRule.onNodeWithText("Shared. Other members can sign in with this passkey.").assertIsDisplayed()
    }
}
