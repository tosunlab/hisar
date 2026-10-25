package com.tryhisar.hisar.model

import kotlin.test.Test
import kotlin.test.assertEquals

class PasswordCharacterTests {
    private val passwords = listOf(
        "😀".repeat(10) to true,
        "😀".repeat(5) to false,
        "e\u0301".repeat(5) to false,
        "🇹🇷".repeat(5) to false,
        "👩‍👩‍👧‍👦".repeat(5) to false,
        "12345678🇹🇷" to false,
        "123456789🇹🇷" to true,
        "12345678👩‍👩‍👧‍👦" to false,
        "123456789👩‍👩‍👧‍👦" to true,
    )

    @Test
    fun testMasterPasswordMinimumCountsGraphemeClusters() {
        for ((password, isValid) in passwords) {
            assertEquals(isValid, AccountLifecycleLogic.canChangePassword("old password", password, password), password)
        }
    }

    @Test
    fun testExportPassphraseMinimumCountsGraphemeClusters() {
        for ((password, isValid) in passwords) {
            assertEquals(isValid, ExportLogic.checkInput(ExportFormat.ENCRYPTED, password, password, 10) == null, password)
        }
    }
}
