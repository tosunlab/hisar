package com.tryhisar.hisar.platform

import android.util.AtomicFile
import java.io.File
import javax.crypto.Cipher
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import kotlin.io.encoding.Base64

/**
 * One account's secret sealed with AES-GCM under a Keystore key: the file holds the 12-byte IV, then the ciphertext and
 * its tag. The account ID is the associated data, so one account's file never opens as another's.
 */
internal class SealedFile(directory: File, prefix: String, private val accountID: String) {
    private val file = AtomicFile(File(directory, prefix + Base64.UrlSafe.withPadding(Base64.PaddingOption.ABSENT).encode(accountID.toByteArray())))

    /**
     * The cipher that opens this file, or null when nothing is sealed. A per-use biometric key refuses every use of the
     * cipher before its prompt, so the associated data goes in at [open] and [seal], after the prompt.
     */
    fun prepareOpen(key: SecretKey): Cipher? {
        val sealed = readOrNull() ?: return null
        return Cipher.getInstance(TRANSFORMATION).apply {
            init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(TAG_BITS, sealed, 0, IV_SIZE))
        }
    }

    fun open(cipher: Cipher): ByteArray {
        val sealed = readOrNull() ?: throw IllegalStateException("nothing is saved for this account")
        cipher.updateAAD(accountID.toByteArray())
        return cipher.doFinal(sealed, IV_SIZE, sealed.size - IV_SIZE)
    }

    fun prepareSeal(key: SecretKey): Cipher = Cipher.getInstance(TRANSFORMATION).apply { init(Cipher.ENCRYPT_MODE, key) }

    fun seal(cipher: Cipher, secret: ByteArray) {
        cipher.updateAAD(accountID.toByteArray())
        val sealed = cipher.iv + cipher.doFinal(secret)
        val stream = file.startWrite()
        try {
            stream.write(sealed)
            file.finishWrite(stream)
        } catch (error: Exception) {
            file.failWrite(stream)
            throw error
        }
    }

    fun delete() = file.delete()

    private fun readOrNull(): ByteArray? = if (file.baseFile.exists()) file.readFully() else null

    private companion object {
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val IV_SIZE = 12
        const val TAG_BITS = 128
    }
}
