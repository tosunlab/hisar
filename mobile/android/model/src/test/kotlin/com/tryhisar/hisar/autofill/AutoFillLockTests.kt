package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.LoginMatch
import com.tryhisar.hisar.model.PasskeyAssertionRequest
import com.tryhisar.hisar.model.PasskeyRegistrationOptions
import com.tryhisar.hisar.model.PasskeyRegistrationRequest
import com.tryhisar.hisar.model.StoredPasskey
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import kotlinx.io.bytestring.ByteString
import kotlinx.io.bytestring.encodeToByteString

/** A read an AutoFill screen began before Hisar locked must not show the locked vault's data when it ends after the lock. */
class AutoFillLockTests {
    // The backend call named [heldCall] gets its answer, then waits for [release] before returning it.
    private class HeldBackend : FakeBackend() {
        @Volatile var heldCall: String? = null
        val started = CountDownLatch(1)
        val release = CountDownLatch(1)

        private fun <Value> hold(name: String, answer: Value): Value {
            if (heldCall == name) {
                started.countDown()
                check(release.await(10, TimeUnit.SECONDS))
            }
            return answer
        }

        override fun listAllLoginForService(serviceIdentifier: String): List<LoginMatch> =
            hold("listAllLoginForService", super.listAllLoginForService(serviceIdentifier))

        override fun listAllPasskeyForRelyingParty(relyingPartyID: String, allAllowedCredentialID: List<ByteString>): List<StoredPasskey> =
            hold("listAllPasskeyForRelyingParty", super.listAllPasskeyForRelyingParty(relyingPartyID, allAllowedCredentialID))

        override fun preparePasskeyRegistration(relyingPartyID: String, allExcludedCredentialID: List<ByteString>): PasskeyRegistrationOptions =
            hold("preparePasskeyRegistration", super.preparePasskeyRegistration(relyingPartyID, allExcludedCredentialID))

        // The last vault's items end the read of every login, so that read is held whole.
        override fun listAllItem(vaultID: String): List<ItemDetail> =
            super.listAllItem(vaultID).let { if (vaultID == allVault.last().id) hold("listAllItem", it) else it }
    }

    private val backend = HeldBackend()
    private val events = MutableSharedFlow<BackendEvent>()
    private val clientDataHash = ByteString(ByteArray(32) { it.toByte() })

    private fun TestScope.createModel() = AutoFillModel(
        scope = backgroundScope,
        ioDispatcher = Dispatchers.IO,
        createBackend = { backend },
        events = events,
        verifier = { _, _ -> emptyList() },
        completeRequest = {},
        completeOneTimeCodeRequest = {},
        copyOneTimeCode = {},
        cancelRequest = {},
        passkeyCompletion = AutoFillModel.PasskeyCompletion({}, {}, {}, {}),
    )

    // Runs [read] until the backend call [heldCall] has its answer, locks Hisar, and lets the answer return.
    private suspend fun TestScope.lockDuring(model: AutoFillModel, heldCall: String, read: suspend () -> Unit) {
        backend.heldCall = heldCall
        val job = launch { read() }
        try {
            assertTrue(withContext(Dispatchers.IO) { backend.started.await(10, TimeUnit.SECONDS) }, "the read reached $heldCall")
            backend.lock()
            events.emit(BackendEvent.Locked)
            model.screen.first { it == AutoFillModel.Screen.Locked }
        } finally {
            backend.release.countDown()
        }
        job.join()
        assertEquals(AutoFillModel.Screen.Locked, model.screen.value)
    }

    @Test
    fun testLoginListReadBeforeTheLockStaysHidden() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareCredentialList(FillTarget.Websites(listOf("github.com")))
        assertTrue(model.allMatchingLogin.value.isNotEmpty())
        lockDuring(model, "listAllLoginForService") { model.loadAllMatchingLogin() }
        assertTrue(model.allMatchingLogin.value.isEmpty(), "the earlier read restored the locked vault's logins")
    }

    @Test
    fun testPasskeyListReadBeforeTheLockStaysHidden() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareCredentialList(FillTarget.Websites(emptyList()), passkeyRequest = PasskeyAssertionRequest("github.com", clientDataHash))
        assertTrue(model.allMatchingPasskey.value.isNotEmpty())
        lockDuring(model, "listAllPasskeyForRelyingParty") { model.loadAllMatchingPasskey() }
        assertTrue(model.allMatchingPasskey.value.isEmpty(), "the earlier read restored the locked vault's passkeys")
    }

    @Test
    fun testSearchReadBeforeTheLockStaysHidden() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareCredentialList(FillTarget.Websites(listOf("github.com")))
        lockDuring(model, "listAllItem") { model.loadAllOtherLogin() }
        assertTrue(model.allOtherLogin.value.isEmpty(), "the earlier read restored the locked vault's logins for search")
        assertEquals(false, model.isOtherLoginLoaded.value)
    }

    @Test
    fun testPasskeyRegistrationOptionsReadBeforeTheLockStayHidden() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest("github.com", "alice", "alice-handle".encodeToByteString(), clientDataHash))
        assertNotNull(model.passkeyRegistrationOptions.value)
        assertEquals("personal/github", model.selectedRegistrationLoginID.value)
        lockDuring(model, "preparePasskeyRegistration") { model.loadPasskeyRegistrationOptions() }
        assertNull(model.passkeyRegistrationOptions.value, "the earlier read restored the logins the passkey could join")
        assertNull(model.selectedRegistrationLoginID.value)
    }
}
