package com.tryhisar.hisar.platform

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.security.keystore.StrongBoxUnavailableException
import com.tryhisar.hisarkit.SecretKeyStore
import java.io.File
import java.security.KeyStore
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey

/**
 * Keeps each account's Secret Key in [directory], sealed with one AES-256 Android Keystore key that needs no
 * authentication, as the iPhone's keychain item does (readable after the first unlock, never leaving the device). The
 * directory is the app's no-backup directory, so neither the sealed file nor the key is ever in a backup.
 */
class KeystoreSecretKeyStore(private val directory: File, private val keyAlias: String = DEFAULT_KEY_ALIAS) : SecretKeyStore {
    private val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }

    override fun loadSecretKey(accountID: String): String {
        val key = keyStore.getKey(keyAlias, null) as SecretKey? ?: throw SecretKeyMissingException()
        val file = SealedFile(directory, FILE_PREFIX, accountID)
        val cipher = file.prepareOpen(key) ?: throw SecretKeyMissingException()
        return String(file.open(cipher), Charsets.UTF_8)
    }

    override fun storeSecretKey(accountID: String, secretKey: String) {
        val file = SealedFile(directory, FILE_PREFIX, accountID)
        file.seal(file.prepareSeal(getOrCreateKey()), secretKey.toByteArray(Charsets.UTF_8))
    }

    override fun deleteSecretKey(accountID: String) = SealedFile(directory, FILE_PREFIX, accountID).delete()

    @Synchronized
    private fun getOrCreateKey(): SecretKey {
        (keyStore.getKey(keyAlias, null) as SecretKey?)?.let { return it }
        return try {
            generateKey(isStrongBoxBacked = true)
        } catch (_: StrongBoxUnavailableException) {
            generateKey(isStrongBoxBacked = false)
        }
    }

    private fun generateKey(isStrongBoxBacked: Boolean): SecretKey {
        val specification = KeyGenParameterSpec.Builder(keyAlias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(256)
            .setIsStrongBoxBacked(isStrongBoxBacked)
            .build()
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE).run {
            init(specification)
            generateKey()
        }
    }

    private class SecretKeyMissingException : Exception("the Secret Key is not saved on this device")

    companion object {
        const val DEFAULT_KEY_ALIAS = "com.tryhisar.hisar.secret-key"
        private const val FILE_PREFIX = "secret-key-"
        internal const val ANDROID_KEYSTORE = "AndroidKeyStore"
    }
}
