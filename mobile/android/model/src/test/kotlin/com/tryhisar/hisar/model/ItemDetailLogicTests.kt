package com.tryhisar.hisar.model

import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ItemDetailLogicTests {
    @Test
    fun testItemDetailDecodesAndroidAppsAndDefaultsToNone() {
        val json = """{"id":"1","title":"Bank","urls":["https://bank.example"],"androidApps":[{"packageName":"com.bank.app","certificateSha256":"14:6D:E9"}]}"""
        val linked = backendJson.decodeFromString<ItemDetail>(json)
        assertEquals(listOf(ItemAndroidApp(packageName = "com.bank.app", certificateSHA256 = "14:6D:E9")), linked.allAndroidApp)
        assertEquals(listOf("https://bank.example"), linked.allURL)
        val older = backendJson.decodeFromString<ItemDetail>("""{"id":"2","title":"Bank","androidApps":null}""")
        assertEquals(emptyList(), older.allAndroidApp)
    }

    @Test
    fun testServerDecodesFromTheBackendAndWarnsAboutItsPort() {
        val json = """{"category":"server","title":"Database","username":"","server":{"host":"2001:db8::1","port":"5432","sshKeyId":"key-1","sshKeyTitle":"Deploy key","sshKeyFingerprint":"SHA256:abc","portWarning":""}}"""
        val item = backendJson.decodeFromString<ItemDetail>(json)
        assertTrue(item.isServer)
        assertEquals("Deploy key", item.server?.sshKeyTitle)
        assertEquals("[2001:db8::1]:5432", item.server?.address)
        assertEquals("[2001:db8::1]:5432", item.server?.summary(username = ""))
        assertEquals("Server", ItemServer().summary(username = ""))
        val encoded = backendJson.encodeToString(ItemDetail.serializer(), item)
        assertTrue(""""sshKeyId":"key-1"""" in encoded && """"server"""" in encoded, encoded)
        val warningByPort = mapOf(
            "" to "", "22" to "", "65535" to "", "0" to ItemServer.PORT_WARNING_TEXT, "70000" to ItemServer.PORT_WARNING_TEXT,
            "ssh" to ItemServer.PORT_WARNING_TEXT, "２２" to ItemServer.PORT_WARNING_TEXT,
        )
        for ((port, warning) in warningByPort) assertEquals(warning, ItemServer.describePort(port), port)
    }

    @Test
    fun testDatabaseDecodesFromTheBackend() {
        val json = """{"category":"database","title":"Cache","username":"","database":{"engine":"redis","engineName":"Redis","host":"2001:db8::1","port":"6379","name":"","options":"","connectionString":"redis://[2001:db8::1]:6379","portWarning":""}}"""
        val item = backendJson.decodeFromString<ItemDetail>(json)
        assertTrue(item.isDatabase)
        assertEquals("redis://[2001:db8::1]:6379", item.database?.connectionString)
        assertEquals("Redis · [2001:db8::1]:6379", item.database?.summary)
        assertEquals("Database", ItemDatabase().summary)
        assertEquals("DB2 · app", ItemDatabase(engineName = "DB2", name = "app").summary)
        assertTrue(DatabaseEngine.isKnown("sqlserver"))
        assertFalse(DatabaseEngine.isKnown("db2"))
        val encoded = backendJson.encodeToString(ItemDetail.serializer(), item)
        assertTrue(""""database"""" in encoded && """"engine":"redis"""" in encoded, encoded)
    }

    @Test
    fun testOneTimeCodeFormattingAndCountdown() {
        val code = OneTimeCode(code = "123456", expiresAt = "2026-10-09T12:00:30Z", period = 30)
        assertEquals("123 456", code.formattedCode)
        assertEquals("1234 5678", OneTimeCode(code = "12345678", expiresAt = "", period = 30).formattedCode)
        assertEquals("1234 567", OneTimeCode(code = "1234567", expiresAt = "", period = 30).formattedCode)
        val now = Instant.parse("2026-10-09T12:00:20Z")
        assertEquals(10, code.countSecondsLeft(now.plusMillis(400)))
        assertEquals(0, code.countSecondsLeft(now.plusSeconds(11)))
        assertEquals(0, OneTimeCode(code = "1", expiresAt = "not a date", period = 30).countSecondsLeft(now))
    }

    @Test
    fun testOpenableURLAddsHTTPSAndRejectsOtherSchemes() {
        assertEquals("https://github.com", VaultLogic.convertToOpenableURL("github.com")?.toString())
        assertEquals("http://router.local:8080/x", VaultLogic.convertToOpenableURL(" http://router.local:8080/x ")?.toString())
        assertNull(VaultLogic.convertToOpenableURL("javascript://alert(1)"))
        assertNull(VaultLogic.convertToOpenableURL(""))
        assertEquals("accounts.google.com", VaultLogic.extractHost("HTTPS://Accounts.Google.com/signin"))
    }

    @Test
    fun testOpenableURLSupportsInternationalHostsWithoutChangingStoredText() {
        val item = backendJson.decodeFromString<ItemDetail>("""{"urls":["https://例え.test:8443/a%2Fb/東京?q=%2F+one#part%20two"]}""")
        assertEquals(
            "https://例え.test:8443/a%2Fb/東京?q=%2F+one#part%20two",
            VaultLogic.convertToOpenableURL(item.allURL.single())?.toString(),
        )
        assertEquals("https://例え.test:8443/a%2Fb/東京?q=%2F+one#part%20two", item.allURL.single())
        val expectedByWebsite = mapOf(
            "例え.test" to "https://例え.test",
            "HTTP://bücher.test:8080/download" to "HTTP://bücher.test:8080/download",
            "https://user%3Aname:p%40ss@例え.test:8443/a%2Fb?q=%2F#part" to "https://user%3Aname:p%40ss@例え.test:8443/a%2Fb?q=%2F#part",
            "https://user:pass@Example.COM:8443/a%2Fb?q=%2F#part" to "https://user:pass@Example.COM:8443/a%2Fb?q=%2F#part",
            "http://user:pass@[2001:db8::1]:8080/a%2Fb?q=%2F#part" to "http://user:pass@[2001:db8::1]:8080/a%2Fb?q=%2F#part",
            "https://xn--r8jz45g.test" to "https://xn--r8jz45g.test",
        )
        for ((website, expected) in expectedByWebsite) {
            assertEquals(expected, VaultLogic.convertToOpenableURL(website)?.toString(), website)
        }
        for (invalid in listOf("https://例え..test", "https://-例え.test", "https://例え-.test", "https://\u200D.test", "https://例え.test:port", "https://例え.test:８０", "https://例え.test/%zz", "ftp://例え.test")) {
            assertNull(VaultLogic.convertToOpenableURL(invalid), invalid)
        }
        assertNull(VaultLogic.extractHost("https://例え.test"))
    }

    @Test
    fun testInternationalHostOpensTheStoredDomain() {
        assertEquals("https://faß.de/path", VaultLogic.convertToOpenableURL("https://faß.de/path")?.toString())
        assertNull(VaultLogic.extractHost("https://faß.de/path"))
    }

    @Test
    fun testDetailHeaderNamesTheCategoryOrItsSummary() {
        assertEquals("Login", ItemDetail(title = "GitHub").categoryDescription)
        assertEquals("SSH key", ItemDetail(category = ItemDetail.CATEGORY_SSH_KEY).categoryDescription)
        assertEquals("Secure note", ItemDetail(category = ItemDetail.CATEGORY_SECURE_NOTE).categoryDescription)
        assertEquals("Visa •••• 4242", ItemDetail(category = ItemDetail.CATEGORY_CREDIT_CARD, card = ItemCard(type = "Visa", last4 = "4242")).categoryDescription)
        assertEquals("Credit card", ItemDetail(category = ItemDetail.CATEGORY_CREDIT_CARD).categoryDescription)
        assertEquals("Springfield, US", ItemDetail(category = ItemDetail.CATEGORY_ADDRESS, address = ItemAddress(city = "Springfield", country = "US")).categoryDescription)
        assertEquals("build.example.com:2222 · deploy", ItemDetail(category = ItemDetail.CATEGORY_SERVER, username = "deploy", server = ItemServer(host = "build.example.com", port = "2222")).categoryDescription)
    }

    @Test
    fun testSecretReferenceButtonNamesTheFieldAScriptWants() {
        assertEquals("password", ItemDetail(password = "p").referenceFieldName)
        assertNull(ItemDetail(username = "alice").referenceFieldName, "a login without a password has nothing to reference")
        assertEquals("password", ItemDetail(category = ItemDetail.CATEGORY_DATABASE, password = "p").referenceFieldName)
        assertEquals("public key", ItemDetail(category = ItemDetail.CATEGORY_SSH_KEY).referenceFieldName)
        assertEquals("notes", ItemDetail(category = ItemDetail.CATEGORY_SECURE_NOTE, notes = "n").referenceFieldName)
        assertEquals(CardFieldName.NUMBER, ItemDetail(category = ItemDetail.CATEGORY_CREDIT_CARD, card = ItemCard(number = "4242")).referenceFieldName)
        assertEquals(APICredentialFieldName.SECRET, ItemDetail(category = ItemDetail.CATEGORY_API_CREDENTIAL, apiCredential = ItemAPICredential(secret = "s")).referenceFieldName)
        assertEquals(SoftwareLicenseFieldName.LICENSE_KEY, ItemDetail(category = ItemDetail.CATEGORY_SOFTWARE_LICENSE, softwareLicense = ItemSoftwareLicense(licenseKey = "k")).referenceFieldName)
        assertNull(ItemDetail(category = ItemDetail.CATEGORY_IDENTITY, identity = ItemIdentity(passportNumber = "U1")).referenceFieldName)
        assertNull(ItemDetail(category = ItemDetail.CATEGORY_ADDRESS).referenceFieldName)
    }
}
