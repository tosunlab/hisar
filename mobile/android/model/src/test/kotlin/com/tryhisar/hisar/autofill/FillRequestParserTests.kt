package com.tryhisar.hisar.autofill

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** The structures below are what the S3 spike measured from WebView, Chrome and native fields on API 34 and 37. */
class FillRequestParserTests {
    private val chrome = AndroidApp("com.android.chrome", "C4:".repeat(31) + "C4")
    private val testApp = AndroidApp("com.example.test", "7E:".repeat(31) + "7E")
    private val browsers = PrivilegedBrowsers(setOf(chrome), allWebViewApp = setOf(testApp))

    private fun parse(allRoot: List<FormNode<String>>, caller: AndroidApp = chrome) =
        FillRequestParser.parse(allRoot, caller.packageName, caller.certificateSHA256, browsers)

    private fun app(packageName: String) = AndroidApp(packageName, "AB:".repeat(31) + "AB")

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
        val form = parse(
            listOf(FormNode(children = listOf(chromeToolbar, page(input("u", "type" to "text", hints = listOf("username")), input("p", "type" to "password", hints = listOf("current-password")))))),
        )
        assertEquals(LoginForm(AutofillSource.Web("http", "127.0.0.1"), "u", "p", null), form)
        assertEquals("http://127.0.0.1", (form?.source as AutofillSource.Web).serviceIdentifier)
        assertEquals(RequestKind.PASSWORD, form.kind)
    }

    @Test
    fun testBareFormFallsBackToTheHtmlTypeAndNameAndIgnoresTheBrowsersOwnViews() {
        val form = parse(
            listOf(FormNode(children = listOf(chromeToolbar, page(input("u", "type" to "text", "name" to "login"), input("p", "type" to "password", "name" to "secret"))))),
        )
        assertEquals(LoginForm(AutofillSource.Web("http", "127.0.0.1"), "u", "p", null), form)
    }

    @Test
    fun testUsernameIsTheTextFieldRightBeforeThePassword() {
        val form = parse(
            listOf(page(input("search", "type" to "search"), input("u", "type" to "text", "name" to "account"), input("p", "type" to "password"))),
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
        val form = parse(listOf(FormNode(children = listOf(webView))))
        assertEquals(LoginForm(AutofillSource.Web("https", "accounts.example.com"), "u", "p", null), form)
    }

    // The structure WebView 113 on API 34 delivers for https://accounts.example.com/login, read in the service.
    private fun webView113(formDomain: String, formScheme: String?, focusedID: String? = null) = FormNode(
        autofillID = "webview",
        children = listOf(
            FormNode(
                autofillID = "form", htmlTag = "form", htmlAttributes = mapOf("name" to ""), webDomain = formDomain, webScheme = formScheme,
                children = listOf(
                    FormNode(
                        autofillID = "u", isTextField = true, isFocused = focusedID == "u", autofillHints = listOf("username"), htmlTag = "input", hint = "Email",
                        htmlAttributes = mapOf("name" to "username", "type" to "text", "label" to "Email", "id" to "username", "computed-autofill-hints" to "HTML_TYPE_EMAIL"),
                    ),
                    FormNode(
                        autofillID = "p", isTextField = true, isFocused = focusedID == "p", autofillHints = listOf("current-password"), htmlTag = "input", hint = "Password",
                        htmlAttributes = mapOf("name" to "password", "type" to "password", "label" to "Password", "id" to "password", "computed-autofill-hints" to "UNKNOWN_TYPE"),
                    ),
                ),
            ),
        ),
    )

    // The form WebView 145 and Chrome 145 on API 37 deliver, with the page address on the form and on each input. Chrome
    // fills forms across frames, so one form can hold an input whose address is another origin's frame.
    private fun form145(formDomain: String, usernameDomain: String = formDomain, passwordDomain: String = formDomain, focusedID: String? = null) = FormNode(
        autofillID = "form", htmlTag = "form", htmlAttributes = mapOf("name" to ""), webDomain = formDomain, webScheme = "https",
        children = listOf(
            FormNode(
                autofillID = "u", isTextField = true, isFocused = focusedID == "u", autofillHints = listOf("username"), htmlTag = "input", hint = "Email",
                htmlAttributes = mapOf("name" to "username", "type" to "text", "label" to "Email", "id" to "username", "computed-autofill-hints" to "USERNAME"),
                webDomain = usernameDomain, webScheme = "https",
            ),
            FormNode(
                autofillID = "p", isTextField = true, isFocused = focusedID == "p", autofillHints = listOf("current-password"), htmlTag = "input", hint = "Password",
                htmlAttributes = mapOf("name" to "password", "type" to "password", "label" to "Password", "id" to "password", "computed-autofill-hints" to "PASSWORD"),
                webDomain = passwordDomain, webScheme = "https",
            ),
        ),
    )

    @Test
    fun testWebViewOnAPI34PutsThePageAddressOnTheFormOnly() {
        val form = parse(listOf(webView113("accounts.example.com", "https")), testApp)
        assertEquals(LoginForm(AutofillSource.Web("https", "accounts.example.com"), "u", "p", null), form)
    }

    @Test
    fun testWebViewOnAPI34PageLoadedFromDataHasNoAddressSoItIsTheApp() {
        val form = parse(listOf(webView113("", null)), testApp)
        assertEquals(LoginForm(AutofillSource.App(testApp.packageName), "u", "p", null), form, "loadDataWithBaseURL names no page on WebView 113")
    }

    @Test
    fun testOnePageFillsEveryFieldWhicheverIsFocused() {
        val accounts = AutofillSource.Web("https", "accounts.example.com")
        for (focusedID in listOf("u", "p")) {
            val onAPI37 = parse(listOf(FormNode(autofillID = "webview", children = listOf(form145("accounts.example.com", focusedID = focusedID)))), testApp)
            assertEquals(LoginForm(accounts, "u", "p", null), onAPI37, "WebView 145, $focusedID focused")
            val onAPI34 = parse(listOf(webView113("accounts.example.com", "https", focusedID)), testApp)
            assertEquals(LoginForm(accounts, "u", "p", null), onAPI34, "WebView 113, $focusedID focused")
        }
    }

    @Test
    fun testAnotherPagesFieldGetsNothingFromTheFramedBank() {
        val bankFrameInAttackerPage = { focusedID: String ->
            listOf(FormNode(children = listOf(chromeToolbar, form145("attacker.example", usernameDomain = "bank.example.com", focusedID = focusedID))))
        }
        val bankFocused = parse(bankFrameInAttackerPage("u"))
        assertEquals(LoginForm(AutofillSource.Web("https", "bank.example.com"), "u", null, null), bankFocused, "only the bank's own field is filled with the bank's logins")
        val attackerFocused = parse(bankFrameInAttackerPage("p"))
        assertEquals(LoginForm(AutofillSource.Web("https", "attacker.example"), null, "p", null), attackerFocused, "the attacker's field is filled only as the attacker's page")
    }

    @Test
    fun testFormSpanningSeveralPagesWithoutFocusIsNotFilled() {
        val mixed = listOf(FormNode(children = listOf(chromeToolbar, form145("attacker.example", usernameDomain = "bank.example.com"))))
        assertNull(parse(mixed))
        val focusedOutsideThePages = listOf(
            FormNode(children = listOf(chromeToolbar.copy(children = chromeToolbar.children.map { it.copy(isFocused = it.autofillID == "omnibox") }), mixed.single().children.last())),
        )
        assertNull(parse(focusedOutsideThePages), "a focused browser view names no page")
        val sameHostOtherScheme = listOf(page(input("u", hints = listOf("username")), input("p", hints = listOf("password")).copy(webScheme = "https")))
        assertNull(parse(sameHostOtherScheme), "http and https are different pages")
    }

    @Test
    fun testNativeFieldsUseTheirHintsAndInputTypes() {
        val form = parse(
            listOf(
                FormNode(
                    children = listOf(
                        FormNode(autofillID = "title", isTextField = false),
                        FormNode(autofillID = "email", isTextField = true, autofillHints = listOf("username"), inputType = 0x21),
                        FormNode(autofillID = "password", isTextField = true, autofillHints = listOf("passwordAuto"), inputType = 0x81),
                    ),
                ),
            ),
            app("com.example.bank"),
        )
        assertEquals(LoginForm(AutofillSource.App("com.example.bank"), "email", "password", null), form)
    }

    @Test
    fun testInputTypesAndViewIdsNameUndeclaredNativeFields() {
        val form = parse(
            listOf(
                FormNode(
                    children = listOf(
                        FormNode(autofillID = "a", isTextField = true, inputType = 0x21),
                        FormNode(autofillID = "b", isTextField = true, inputType = 0x12),
                        FormNode(autofillID = "c", isTextField = true, idEntry = "otp_input"),
                    ),
                ),
            ),
            app("com.example.bank"),
        )
        assertEquals(LoginForm(AutofillSource.App("com.example.bank"), "a", "b", "c"), form)
    }

    @Test
    fun testAnotherAppsWebPageIsTheApp() {
        val form = parse(listOf(page(input("u", hints = listOf("username")), input("p", hints = listOf("password")))), app("com.example.phishing"))
        assertEquals(AutofillSource.App("com.example.phishing"), form?.source, "only a browser's page address is trusted")
    }

    @Test
    fun testBrowserWithoutAPageAddressCanOnlySearch() {
        val form = parse(listOf(FormNode(children = listOf(FormNode(autofillID = "p", isTextField = true, inputType = 0x81)))))
        assertEquals(AutofillSource.UnknownPage(chrome.packageName), form?.source)
    }

    @Test
    fun testAPackageTrustedOnlyForPagesIsAnAppWithoutAPage() {
        val nativeForm = listOf(FormNode(children = listOf(FormNode(autofillID = "p", isTextField = true, inputType = 0x81))))
        assertEquals(AutofillSource.App(testApp.packageName), parse(nativeForm, testApp)?.source)
    }

    @Test
    fun testABrowsersPackageNameWithAnotherCertificateIsThatApp() {
        val signInPage = listOf(page(input("u", hints = listOf("username")), input("p", hints = listOf("password"))))
        assertEquals(AutofillSource.Web("http", "127.0.0.1"), parse(signInPage)?.source)
        val repackaged = chrome.copy(certificateSHA256 = "BA:".repeat(31) + "BA")
        assertEquals(AutofillSource.App(chrome.packageName), parse(signInPage, repackaged)?.source, "the page address of an app named like Chrome is not trusted")
        val nativeForm = listOf(FormNode(children = listOf(FormNode(autofillID = "p", isTextField = true, inputType = 0x81))))
        assertEquals(AutofillSource.App(chrome.packageName), parse(nativeForm, repackaged)?.source, "it is an app, so its logins can be linked to it")
        assertEquals(AutofillSource.App(chrome.packageName), FillRequestParser.parse(signInPage, chrome.packageName, null, browsers)?.source, "an app with several signers is never a browser")
        assertEquals(AutofillSource.App(testApp.packageName), parse(signInPage, testApp.copy(certificateSHA256 = repackaged.certificateSHA256))?.source)
    }

    @Test
    fun testOneTimeCodeFieldAloneAsksForACode() {
        val form = parse(listOf(page(input("code", "type" to "text", hints = listOf("one-time-code")))))
        assertEquals(LoginForm(AutofillSource.Web("http", "127.0.0.1"), null, null, "code"), form)
        assertEquals(RequestKind.ONE_TIME_CODE, form?.kind)
    }

    @Test
    fun testSchemeDefaultsToHttpsAndTheDomainIsLowercase() {
        val form = parse(
            listOf(FormNode(webDomain = "Accounts.Example.COM", children = listOf(FormNode(autofillID = "p", isTextField = true, htmlTag = "input", htmlAttributes = mapOf("type" to "password"))))),
        )
        assertEquals(AutofillSource.Web("https", "accounts.example.com"), form?.source)
    }

    @Test
    fun testFormWithoutSignInFieldsIsNotFilled() {
        assertNull(parse(listOf(page(input("q", "type" to "search", "name" to "query")))))
        assertNull(parse(listOf(FormNode(children = listOf(FormNode(autofillID = "note", isTextField = true)))), app("com.example.notes")))
    }
}
