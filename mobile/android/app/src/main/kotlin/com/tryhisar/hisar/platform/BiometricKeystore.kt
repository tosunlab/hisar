package com.tryhisar.hisar.platform

import android.content.Context
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_STRONG
import androidx.biometric.BiometricManager.Authenticators.DEVICE_CREDENTIAL
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import com.tryhisar.hisarkit.PasskeyVerifier
import java.util.concurrent.TimeUnit
import androidx.biometric.BiometricPrompt
import com.tryhisar.hisarkit.Biometric
import com.tryhisar.hisarkit.BiometricKeyPreparer
import java.io.File
import java.security.KeyStore
import java.util.concurrent.CompletableFuture
import java.util.concurrent.ExecutionException
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey

/**
 * Biometric unlock with a per-use AES-256-GCM Keystore key per account, which only a strong biometric opens and a new
 * enrollment destroys. Go calls prepareStoreKey or prepareLoadKey, then authenticate, whose prompt carries that cipher,
 * then storeKey or loadKey with the cipher the prompt authorized: one prompt per use. An authenticate without a prepare
 * (approvals) shows a plain prompt.
 */
class BiometricKeystore(
    private val context: Context,
    private val directory: File,
    private val foregroundActivity: ForegroundActivity,
    private val keyAliasPrefix: String = DEFAULT_KEY_ALIAS_PREFIX,
) : Biometric, BiometricKeyPreparer, PasskeyVerifier {
    private enum class Operation { STORE, LOAD }

    private class KeyUse(val operation: Operation, val accountID: String, val cipher: Cipher)

    private val keyStore = KeyStore.getInstance(KeystoreSecretKeyStore.ANDROID_KEYSTORE).apply { load(null) }
    private val mainHandler = Handler(Looper.getMainLooper())
    private var prepared: KeyUse? = null
    private var authorized: KeyUse? = null

    override fun isAvailable(): Boolean = BiometricManager.from(context).canAuthenticate(BIOMETRIC_STRONG) == BiometricManager.BIOMETRIC_SUCCESS

    override fun prepareStoreKey(accountID: String) {
        deleteKey(accountID)
        val cipher = file(accountID).prepareSeal(createKey(accountID))
        synchronized(this) { prepared = KeyUse(Operation.STORE, accountID, cipher) }
    }

    /** Throws when the key is gone or a new enrollment invalidated it; Go then turns biometric unlock off. */
    override fun prepareLoadKey(accountID: String) {
        val key = keyStore.getKey(alias(accountID), null) as SecretKey? ?: throw IllegalStateException("the key is not on this device")
        val cipher = file(accountID).prepareOpen(key) ?: throw IllegalStateException("the key is not on this device")
        synchronized(this) { prepared = KeyUse(Operation.LOAD, accountID, cipher) }
    }

    override fun authenticate(reason: String) {
        check(Looper.myLooper() != Looper.getMainLooper()) { "the biometric prompt waits for the main thread" }
        val keyUse = synchronized(this) { prepared.also { prepared = null; authorized = null } }
        val result = CompletableFuture<BiometricPrompt.AuthenticationResult>()
        mainHandler.post { showPrompt(formatReason(reason), keyUse?.cipher, result) }
        val authenticated = try {
            result.get()
        } catch (error: ExecutionException) {
            throw error.cause ?: error
        }
        if (keyUse != null) {
            val cipher = authenticated.cryptoObject?.cipher ?: throw BiometricException("The biometric prompt did not unlock the key.")
            synchronized(this) { authorized = KeyUse(keyUse.operation, keyUse.accountID, cipher) }
        }
    }

    override fun authenticatePasskey(reason: String) {
        check(Looper.myLooper() != Looper.getMainLooper()) { "passkey verification waits for the main thread" }
        val result = CompletableFuture<Unit>()
        mainHandler.post {
            val activity = foregroundActivity.current
            if (activity == null || activity.isFinishing || activity.isDestroyed) {
                result.completeExceptionally(BiometricException("Open Hisar to verify this passkey."))
                return@post
            }
            val callback = object : BiometricPrompt.AuthenticationCallback() {
                override fun onAuthenticationSucceeded(authenticated: BiometricPrompt.AuthenticationResult) {
                    if (foregroundActivity.current === activity && !activity.isFinishing && !activity.isDestroyed) {
                        result.complete(Unit)
                    } else {
                        result.completeExceptionally(BiometricException("This passkey ceremony has ended."))
                    }
                }

                override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                    result.completeExceptionally(BiometricException(errString.toString()))
                }
            }
            val prompt = BiometricPrompt(activity, context.mainExecutor, callback)
            val observer = object : DefaultLifecycleObserver {
                override fun onDestroy(owner: LifecycleOwner) {
                    result.completeExceptionally(BiometricException("This passkey ceremony has ended."))
                    prompt.cancelAuthentication()
                }
            }
            activity.lifecycle.addObserver(observer)
            result.whenComplete { _, _ -> mainHandler.post {
                activity.lifecycle.removeObserver(observer)
                prompt.cancelAuthentication()
            } }
            prompt.authenticate(BiometricPrompt.PromptInfo.Builder()
                .setTitle(formatReason(reason))
                .setAllowedAuthenticators(BIOMETRIC_STRONG or DEVICE_CREDENTIAL)
                .setConfirmationRequired(true)
                .build())
        }
        try {
            result.get(120, TimeUnit.SECONDS)
        } catch (error: Exception) {
            result.completeExceptionally(error)
            throw (error as? ExecutionException)?.cause ?: error
        }
    }

    override fun storeKey(accountID: String, key: ByteArray) {
        file(accountID).seal(takeAuthorized(Operation.STORE, accountID), key)
    }

    override fun loadKey(accountID: String): ByteArray = file(accountID).open(takeAuthorized(Operation.LOAD, accountID))

    override fun deleteKey(accountID: String) {
        file(accountID).delete()
        keyStore.deleteEntry(alias(accountID))
    }

    private fun showPrompt(title: String, cipher: Cipher?, result: CompletableFuture<BiometricPrompt.AuthenticationResult>) {
        val activity = foregroundActivity.current
        if (activity == null) {
            result.completeExceptionally(BiometricException("Open Hisar to use biometrics."))
            return
        }
        val callback = object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(authenticated: BiometricPrompt.AuthenticationResult) {
                result.complete(authenticated)
            }

            override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                result.completeExceptionally(BiometricException(describeError(errorCode, errString)))
            }
        }
        val promptInfo = BiometricPrompt.PromptInfo.Builder()
            .setTitle(title)
            .setNegativeButtonText("Use master password")
            .setAllowedAuthenticators(BIOMETRIC_STRONG)
            .setConfirmationRequired(false)
            .build()
        val prompt = BiometricPrompt(activity, context.mainExecutor, callback)
        if (cipher == null) prompt.authenticate(promptInfo) else prompt.authenticate(promptInfo, BiometricPrompt.CryptoObject(cipher))
    }

    private fun takeAuthorized(operation: Operation, accountID: String): Cipher {
        val keyUse = synchronized(this) { authorized.also { authorized = null } }
        if (keyUse == null || keyUse.operation != operation || keyUse.accountID != accountID) {
            throw BiometricException("Hisar used the biometric key without asking for biometrics first.")
        }
        return keyUse.cipher
    }

    private fun createKey(accountID: String): SecretKey {
        val specification = KeyGenParameterSpec.Builder(alias(accountID), KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(256)
            .setUserAuthenticationRequired(true)
            .setInvalidatedByBiometricEnrollment(true)
            .apply {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                    setUserAuthenticationParameters(0, KeyProperties.AUTH_BIOMETRIC_STRONG)
                }
            }
            .build()
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KeystoreSecretKeyStore.ANDROID_KEYSTORE).run {
            init(specification)
            generateKey()
        }
    }

    private fun alias(accountID: String) = keyAliasPrefix + accountID

    private fun file(accountID: String) = SealedFile(directory, FILE_PREFIX, accountID)

    private class BiometricException(message: String) : Exception(message)

    companion object {
        const val DEFAULT_KEY_ALIAS_PREFIX = "com.tryhisar.hisar.biometric-unlock."
        private const val FILE_PREFIX = "biometric-unlock-"

        /** Go's reasons are lower-case phrases such as "unlock Hisar". */
        private fun formatReason(reason: String): String = reason.replaceFirstChar { it.uppercaseChar() }.ifEmpty { "Unlock Hisar" }

        /** The app sorts these by "was cancelled" and "master password", as the iPhone app does. */
        private fun describeError(errorCode: Int, errString: CharSequence): String = when (errorCode) {
            BiometricPrompt.ERROR_NEGATIVE_BUTTON -> "Enter your master password."
            BiometricPrompt.ERROR_USER_CANCELED, BiometricPrompt.ERROR_CANCELED, BiometricPrompt.ERROR_TIMEOUT -> "The biometric check was cancelled."
            BiometricPrompt.ERROR_LOCKOUT, BiometricPrompt.ERROR_LOCKOUT_PERMANENT -> "Biometrics are locked. Unlock with your master password."
            else -> errString.toString().trimEnd('.') + "."
        }
    }
}
