package com.tryhisar.hisar

import com.tryhisar.hisar.app.SettingsModel
import com.tryhisar.hisar.platform.CredentialTransfer.Companion.CREDENTIAL_TRANSFER_FOLDER_NAME
import com.tryhisar.hisar.ui.settings.SHARED_FOLDER_NAME
import java.io.File
import java.nio.file.Files
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse

/** What a process killed before its screens closed leaves in the cache, and what the next launch deletes. */
class HandoffFilesTests {
    @Test
    fun testLaunchDeletesKitFilesTheLastProcessLeft() {
        val cacheDir = Files.createTempDirectory("hisar-cache").toFile()
        try {
            val allLeftover = listOf(
                "${SettingsModel.HANDOFF_FOLDER_NAME}/PendingRecovery-1/Hisar Emergency Kit.pdf",
                "${SettingsModel.HANDOFF_FOLDER_NAME}/EmergencyKit-1/Hisar Emergency Kit.pdf",
                "${SettingsModel.HANDOFF_FOLDER_NAME}/Export-1/hisar-export.json",
                "$SHARED_FOLDER_NAME/Hisar Emergency Kit.pdf",
                "$CREDENTIAL_TRANSFER_FOLDER_NAME/transfer.json",
            ).map { File(cacheDir, it) }
            val kept = File(cacheDir, "website-icons/github.com.png")
            for (file in allLeftover + kept) {
                file.parentFile.mkdirs()
                file.writeText("x")
            }

            deleteLeftoverHandoffFiles(cacheDir)

            for (file in allLeftover) assertFalse(file.exists(), "$file outlived the process that wrote it")
            assertEquals("x", kept.readText())
        } finally {
            cacheDir.deleteRecursively()
        }
    }
}
