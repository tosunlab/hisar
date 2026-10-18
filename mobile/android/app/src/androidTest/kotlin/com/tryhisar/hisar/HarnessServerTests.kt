package com.tryhisar.hisar

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.tryhisar.hisar.model.SignUpResult
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.backendJson
import com.tryhisar.hisarkit.Hisar
import com.tryhisar.hisarkit.Hisarkit
import com.tryhisar.hisarkit.SecretKeyStore
import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** HisarKit on the device reaches the server scripts/test-android-ui.sh started, with the invite it created. */
@RunWith(AndroidJUnit4::class)
class HarnessServerTests : HisarUITestCase() {
    @Test
    fun testInviteCreatesAnAccountThatSignsInAgain() {
        val server = readArgument("HISAR_UI_TEST_SERVER")
        val inviteLink = readArgument("HISAR_UI_TEST_SIGN_IN_AGAIN_INVITE_LINK")
        val email = readArgument("HISAR_UI_TEST_SIGN_IN_AGAIN_EMAIL")

        val signedUp = backendJson.decodeFromString<SignUpResult>(openPhone("first").signUp(inviteLink, email, "Android Harness", MASTER_PASSWORD))
        assertTrue("signed up and unlocked: ${signedUp.status}", signedUp.status.isUnlocked)
        assertTrue("a Secret Key: ${signedUp.secretKey}", signedUp.secretKey.startsWith("H1-"))

        val secondPhone = openPhone("second")
        val signedIn = backendJson.decodeFromString<VaultStatus>(secondPhone.signIn(server, email, signedUp.secretKey, MASTER_PASSWORD))
        assertTrue("signed in and unlocked: $signedIn", signedIn.isUnlocked)
        assertEquals(email, signedIn.email)
        val vaults = backendJson.decodeFromString<List<VaultSummary>>(secondPhone.listAllVault())
        assertEquals(listOf("Personal"), vaults.map { it.name })
    }

    private fun openPhone(name: String): Hisar {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val stateDirectory = File(context.noBackupFilesDir, "harness-$name").apply { mkdirs() }
        val version = context.packageManager.getPackageInfo(context.packageName, 0).versionName.orEmpty()
        return Hisarkit.new_(stateDirectory.path, "Hisar harness $name", MemorySecretKeyStore(), null, null, null).apply { setAppVersion(version) }
    }

    private class MemorySecretKeyStore : SecretKeyStore {
        private val secretKeys = mutableMapOf<String, String>()

        override fun loadSecretKey(accountID: String): String = secretKeys[accountID] ?: throw IllegalStateException("no Secret Key for $accountID")

        override fun storeSecretKey(accountID: String, secretKey: String) {
            secretKeys[accountID] = secretKey
        }

        override fun deleteSecretKey(accountID: String) {
            secretKeys.remove(accountID)
        }
    }
}
