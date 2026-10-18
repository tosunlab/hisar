package com.tryhisar.hisar.platform

import android.security.keystore.KeyInfo
import androidx.test.ext.junit.runners.AndroidJUnit4
import java.security.KeyStore
import javax.crypto.SecretKey
import javax.crypto.SecretKeyFactory
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class KeystoreSecretKeyStoreTests {
    private val keyAlias = "com.tryhisar.hisar.tests.secret-key"
    private val directory = createTemporaryDirectory("secret-key-tests")
    private val store = KeystoreSecretKeyStore(directory, keyAlias)
    private val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    @After
    fun tearDown() {
        directory.deleteRecursively()
        keyStore.deleteEntry(keyAlias)
    }

    @Test
    fun testStoreLoadDelete() {
        store.storeSecretKey("ACCOUNT1", "H1-ACCOUNT1-AAAAA")
        store.storeSecretKey("ACCOUNT1", "H1-ACCOUNT1-BBBBB")
        assertEquals("H1-ACCOUNT1-BBBBB", store.loadSecretKey("ACCOUNT1"))
        store.deleteSecretKey("ACCOUNT1")
        assertThrows("a missing Secret Key is an error for Go", Exception::class.java) { store.loadSecretKey("ACCOUNT1") }
        store.deleteSecretKey("ACCOUNT1")
    }

    @Test
    fun testSecretKeyIsEncryptedWithAKeystoreKey() {
        store.storeSecretKey("ACCOUNT2", "H1-ACCOUNT2-CCCCC-DDDDD")

        val file = directory.listFiles().orEmpty().single()
        assertFalse("the file holds the Secret Key in the clear", String(file.readBytes(), Charsets.ISO_8859_1).contains("H1-ACCOUNT2"))
        val key = keyStore.getKey(keyAlias, null) as SecretKey
        assertNull("the Keystore key never leaves the Keystore", key.encoded)
        val keyInfo = SecretKeyFactory.getInstance(key.algorithm, "AndroidKeyStore").getKeySpec(key, KeyInfo::class.java) as KeyInfo
        assertFalse("Go reads the Secret Key without a prompt, as the iPhone's keychain item allows", keyInfo.isUserAuthenticationRequired)

        keyStore.deleteEntry(keyAlias)
        assertThrows("the file is useless without the device's Keystore key", Exception::class.java) { store.loadSecretKey("ACCOUNT2") }
    }
}
