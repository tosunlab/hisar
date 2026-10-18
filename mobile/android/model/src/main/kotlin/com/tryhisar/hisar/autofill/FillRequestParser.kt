package com.tryhisar.hisar.autofill

/**
 * One view of a fill request's AssistStructure, as the parser reads it. [Id] is the platform's AutofillId, so the parser
 * runs on the JVM. Browsers and WebViews report a page's inputs at or below a node that carries [webDomain]: the form's
 * node, and on WebView 145 (API 37) each input too. For a page loaded with loadDataWithBaseURL the form's [webDomain] is
 * empty, so WebView 145 names the page only on its inputs and WebView 91 and 113 (API 29 and 34) name no page at all.
 */
data class FormNode<Id>(
    val autofillID: Id? = null,
    val isTextField: Boolean = false,
    val autofillHints: List<String> = emptyList(),
    val htmlTag: String? = null,
    val htmlAttributes: Map<String, String> = emptyMap(),
    val inputType: Int = 0,
    val idEntry: String? = null,
    val hint: String? = null,
    val webDomain: String? = null,
    val webScheme: String? = null,
    val children: List<FormNode<Id>> = emptyList(),
)

/** Where a fill request comes from: a page a browser shows, or an app. */
sealed interface AutofillSource {
    data class Web(val scheme: String, val domain: String) : AutofillSource {
        val serviceIdentifier: String get() = "$scheme://$domain"
    }

    /** A browser that named no page, which only a search can fill. */
    data class UnknownPage(val browserPackage: String) : AutofillSource

    data class App(val packageName: String) : AutofillSource
}

enum class FieldKind { USERNAME, PASSWORD, ONE_TIME_CODE }

/** The fields of a sign-in form Hisar can fill, and where the form is. */
data class LoginForm<Id>(val source: AutofillSource, val username: Id?, val password: Id?, val oneTimeCode: Id?) {
    /** A form with a username or password asks for a login; one with only a code field asks for a one-time code. */
    val kind: RequestKind get() = if (username == null && password == null) RequestKind.ONE_TIME_CODE else RequestKind.PASSWORD

    val allFilledID: List<Id> get() = listOfNotNull(username, password, oneTimeCode)
}

/**
 * Finds the username, password and one-time code fields of a fill request. A field's kind comes from, first match wins:
 * its autofill hints (which carry HTML `autocomplete` words from browsers), its HTML `type` and name, its input type,
 * then words in its view ID or hint text. A page's address counts only from a browser in [allBrowserPackage]: any app
 * can claim a web domain in its own views, so another app's page goes through the app's own checks instead.
 */
object FillRequestParser {
    fun <Id> parse(allRoot: List<FormNode<Id>>, packageName: String, allBrowserPackage: Set<String>): LoginForm<Id>? {
        val allWebField = allRoot.flatMap { listAllWebField(it, page = null) }
        val webPage = allWebField.firstOrNull()?.second
        val allField = allWebField.map { it.first }.ifEmpty { allRoot.flatMap(::listAllField) }
        val classified = allField.mapNotNull { node -> classify(node)?.let { node to it } }
        val password = classified.firstOrNull { it.second == FieldKind.PASSWORD }?.first
        val username = classified.firstOrNull { it.second == FieldKind.USERNAME }?.first ?: password?.let { findTextFieldBefore(allField, it) }
        val oneTimeCode = classified.firstOrNull { it.second == FieldKind.ONE_TIME_CODE }?.first
        if (username == null && password == null && oneTimeCode == null) return null
        val source = when {
            webPage != null && packageName in allBrowserPackage ->
                AutofillSource.Web(webPage.webScheme?.lowercase()?.ifEmpty { null } ?: "https", webPage.webDomain!!.lowercase())
            packageName in TrustedBrowsers.allPackageName -> AutofillSource.UnknownPage(packageName)
            else -> AutofillSource.App(packageName)
        }
        return LoginForm(source, username?.autofillID, password?.autofillID, oneTimeCode?.autofillID)
    }

    /** The text fields that are or lie below a node with a page address, each with the nearest such node. */
    private fun <Id> listAllWebField(node: FormNode<Id>, page: FormNode<Id>?): List<Pair<FormNode<Id>, FormNode<Id>>> {
        val nodePage = if (node.webDomain.isNullOrEmpty()) page else node
        val field = if (nodePage != null && node.isTextField && node.autofillID != null) listOf(node to nodePage) else emptyList()
        return field + node.children.flatMap { listAllWebField(it, nodePage) }
    }

    private fun <Id> listAllField(node: FormNode<Id>): List<FormNode<Id>> =
        (if (node.isTextField && node.autofillID != null) listOf(node) else emptyList()) + node.children.flatMap(::listAllField)

    // A login form without a declared username field usually has it as the text field right before the password.
    private fun <Id> findTextFieldBefore(allField: List<FormNode<Id>>, password: FormNode<Id>): FormNode<Id>? {
        val index = allField.indexOf(password)
        return allField.take(index).lastOrNull { classify(it) == null && !isHidden(it) }
    }

    private fun isHidden(node: FormNode<*>): Boolean = node.htmlAttributes["type"]?.lowercase() in setOf("hidden", "submit", "button", "checkbox", "radio")

    fun classify(node: FormNode<*>): FieldKind? =
        classifyHints(node.autofillHints) ?: classifyHtml(node) ?: classifyInputType(node.inputType) ?: classifyWords(listOfNotNull(node.idEntry, node.hint))

    private fun classifyHints(allHint: List<String>): FieldKind? {
        val allWord = allHint.flatMap { it.lowercase().split(' ') }
        return when {
            allWord.any { it == "one-time-code" || it == "smsotpcode" || it == "2faappotpcode" || it == "onetimecode" } -> FieldKind.ONE_TIME_CODE
            allWord.any { "password" in it } -> FieldKind.PASSWORD
            allWord.any { it == "username" || it == "email" || it == "emailaddress" || it == "newusername" } -> FieldKind.USERNAME
            else -> null
        }
    }

    private fun classifyHtml(node: FormNode<*>): FieldKind? {
        if (node.htmlTag?.lowercase() != "input") return null
        val attributes = node.htmlAttributes.mapKeys { it.key.lowercase() }
        val type = attributes["type"]?.lowercase().orEmpty()
        classifyHints(listOfNotNull(attributes["autocomplete"]))?.let { return it }
        return when {
            type == "password" -> FieldKind.PASSWORD
            type == "email" -> FieldKind.USERNAME
            type in setOf("", "text", "tel", "number") -> classifyWords(listOfNotNull(attributes["name"], attributes["id"]))
            else -> null
        }
    }

    private fun classifyInputType(inputType: Int): FieldKind? {
        val inputClass = inputType and TYPE_MASK_CLASS
        val variation = inputType and TYPE_MASK_VARIATION
        return when {
            inputClass == TYPE_CLASS_TEXT && variation in setOf(VARIATION_PASSWORD, VARIATION_VISIBLE_PASSWORD, VARIATION_WEB_PASSWORD) -> FieldKind.PASSWORD
            inputClass == TYPE_CLASS_NUMBER && variation == NUMBER_VARIATION_PASSWORD -> FieldKind.PASSWORD
            inputClass == TYPE_CLASS_TEXT && variation in setOf(VARIATION_EMAIL_ADDRESS, VARIATION_WEB_EMAIL_ADDRESS) -> FieldKind.USERNAME
            else -> null
        }
    }

    private fun classifyWords(allText: List<String>): FieldKind? {
        val text = allText.joinToString(" ").lowercase()
        return when {
            text.isEmpty() -> null
            listOf("otp", "one-time", "one_time", "onetime", "2fa", "totp").any { it in text } -> FieldKind.ONE_TIME_CODE
            listOf("pass", "pwd").any { it in text } -> FieldKind.PASSWORD
            listOf("user", "email", "login").any { it in text } -> FieldKind.USERNAME
            else -> null
        }
    }

    // android.text.InputType, which the JVM module cannot import.
    private const val TYPE_MASK_CLASS = 0x0000000f
    private const val TYPE_MASK_VARIATION = 0x00000ff0
    private const val TYPE_CLASS_TEXT = 0x00000001
    private const val TYPE_CLASS_NUMBER = 0x00000002
    private const val VARIATION_EMAIL_ADDRESS = 0x00000020
    private const val VARIATION_PASSWORD = 0x00000080
    private const val VARIATION_VISIBLE_PASSWORD = 0x00000090
    private const val VARIATION_WEB_EMAIL_ADDRESS = 0x000000d0
    private const val VARIATION_WEB_PASSWORD = 0x000000e0
    private const val NUMBER_VARIATION_PASSWORD = 0x00000010
}

/** The browsers whose reported page address Hisar trusts. The service's compatibility list is part of it. */
object TrustedBrowsers {
    val allPackageName: Set<String> = setOf(
        "com.android.chrome", "com.chrome.beta", "com.chrome.dev", "com.chrome.canary", "com.google.android.apps.chrome", "org.chromium.chrome",
        "org.mozilla.firefox", "org.mozilla.firefox_beta", "org.mozilla.fenix", "org.mozilla.focus", "org.torproject.torbrowser",
        "com.microsoft.emmx", "com.microsoft.emmx.beta", "com.microsoft.emmx.dev", "com.microsoft.emmx.canary",
        "com.brave.browser", "com.brave.browser_beta", "com.brave.browser_nightly",
        "com.opera.browser", "com.opera.browser.beta", "com.opera.gx", "com.opera.mini.native", "com.opera.mini.native.beta",
        "com.sec.android.app.sbrowser", "com.sec.android.app.sbrowser.beta",
        "com.vivaldi.browser", "com.vivaldi.browser.snapshot", "com.duckduckgo.mobile.android", "com.ecosia.android", "com.kiwibrowser.browser",
    )
}
