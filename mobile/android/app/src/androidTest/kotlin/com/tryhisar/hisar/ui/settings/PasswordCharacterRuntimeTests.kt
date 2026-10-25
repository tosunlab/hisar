package com.tryhisar.hisar.ui.settings

import com.tryhisar.hisar.model.AccountLifecycleLogic
import com.tryhisar.hisar.model.ExportFormat
import com.tryhisar.hisar.model.ExportLogic
import org.junit.Assert.assertEquals
import org.junit.Test

class PasswordCharacterRuntimeTests {
    @Test
    fun testPasswordMinimumUsesAndroidGraphemeSegmentation() {
        for ((password, isValid) in listOf(
            "😀".repeat(10) to true,
            "😀".repeat(5) to false,
            "e\u0301".repeat(5) to false,
            "🇹🇷".repeat(5) to false,
            "👩‍👩‍👧‍👦".repeat(5) to false,
            "12345678🇹🇷" to false,
            "123456789🇹🇷" to true,
            "12345678👩‍👩‍👧‍👦" to false,
            "123456789👩‍👩‍👧‍👦" to true,
        )) {
            assertEquals(password, isValid, AccountLifecycleLogic.canChangePassword("old password", password, password))
            assertEquals(password, isValid, ExportLogic.checkInput(ExportFormat.ENCRYPTED, password, password, 10) == null)
        }
    }
}
