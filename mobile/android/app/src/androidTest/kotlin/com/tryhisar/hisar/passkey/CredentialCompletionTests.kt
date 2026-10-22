package com.tryhisar.hisar.passkey

import android.content.Intent
import android.content.pm.PackageManager
import android.os.Bundle
import android.service.credentials.BeginGetCredentialOption
import android.service.credentials.BeginGetCredentialRequest
import android.service.credentials.CallingAppInfo
import android.service.credentials.CreateCredentialRequest
import android.service.credentials.CredentialProviderService
import androidx.credentials.CreatePublicKeyCredentialRequest
import androidx.credentials.PasswordCredential
import androidx.credentials.provider.PendingIntentHandler
import androidx.credentials.PublicKeyCredential
import androidx.test.core.app.ApplicationProvider
import androidx.test.filters.SdkSuppress
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.autofill.AutoFillModel
import com.tryhisar.hisar.autofill.InstalledApps
import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.Base64URL
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.PasskeyRegistrationRequest
import java.security.KeyPairGenerator
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.io.bytestring.ByteString
import org.junit.Assert.*
import org.junit.Test

@SdkSuppress(minSdkVersion = 34)
class CredentialCompletionTests {
    private val application = ApplicationProvider.getApplicationContext<HisarTestApplication>()

    private class HeldBackend : FakeBackend() {
        val started = CountDownLatch(1)
        val release = CountDownLatch(1)
        private fun <Value> hold(value: Value): Value {
            started.countDown()
            check(release.await(10, TimeUnit.SECONDS))
            return value
        }
        override fun listAllLoginWebsite() = hold(super.listAllLoginWebsite())
        override fun createPasskey(request: PasskeyRegistrationRequest, vaultID: String, itemID: String): com.tryhisar.hisar.model.PasskeyRegistrationResult {
            val result = super.createPasskey(request, vaultID, itemID)
            val key = KeyPairGenerator.getInstance("EC").apply { initialize(ECGenParameterSpec("secp256r1")) }.generateKeyPair().public as ECPublicKey
            fun coordinate(bytes: ByteArray): ByteArray = bytes.takeLast(32).toByteArray().let { ByteArray(32 - it.size) + it }
            val cose = byteArrayOf(0xa5.toByte(), 1, 2, 3, 0x26, 0x20, 1, 0x21, 0x58, 0x20) +
                coordinate(key.w.affineX.toByteArray()) + byteArrayOf(0x22, 0x58, 0x20) + coordinate(key.w.affineY.toByteArray())
            val credentialID = checkNotNull(Base64URL.decode(result.credentialID)).toByteArray()
            val data = ByteArray(32) + byteArrayOf(0x5d, 0, 0, 0, 0) + ByteArray(16) + byteArrayOf(0, credentialID.size.toByte()) + credentialID + cose
            return hold(result.copy(authenticatorData = Base64URL.encode(ByteString(data))))
        }
    }

    private fun unlockIntent(): Intent {
        val signingInfo = checkNotNull(application.packageManager.getPackageInfo(application.packageName, PackageManager.GET_SIGNING_CERTIFICATES).signingInfo)
        val request = BeginGetCredentialRequest.Builder()
            .setCallingAppInfo(CallingAppInfo(application.packageName, signingInfo))
            .addBeginGetCredentialOption(BeginGetCredentialOption("password", PasswordCredential.TYPE_PASSWORD_CREDENTIAL, Bundle()))
            .build()
        return Intent().putExtra("com.tryhisar.hisar.passkey.PICK", "unlock")
            .putExtra(CredentialProviderService.EXTRA_BEGIN_GET_CREDENTIAL_REQUEST, request)
    }

    @Test fun testCompleteUnlockDoesNotReturnAnOfferReadBeforeLock() = runBlocking {
        val backend = HeldBackend().apply { seedSignedIn() }
        val app = checkNotNull(InstalledApps.readAndroidApp(application, application.packageName))
        backend.addAndroidApp("personal", "github", app.packageName, app.certificateSHA256)
        application.installFakeBackend(backend)
        val viewModel = withContext(Dispatchers.Main) { CredentialViewModel(application, unlockIntent()).also { it.startWhenResumed() } }
        try {
            assertTrue(withContext(Dispatchers.IO) { backend.started.await(10, TimeUnit.SECONDS) })
            backend.lock()
            application.sendEvent(BackendEvent.Locked)
            withTimeout(5_000) { viewModel.model.screen.first { it == AutoFillModel.Screen.Locked } }
        } finally {
            backend.release.countDown()
        }
        assertNull(withTimeoutOrNull(500) { viewModel.result.filterNotNull().first() })
        assertEquals(AutoFillModel.Screen.Locked, viewModel.model.screen.value)
        withContext(Dispatchers.Main) { assertTrue(viewModel.model.unlock(FakeBackend.PASSWORD)) }
        val response = withTimeout(5_000) { viewModel.result.filterNotNull().first() }
        val offer = response.getParcelableExtra(CredentialProviderService.EXTRA_BEGIN_GET_CREDENTIAL_RESPONSE, android.service.credentials.BeginGetCredentialResponse::class.java)
        assertEquals(1, offer?.credentialEntries?.size)
    }

    // A native app (this test app, which example.org vouches for) asks Hisar to save a passkey.
    private fun createIntent(userVerification: String): Intent {
        val app = checkNotNull(InstalledApps.readAndroidApp(application, application.packageName))
        application.statementByRelyingParty["example.org"] =
            """[{"relation":["delegate_permission/common.get_login_creds"],"target":{"namespace":"android_app","package_name":"${app.packageName}","sha256_cert_fingerprints":["${app.certificateSHA256}"]}}]"""
        val signingInfo = checkNotNull(application.packageManager.getPackageInfo(application.packageName, PackageManager.GET_SIGNING_CERTIFICATES).signingInfo)
        val json = """{"rp":{"id":"example.org","name":"Example"},"user":{"id":"aGFuZGxl","name":"alice","displayName":"Alice"},"challenge":"Y2hhbGxlbmdl","pubKeyCredParams":[{"type":"public-key","alg":-7}],"authenticatorSelection":{"userVerification":"$userVerification"}}"""
        val request = CreatePublicKeyCredentialRequest(json)
        val platformRequest = CreateCredentialRequest(CallingAppInfo(application.packageName, signingInfo), PublicKeyCredential.TYPE_PUBLIC_KEY_CREDENTIAL, request.credentialData)
        return Intent().putExtra("com.tryhisar.hisar.passkey.PICK", "create")
            .putExtra(CredentialProviderService.EXTRA_CREATE_CREDENTIAL_REQUEST, platformRequest)
    }

    @Test fun testCommittedRegistrationReturnsItsPublicResponseAfterLock() = runBlocking {
        val backend = HeldBackend().apply { seedSignedIn() }
        application.installFakeBackend(backend)
        val viewModel = withContext(Dispatchers.Main) { CredentialViewModel(application, createIntent("discouraged")).also { it.startWhenResumed() } }
        withTimeout(5_000) { viewModel.model.passkeyRegistrationOptions.filterNotNull().first() }
        val save = launch(Dispatchers.Main) { viewModel.model.savePasskey() }
        try {
            assertTrue(withContext(Dispatchers.IO) { backend.started.await(10, TimeUnit.SECONDS) })
            backend.lock()
            application.sendEvent(BackendEvent.Locked)
            withTimeout(5_000) { viewModel.model.screen.first { it == AutoFillModel.Screen.Locked } }
        } finally {
            backend.release.countDown()
        }
        save.join()
        val response = withTimeout(5_000) { viewModel.result.filterNotNull().first() }
        assertNotNull(PendingIntentHandler.retrieveCreateCredentialResponse(PublicKeyCredential.TYPE_PUBLIC_KEY_CREDENTIAL, response))
        assertEquals(1, backend.allPasskey.count { it.relyingPartyID == "example.org" })
    }
}
