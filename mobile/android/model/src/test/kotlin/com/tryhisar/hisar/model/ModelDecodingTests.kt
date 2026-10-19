package com.tryhisar.hisar.model

import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

class ModelDecodingTests {
    @Test
    fun testDecodesGoStatus() {
        val json = """{"state":"unlocked","server":"https://hisar.example.com","email":"a@example.com","name":"A","biometricAvailable":true,"biometricEnabled":true,"biometricReady":false,"isOffline":true}"""
        val status = backendJson.decodeFromString<VaultStatus>(json)
        assertTrue(status.isUnlocked)
        assertTrue(status.isOffline)
        assertTrue(status.biometricEnabled)
        assertFalse(status.biometricReady)
    }

    @Test
    fun testDecodesItemSummaryWithNullFields() {
        val json = """[{"id":"i1","revision":"r","category":"","title":"T","username":"u","urls":["a.com"],"password":"","notes":"","fields":null,"publicKey":"","fingerprint":"","keyType":""}]"""
        val allItem = backendJson.decodeFromString<List<ItemDetail>>(json)
        assertEquals(listOf("a.com"), allItem.first().allURL)
        assertEquals(emptyList(), allItem.first().allField)
        assertTrue(allItem.first().isLogin)
        assertFalse(allItem.first().hasOneTimePassword)
        val withCode = backendJson.decodeFromString<ItemDetail>("""{"id":"i2","oneTimePassword":"","hasOneTimePassword":true}""")
        assertTrue(withCode.hasOneTimePassword)
    }

    @Test
    fun testEncodesItemWithGoKeys() {
        val item = ItemDetail(title = "T", allURL = listOf("a.com"), allField = listOf(ItemField("PIN", "1", concealed = true)))
        val encoded = backendJson.encodeToJsonElement(ItemDetail.serializer(), item).jsonObject
        assertEquals(listOf("a.com"), encoded.getValue("urls").jsonArray.map { it.jsonPrimitive.content })
        assertTrue(encoded.getValue("fields").jsonArray.first().jsonObject.getValue("concealed").jsonPrimitive.boolean)
        assertEquals("", encoded.getValue("id").jsonPrimitive.content)
        val withCode = ItemDetail(title = "T", oneTimePassword = "JBSWY3DPEHPK3PXP")
        val encodedWithCode = backendJson.encodeToJsonElement(ItemDetail.serializer(), withCode).jsonObject
        assertEquals("JBSWY3DPEHPK3PXP", encodedWithCode.getValue("oneTimePassword").jsonPrimitive.content)
    }

    @Test
    fun testDecodesLoginsAndSignUpResult() {
        val allLogin = backendJson.decodeFromString<List<LoginMatch>>(
            """[{"vaultID":"v","itemID":"i","title":"T","username":"u","vaultName":"Personal"},{"vaultID":"v","itemID":"j","title":"T","username":"u","vaultName":"Personal","hasOneTimePassword":true}]""",
        )
        assertEquals("v/i", allLogin.first().id)
        assertEquals(listOf(false, true), allLogin.map { it.hasOneTimePassword })
        val credential = backendJson.decodeFromString<LoginCredential>("""{"username":"u","password":"p","hasOneTimePassword":true}""")
        assertEquals(LoginCredential("u", "p", hasOneTimePassword = true), credential)
        val code = backendJson.decodeFromString<OneTimeCode>("""{"code":"287082","expiresAt":"1970-01-01T00:01:00Z","period":30}""")
        assertEquals(Instant.ofEpochSecond(60), code.expiryDate)
        val result = backendJson.decodeFromString<SignUpResult>("""{"status":{"state":"unlocked"},"secretKey":"H1-X"}""")
        assertEquals("H1-X", result.secretKey)
        assertTrue(result.status.isUnlocked)
    }

    @Test
    fun testCardNumberIsGroupedAsTyped() {
        assertEquals("4444 5555 5555 5555", ItemCard.formatNumberInput("4444555555555555"))
        assertEquals("4444 5", ItemCard.formatNumberInput("44445"))
        assertEquals("4444", ItemCard.formatNumberInput("4444 "))
        assertEquals("4111 1111 1111 1111", ItemCard.formatNumberInput("4111-1111-1111-1111"))
        assertEquals("3782 822463 10005", ItemCard.formatNumberInput("378282246310005"))
        assertEquals("3622 720627 1667", ItemCard.formatNumberInput("36227206271667"))
        assertEquals("3056 9300 0902 0004", ItemCard.formatNumberInput("3056930009020004"))
        assertEquals("Store card #12", ItemCard.formatNumberInput("Store card #12"))
    }

    @Test
    fun testExpiryIsWrittenAsMonthAndYearAsTyped() {
        val allCase = mapOf(
            "0" to "0", "01" to "01", "013" to "01/3", "0130" to "01/30", "01305" to "01/30", "3" to "03", "13" to "01/3",
            "1/" to "01/", "01/" to "01/", "1/30" to "01/30", "12/2030" to "12/30", "2030-12" to "12/30", "122030" to "12/30",
            "" to "", "soon" to "soon",
        )
        for ((typed, want) in allCase) {
            assertEquals(want, ItemCard.formatExpiryInput(typed), typed)
        }
    }
}
