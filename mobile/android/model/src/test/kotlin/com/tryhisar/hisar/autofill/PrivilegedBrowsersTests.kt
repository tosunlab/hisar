package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.model.FakeBackend
import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.fail

/** The privileged-browser list Hisar ships, as AutoFill reads it to decide whose page addresses it trusts. */
class PrivilegedBrowsersTests {
    private val browsers = PrivilegedBrowsers.parse(
        File(
            System.getProperty("hisar.repositoryRoot") ?: fail("Run this test through Gradle, which sets hisar.repositoryRoot"),
            "mobile/android/app/src/main/assets/privileged_browsers.json",
        ).readText(),
    )
    private val chrome = AndroidApp("com.android.chrome", "F0:FD:6C:5B:41:0F:25:CB:25:C3:B5:33:46:C8:97:2F:AE:30:F8:EE:74:11:DF:91:04:80:AD:6B:2D:60:DB:83")
    private val repackagedChrome = chrome.copy(certificateSHA256 = "BA:".repeat(31) + "BA")
    private val backend = FakeBackend().apply { seedSignedIn() }
    private val allAskedApp = mutableListOf<AndroidApp>()
    private val matcher = LoginMatcher(backend) { app, _ ->
        allAskedApp += app
        emptyList()
    }

    private val bankPage = listOf(
        FormNode(
            webDomain = "bank.example.com", webScheme = "https",
            children = listOf(
                FormNode(autofillID = "u", isTextField = true, htmlTag = "input", htmlAttributes = mapOf("type" to "email")),
                FormNode(autofillID = "p", isTextField = true, htmlTag = "input", htmlAttributes = mapOf("type" to "password")),
            ),
        ),
    )

    @Test
    fun testChromeSignedByGoogleGetsTheLoginsOfItsPage() {
        val source = FillRequestParser.parse(bankPage, chrome.packageName, chrome.certificateSHA256, browsers)?.source
        assertEquals(AutofillSource.Web("https", "bank.example.com"), source)
        assertEquals(listOf("Bank"), matcher.listAllMatch(FillTarget.Websites(listOf((source as AutofillSource.Web).serviceIdentifier))).map { it.login.title })
    }

    @Test
    fun testAppNamedLikeChromeWithAnotherCertificateGetsNoLoginsOfThePageItNames() {
        val source = FillRequestParser.parse(bankPage, repackagedChrome.packageName, repackagedChrome.certificateSHA256, browsers)?.source
        assertEquals(AutofillSource.App("com.android.chrome"), source, "it is filled as the app it is")
        assertEquals(emptyList(), matcher.listAllMatch(FillTarget.App(repackagedChrome, "Chrome")), "only logins linked to its certificate, or sites that vouch for it")
        assertEquals(listOf(repackagedChrome), allAskedApp, "Digital Asset Links are asked about the certificate it is signed with")
    }
}
