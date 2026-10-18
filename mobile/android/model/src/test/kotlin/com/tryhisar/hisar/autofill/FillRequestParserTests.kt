package com.tryhisar.hisar.autofill

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** The structures below are what the S3 spike measured from WebView, Chrome and native fields on API 34 and 37. */
class FillRequestParserTests {
    private val chrome = "com.android.chrome"
    private val browsers = setOf(chrome)

    private fun input(id: String, vararg attribute: Pair<String, String>, hints: List<String> = emptyList()) = FormNode(
        autofillID = id, isTextField = true, autofillHints = hints, htmlTag = "input", htmlAttributes = mapOf(*attribute),
        webDomain = "127.0.0.1", webScheme = "http",
    )

    private fun page(vararg input: FormNode<String>) = FormNode(
        htmlTag = "form", webDomain = "127.0.0.1", webScheme = "http", children = input.toList(),
    )

    private val chromeToolbar = FormNode(
        children = listOf(FormNode(autofillID = "menu", isTextField = true, idEntry = "menu_button"), FormNode(autofillID = "omnibox", isTextField = true, idEntry = "url_bar_user")),
    )

    @Test
    fun testAutocompleteWordsFromChromeNameTheFields() {
        val form = FillRequestParser.parse(
            listOf(FormNode(children = listOf(chromeToolbar, page(input("u", "type" to "text", hints = listOf("username")), input("p", "type" to "password", hints = listOf("current-password")))))),
            chrome, browsers,
        )
        assertEquals(LoginForm(AutofillSource.Web("http", "127.0.0.1"), "u", "p", null), form)
        assertEquals("http://127.0.0.1", (form?.source as AutofillSource.Web).serviceIdentifier)
        assertEquals(RequestKind.PASSWORD, form.kind)
    }

    @Test
    fun testBareFormFallsBackToTheHtmlTypeAndNameAndIgnoresTheBrowsersOwnViews() {
        val form = FillRequestParser.parse(
            listOf(FormNode(children = listOf(chromeToolbar, page(input("u", "type" to "text", "name" to "login"), input("p", "type" to "password", "name" to "secret"))))),
            chrome, browsers,
        )
        assertEquals(LoginForm(AutofillSource.Web("http", "127.0.0.1"), "u", "p", null), form)
    }

    @Test
    fun testUsernameIsTheTextFieldRightBeforeThePassword() {
        val form = FillRequestParser.parse(
            listOf(page(input("search", "type" to "search"), input("u", "type" to "text", "name" to "account"), input("p", "type" to "password"))),
            chrome, browsers,
        )
        assertEquals("u", form?.username)
        assertEquals("p", form?.password)
    }

    @Test
    fun testWebViewWithoutItsOwnAddressFindsEveryInputOfThePage() {
        val inputs = listOf(
            input("u", "type" to "text", "name" to "username", hints = listOf("username")).copy(webDomain = "accounts.example.com", webScheme = null),
            input("p", "type" to "password", "name" to "password", hints = listOf("current-password")).copy(webDomain = "accounts.example.com", webScheme = null),
        )
        val webView = FormNode(htmlTag = "form", webDomain = "", children = inputs)
        val form = FillRequestParser.parse(listOf(FormNode(children = listOf(webView))), chrome, browsers)
        assertEquals(LoginForm(AutofillSource.Web("https", "accounts.example.com"), "u", "p", null), form)
    }

    @Test
    fun testNativeFieldsUseTheirHintsAndInputTypes() {
        val form = FillRequestParser.parse(
            listOf(
                FormNode(
                    children = listOf(
                        FormNode(autofillID = "title", isTextField = false),
                        FormNode(autofillID = "email", isTextField = true, autofillHints = listOf("username"), inputType = 0x21),
                        FormNode(autofillID = "password", isTextField = true, autofillHints = listOf("passwordAuto"), inputType = 0x81),
                    ),
                ),
            ),
            "com.example.bank", browsers,
        )
        assertEquals(LoginForm(AutofillSource.App("com.example.bank"), "email", "password", null), form)
    }

    @Test
    fun testInputTypesAndViewIdsNameUndeclaredNativeFields() {
        val form = FillRequestParser.parse(
            listOf(
                FormNode(
                    children = listOf(
                        FormNode(autofillID = "a", isTextField = true, inputType = 0x21),
                        FormNode(autofillID = "b", isTextField = true, inputType = 0x12),
                        FormNode(autofillID = "c", isTextField = true, idEntry = "otp_input"),
                    ),
                ),
            ),
            "com.example.bank", browsers,
        )
        assertEquals(LoginForm(AutofillSource.App("com.example.bank"), "a", "b", "c"), form)
    }

    @Test
    fun testAnotherAppsWebPageIsTheApp() {
        val form = FillRequestParser.parse(listOf(page(input("u", hints = listOf("username")), input("p", hints = listOf("password")))), "com.example.phishing", browsers)
        assertEquals(AutofillSource.App("com.example.phishing"), form?.source, "only a browser's page address is trusted")
    }

    @Test
    fun testBrowserWithoutAPageAddressCanOnlySearch() {
        val form = FillRequestParser.parse(listOf(FormNode(children = listOf(FormNode(autofillID = "p", isTextField = true, inputType = 0x81)))), chrome, browsers)
        assertEquals(AutofillSource.UnknownPage(chrome), form?.source)
    }

    @Test
    fun testAPackageTrustedOnlyForPagesIsAnAppWithoutAPage() {
        val nativeForm = listOf(FormNode(children = listOf(FormNode(autofillID = "p", isTextField = true, inputType = 0x81))))
        assertEquals(AutofillSource.App("com.example.test"), FillRequestParser.parse(nativeForm, "com.example.test", browsers + "com.example.test")?.source)
    }

    @Test
    fun testOneTimeCodeFieldAloneAsksForACode() {
        val form = FillRequestParser.parse(listOf(page(input("code", "type" to "text", hints = listOf("one-time-code")))), chrome, browsers)
        assertEquals(LoginForm(AutofillSource.Web("http", "127.0.0.1"), null, null, "code"), form)
        assertEquals(RequestKind.ONE_TIME_CODE, form?.kind)
    }

    @Test
    fun testSchemeDefaultsToHttpsAndTheDomainIsLowercase() {
        val form = FillRequestParser.parse(
            listOf(FormNode(webDomain = "Accounts.Example.COM", children = listOf(FormNode(autofillID = "p", isTextField = true, htmlTag = "input", htmlAttributes = mapOf("type" to "password"))))),
            chrome, browsers,
        )
        assertEquals(AutofillSource.Web("https", "accounts.example.com"), form?.source)
    }

    @Test
    fun testFormWithoutSignInFieldsIsNotFilled() {
        assertNull(FillRequestParser.parse(listOf(page(input("q", "type" to "search", "name" to "query"))), chrome, browsers))
        assertNull(FillRequestParser.parse(listOf(FormNode(children = listOf(FormNode(autofillID = "note", isTextField = true)))), "com.example.notes", browsers))
    }
}
