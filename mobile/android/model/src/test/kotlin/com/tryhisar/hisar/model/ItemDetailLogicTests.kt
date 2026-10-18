package com.tryhisar.hisar.model

import kotlin.test.Test
import kotlin.test.assertEquals

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
}
