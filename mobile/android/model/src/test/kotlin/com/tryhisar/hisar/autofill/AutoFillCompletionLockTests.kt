package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.model.*
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlin.test.*
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.test.*
import kotlinx.io.bytestring.ByteString
import kotlinx.io.bytestring.encodeToByteString

class AutoFillCompletionLockTests {
    private class HeldBackend : FakeBackend() {
        @Volatile var heldCall: String? = null
        val started = CountDownLatch(1)
        val release = CountDownLatch(1)
        private fun <T> hold(name: String, value: T): T {
            if (heldCall == name) {
                heldCall = null
                started.countDown()
                check(release.await(10, TimeUnit.SECONDS))
            }
            return value
        }
        override fun getStatus() = hold("status", super.getStatus())
        override fun unlock(password: String) = hold("unlock", super.unlock(password))
        override fun unlockWithBiometric() = hold("biometric", super.unlockWithBiometric())
        override fun getLoginForService(vaultID: String, itemID: String, serviceIdentifier: String) =
            hold("password", super.getLoginForService(vaultID, itemID, serviceIdentifier))
        override fun getOneTimeCodeForService(vaultID: String, itemID: String, serviceIdentifier: String) =
            hold("code", super.getOneTimeCodeForService(vaultID, itemID, serviceIdentifier))
        override fun assertPasskey(relyingPartyID: String, credentialID: ByteString, clientDataHash: ByteString, userVerification: String) =
            hold("assert", super.assertPasskey(relyingPartyID, credentialID, clientDataHash, userVerification))
        override fun createPasskey(request: PasskeyRegistrationRequest, vaultID: String, itemID: String) =
            hold("create", super.createPasskey(request, vaultID, itemID))
    }
    private val backend = HeldBackend()
    private val events = MutableSharedFlow<BackendEvent>()
    private val completions = mutableListOf<Any>()
    private val target = FillTarget.Websites(listOf("github.com"))
    private val hash = ByteString(ByteArray(32))
    private fun TestScope.model() = AutoFillModel(
        scope = backgroundScope, ioDispatcher = Dispatchers.IO, createBackend = { backend }, events = events,
        verifier = { _, _ -> emptyList() }, completeRequest = { completions += it },
        completeOneTimeCodeRequest = { completions += it }, copyOneTimeCode = { completions += it }, cancelRequest = { completions += it },
        passkeyCompletion = AutoFillModel.PasskeyCompletion({ completions += it }, { completions += it }, {}, { completions += Unit }),
    )
    private suspend fun TestScope.lockDuring(model: AutoFillModel, call: String, operation: suspend () -> Unit) {
        backend.heldCall = call
        val job = launch { operation() }
        try {
            assertTrue(withContext(Dispatchers.IO) { backend.started.await(10, TimeUnit.SECONDS) })
            backend.lock()
            events.emit(BackendEvent.Locked)
            model.screen.first { it == AutoFillModel.Screen.Locked }
        } finally {
            backend.release.countDown()
        }
        job.join()
        assertEquals(VaultStatus.STATE_LOCKED, backend.getStatus().state)
        assertEquals(AutoFillModel.Screen.Locked, model.screen.value, "a stale completion replaced the lock screen")
        assertTrue(completions.isEmpty(), "a pre-lock credential completed after lock")
    }

    @Test fun testEarlierStatusCannotReplaceTheLockScreen() = runTest {
        backend.seedSignedIn()
        val model = model()
        model.prepareCredentialList(target)
        lockDuring(model, "status") { model.prepareCredentialList(target) }
    }

    @Test fun testCredentialReadBeforeLockCannotCompleteAfterLock() = runTest {
        backend.seedSignedIn()
        val model = model()
        model.prepareCredentialList(target)
        val login = model.allMatchingLogin.value.single()
        lockDuring(model, "password") { model.select(login) }
    }

    @Test fun testPendingPasswordCannotCompleteAfterLock() = runTest {
        backend.seedSignedIn()
        val model = model()
        model.prepareCredentialList(target)
        lockDuring(model, "password") { model.prepareInterfaceToProvideCredential("personal/github", target, LoginSource.Website("https://github.com/")) }
    }

    @Test fun testOneTimeCodeCannotCompleteAfterLock() = runTest {
        backend.seedSignedIn()
        val model = model()
        model.prepareCredentialList(target, RequestKind.ONE_TIME_CODE)
        val login = model.allMatchingLogin.value.single()
        lockDuring(model, "code") { model.select(login) }
    }

    @Test fun testPasswordDoesNotCompleteWhenItsAutomaticCodeReadCrossesLock() = runTest {
        backend.seedSignedIn()
        val model = model()
        model.prepareCredentialList(target)
        val login = model.allMatchingLogin.value.single()
        lockDuring(model, "code") { model.select(login) }
    }

    @Test fun testSilentPasswordReadCannotReturnAfterLock() = runTest {
        backend.seedSignedIn()
        val model = model()
        model.prepareCredentialList(target)
        lockDuring(model, "password") {
            assertNull(model.provideCredentialWithoutUserInteraction("personal/github", LoginSource.Website("https://github.com/")))
        }
    }

    @Test fun testSilentCodeReadCannotReturnAfterLock() = runTest {
        backend.seedSignedIn()
        val model = model()
        model.prepareCredentialList(target)
        lockDuring(model, "code") {
            assertNull(model.provideOneTimeCodeWithoutUserInteraction("personal/github", LoginSource.Website("https://github.com/")))
        }
    }

    @Test fun testPasskeyAssertionCannotCompleteAfterLock() = runTest {
        backend.seedSignedIn()
        val model = model()
        model.prepareCredentialList(target, passkeyRequest = PasskeyAssertionRequest("github.com", hash))
        val passkey = model.allMatchingPasskey.value.single()
        lockDuring(model, "assert") { model.selectPasskey(passkey) }
    }

    @Test fun testSilentPasskeyCannotReturnAfterLock() = runTest {
        backend.seedSignedIn()
        val model = model()
        model.prepareCredentialList(target)
        lockDuring(model, "assert") {
            assertNull(model.providePasskeyWithoutUserInteraction(PasskeyAssertionRequest("github.com", hash, userVerification = "discouraged"), ByteString(1, 1, 1)))
        }
    }

    @Test fun testConfirmedRegistrationCommittedBeforeLock() = runTest {
        backend.seedSignedIn()
        val model = model()
        model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest("example.org", "", "handle".encodeToByteString(), hash))
        backend.heldCall = "create"
        val save = launch { model.savePasskey() }
        try {
            assertTrue(withContext(Dispatchers.IO) { backend.started.await(10, TimeUnit.SECONDS) })
            backend.lock()
            events.emit(BackendEvent.Locked)
            model.screen.first { it == AutoFillModel.Screen.Locked }
        } finally {
            backend.release.countDown()
        }
        save.join()
        assertEquals(1, backend.allPasskey.count { it.relyingPartyID == "example.org" })
        assertEquals(1, completions.filterIsInstance<PasskeyRegistrationCredential>().size, "the vault keeps a passkey whose registration response was dropped")
        assertNull(model.errorMessage.value)
        assertEquals(AutoFillModel.Screen.Locked, model.screen.value)
        assertTrue(model.unlock(FakeBackend.PASSWORD))
        model.savePasskey()
        assertEquals(backend.allPasskey.count { it.relyingPartyID == "example.org" }, completions.size)
    }

    @Test fun testSheetOfferCannotPublishAfterLock() = runTest {
        backend.seedSignedIn()
        val model = model()
        model.prepareCredentialList(target)
        lockDuring(model, "status") {
            val response = model.withinUnlock { withContext(Dispatchers.IO) { backend.getStatus() } }
            if (response != null) completions += response
        }
    }

    @Test fun testSheetOfferScheduledBeforeLockCannotStartAfterIt() = runTest {
        backend.seedSignedIn()
        val model = model()
        model.prepareCredentialList(target)
        val lockCount = model.lockCount.value
        backend.lock()
        events.emit(BackendEvent.Locked)
        model.screen.first { it == AutoFillModel.Screen.Locked }
        assertNull(model.withinUnlock(lockCount) { completions.add(backend.getStatus()) })
        assertTrue(completions.isEmpty())
    }

    @Test fun testEarlierUnlockCannotReplaceALaterLock() = runTest {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = model()
        model.prepareCredentialList(target)
        lockDuring(model, "unlock") { model.unlock(FakeBackend.PASSWORD) }
    }

    @Test fun testEarlierBiometricUnlockCannotReplaceALaterLock() = runTest {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        backend.status = backend.status.copy(biometricReady = true)
        val model = model()
        model.prepareCredentialList(target)
        lockDuring(model, "biometric") { model.unlockWithBiometric() }
    }

    @Test fun testDeviceOwnerAnswerCannotCompleteAfterLock() = runTest {
        for (verified in listOf(true, false)) {
            backend.seedSignedIn()
            val checking = CountDownLatch(1)
            val answer = CountDownLatch(1)
            backend.verifyPasskey = { checking.countDown(); check(answer.await(10, TimeUnit.SECONDS)); verified }
            val model = model()
            model.prepareCredentialList(target, passkeyRequest = PasskeyAssertionRequest("github.com", hash))
            val passkey = model.allMatchingPasskey.value.single()
            val request = launch { model.selectPasskey(passkey) }
            assertTrue(withContext(Dispatchers.IO) { checking.await(10, TimeUnit.SECONDS) })
            backend.lock()
            events.emit(BackendEvent.Locked)
            model.screen.first { it == AutoFillModel.Screen.Locked }
            answer.countDown()
            request.join()
            assertTrue(completions.isEmpty(), "a pre-lock device-owner answer completed the request")
            assertEquals(AutoFillModel.Screen.Locked, model.screen.value)
        }
    }
}
