package com.tryhisar.hisar.model

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** The categories whose list rows show a summary, decoded as HisarKit sends them. */
class ItemSummaryTests {
    @Test
    fun testSoftwareLicenseDecodesFromTheBackendAndSummarizes() {
        val json = """{"category":"software-license","title":"Nova","softwareLicense":{"product":"Nova","version":"v11","licenseKey":"K","licensedTo":"","email":"bob@example.com","purchaseDate":"2024-03-09","downloadUrl":"https://nova.app"}}"""
        val item = backendJson.decodeFromString<ItemDetail>(json)
        assertTrue(item.isSoftwareLicense)
        assertEquals("https://nova.app", item.softwareLicense?.downloadURL)
        assertEquals("v11 · bob@example.com", item.softwareLicense?.summary)
        assertEquals("Nova", ItemSoftwareLicense(product = "Nova").summary)
        assertEquals("Software license", ItemSoftwareLicense().summary)
        val encoded = backendJson.encodeToString(ItemDetail.serializer(), item)
        assertTrue(""""downloadUrl":"https://nova.app"""" in encoded && """"softwareLicense"""" in encoded, encoded)
    }

    @Test
    fun testAPICredentialDecodesFromTheBackendAndSummarizes() {
        val json = """{"category":"api-credential","title":"GitHub token","apiCredential":{"service":"","keyId":"ghp_key","secret":"s","host":"api.github.com","expiry":"2020-01-01","expiryWarning":"This credential has expired"}}"""
        val item = backendJson.decodeFromString<ItemDetail>(json)
        assertTrue(item.isAPICredential)
        assertEquals("ghp_key", item.apiCredential?.keyID)
        assertEquals("This credential has expired", item.apiCredential?.expiryWarning)
        assertEquals("api.github.com", item.apiCredential?.summary)
        assertEquals("API credential", ItemAPICredential().summary)
        val encoded = backendJson.encodeToString(ItemAPICredential.serializer(), item.apiCredential!!)
        assertTrue(""""keyId":"ghp_key"""" in encoded, encoded)
    }

    @Test
    fun testIdentityDecodesFromTheBackendAndSummarizes() {
        val json = """{"category":"identity","title":"Work","identity":{"firstName":"","lastName":"","email":"alice@example.com","nationalIdNumber":"N1"}}"""
        val item = backendJson.decodeFromString<ItemDetail>(json)
        assertTrue(item.isIdentity)
        assertEquals("N1", item.identity?.nationalIDNumber)
        assertEquals("alice@example.com", item.identity?.summary)
        assertEquals("Identity", ItemIdentity().summary)
        val encoded = backendJson.encodeToString(ItemIdentity.serializer(), item.identity!!)
        assertTrue(""""nationalIdNumber":"N1"""" in encoded, encoded)
    }
}
