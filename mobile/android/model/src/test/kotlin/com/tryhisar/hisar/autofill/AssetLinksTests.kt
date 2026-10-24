package com.tryhisar.hisar.autofill

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class AssetLinksTests {
    private val fingerprint = (0 until 32).joinToString(":") { "%02X".format(it * 7 % 256) }
    private val app = AndroidApp("com.github.android", fingerprint.replace(":", "").lowercase())

    private fun statement(relation: String = AssetLinks.LOGIN_RELATION, packageName: String = "com.github.android", fingerprint: String = this.fingerprint) =
        """{"relation":["$relation"],"target":{"namespace":"android_app","package_name":"$packageName","sha256_cert_fingerprints":["$fingerprint"]}}"""

    @Test
    fun testLoginStatementForThePackageAndCertificateVouches() {
        assertTrue(AssetLinks.isVouchingFor("[${statement()}]", app), "colon hex in the file matches plain hex from the device")
        assertTrue(AssetLinks.isVouchingFor("[${statement(relation = "delegate_permission/common.handle_all_urls")}, ${statement()}]", app))
    }

    @Test
    fun testOtherRelationsPackagesAndCertificatesDoNotVouch() {
        assertFalse(AssetLinks.isVouchingFor("[${statement(relation = "delegate_permission/common.handle_all_urls")}]", app), "opening links is not sharing logins")
        assertFalse(AssetLinks.isVouchingFor("[${statement(packageName = "com.github.android.evil")}]", app))
        assertFalse(AssetLinks.isVouchingFor("[${statement(fingerprint = "AA:".repeat(31) + "AA")}]", app), "a sideloaded app with the package name")
        assertFalse(
            AssetLinks.isVouchingFor("""[{"relation":["${AssetLinks.LOGIN_RELATION}"],"target":{"namespace":"web","site":"https://github.com"}}]""", app),
        )
    }

    @Test
    fun testFilesThatAreNotStatementListsVouchForNothing() {
        assertFalse(AssetLinks.isVouchingFor("", app))
        assertFalse(AssetLinks.isVouchingFor("<html>Not found</html>", app))
        assertFalse(AssetLinks.isVouchingFor(statement(), app), "a single object is not a list")
        assertFalse(AssetLinks.isVouchingFor("""[{"relation":"${AssetLinks.LOGIN_RELATION}","target":{"namespace":"android_app"}}]""", app))
    }

    @Test
    fun testOnlySitesNamedInThePackageAreAsked() {
        val allWebsite = listOf(
            LoginWebsite("github.com", "github.com"),
            LoginWebsite("gist.github.com", "github.com"),
            LoginWebsite("bank.example.com", "example.com"),
            LoginWebsite("android.com", "android.com"),
        )
        assertEquals(
            listOf(
                AssetLinks.Candidate("https://github.com", "https://github.com/.well-known/assetlinks.json"),
                AssetLinks.Candidate("https://android.com", "https://android.com/.well-known/assetlinks.json"),
                AssetLinks.Candidate("https://gist.github.com", "https://gist.github.com/.well-known/assetlinks.json"),
            ),
            AssetLinks.listAllCandidate(app, allWebsite),
        )
        assertEquals(emptyList(), AssetLinks.listAllCandidate(AndroidApp("org.unrelated.app", fingerprint), allWebsite))
    }

    @Test
    fun testManyMatchingSitesAreAllAskedWithTheirDomainsFirst() {
        val allWebsite = (1..500).map { LoginWebsite("host$it.github.com", "github.com") } + LoginWebsite("android.com", "android.com")
        val allServiceIdentifier = AssetLinks.listAllCandidate(app, allWebsite).map { it.serviceIdentifier }
        assertEquals(502, allServiceIdentifier.size)
        assertEquals(listOf("https://github.com", "https://android.com"), allServiceIdentifier.take(2))
        assertTrue("https://host500.github.com" in allServiceIdentifier, "the last host may be the only one that vouches")
    }
}
